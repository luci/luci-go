// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package importer handles all configs importing.
package importer

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/klauspost/compress/gzip"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/logging"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/config_service/internal/acl"
	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/metrics"
	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/internal/settings"
	"go.chromium.org/luci/config_service/internal/taskpb"
	"go.chromium.org/luci/config_service/internal/validation"
)

const (
	// compressedContentLimit is the maximum allowed compressed content size to
	// store into Datastore, in order to avoid exceeding 1MiB limit per entity.
	compressedContentLimit = 800 * 1024

	// maxRetryCount is the maximum number of retries allowed when importing
	// a single config set hit non-fatal error.
	maxRetryCount = 5

	// importAllConfigsInterval is the interval between two import config cron
	// jobs.
	importAllConfigsInterval = 10 * time.Minute
)

var (
	// ErrFatalTag is an error tag to indicate an unrecoverable error in the
	// configs importing flow.
	ErrFatalTag = errtag.Make("A config importing unrecoverable error", true)
)

// validator defines the interface to interact with validation logic.
type validator interface {
	Validate(context.Context, config.Set, []validation.File) (*cfgcommonpb.ValidationResult, error)
}

// Importer is able to import a config set.
type Importer struct {
	// GSBucket is the bucket name where the imported configs will be stored to.
	GSBucket string
	// Validator is used to validate the configs before import.
	Validator validator
}

// RegisterImportConfigsCron register the cron to trigger import for all config
// sets
func (i *Importer) RegisterImportConfigsCron(dispatcher *tq.Dispatcher) {
	i.registerTQTask(dispatcher)
	cron.RegisterHandler("import-configs", func(ctx context.Context) error {
		return importAllConfigs(ctx, dispatcher)
	})
}

func (i *Importer) registerTQTask(dispatcher *tq.Dispatcher) {
	dispatcher.RegisterTaskClass(tq.TaskClass{
		ID:        "import-configs",
		Kind:      tq.NonTransactional,
		Prototype: (*taskpb.ImportConfigs)(nil),
		Queue:     "backend-v2",
		Handler: func(ctx context.Context, payload proto.Message) error {
			task := payload.(*taskpb.ImportConfigs)
			switch err := i.ImportConfigSet(ctx, config.Set(task.GetConfigSet())); {
			case ErrFatalTag.In(err):
				return tq.Fatal.Apply(err)
			case err != nil: // non-fatal error
				if info := tq.TaskExecutionInfo(ctx); info != nil && info.ExecutionCount >= maxRetryCount {
					// ignore the task as it exceeds the max retry count. Alert should be
					// set up to monitor the number of retries.
					return tq.Ignore.Apply(err)
				}
				return err // tq will retry the error
			default:
				return nil
			}
		},
	})
}

// importAllConfigs schedules a task for each service and project config set to
// import configs from Gitiles and clean up stale config sets.
func importAllConfigs(ctx context.Context, dispatcher *tq.Dispatcher) error {
	cfgLoc := settings.GetGlobalConfigLoc(ctx)

	// Get all config sets.
	cfgSets, err := getAllServiceCfgSets(ctx, cfgLoc)
	if err != nil {
		return errors.Fmt("failed to load service config sets: %w", err)
	}
	sets, err := getAllProjCfgSets(ctx)
	if err != nil {
		return errors.Fmt("failed to load project config sets: %w", err)
	}
	cfgSets = append(cfgSets, sets...)

	// Enqueue tasks
	err = parallel.WorkPool(8, func(workCh chan<- func() error) {
		for _, cs := range cfgSets {
			workCh <- func() error {
				err := dispatcher.AddTask(ctx, &tq.Task{
					Payload: &taskpb.ImportConfigs{ConfigSet: cs},
					Title:   fmt.Sprintf("configset/%s", cs),
					ETA:     clock.Now(ctx).Add(common.DistributeOffset(importAllConfigsInterval, "config_set", string(cs))),
				})
				return errors.WrapIf(err, "failed to enqueue ImportConfigs task for %q: %s", cs, err)
			}
		}
	})
	if err != nil {
		return err
	}

	// Delete stale config sets.
	var keys []*datastore.Key
	if err := datastore.GetAll(ctx, datastore.NewQuery(model.ConfigSetKind).KeysOnly(true), &keys); err != nil {
		return errors.Fmt("failed to fetch all config sets from Datastore: %w", err)
	}
	cfgSetsInDB := stringset.New(len(keys))
	for _, key := range keys {
		cfgSetsInDB.Add(key.StringID())
	}
	cfgSetsInDB.DelAll(cfgSets)
	if len(cfgSetsInDB) > 0 {
		logging.Infof(ctx, "deleting stale config sets: %v", cfgSetsInDB)
		var toDel []*datastore.Key
		for _, cs := range cfgSetsInDB.ToSlice() {
			toDel = append(toDel, datastore.KeyForObj(ctx, &model.ConfigSet{ID: config.Set(cs)}))
		}
		return datastore.Delete(ctx, toDel)
	}
	return nil
}

// getAllProjCfgSets fetches all "projects/*" config sets
func getAllProjCfgSets(ctx context.Context) ([]string, error) {
	projectsCfg := &cfgcommonpb.ProjectsCfg{}
	var nerr *model.NoSuchConfigError
	switch err := common.LoadSelfConfig[*cfgcommonpb.ProjectsCfg](ctx, common.ProjRegistryFilePath, projectsCfg); {
	case errors.As(err, &nerr) && nerr.IsUnknownConfigSet():
		// May happen on the cron job first run. Just log the warning.
		logging.Warningf(ctx, "failed to compose all project config sets because the self config set is missing")
		return nil, nil
	case err != nil:
		return nil, err
	}
	cfgsets := make([]string, len(projectsCfg.Projects))
	for i, proj := range projectsCfg.Projects {
		cs, err := config.ProjectSet(proj.Id)
		if err != nil {
			return nil, err
		}
		cfgsets[i] = string(cs)
	}
	return cfgsets, nil
}

// getAllServiceCfgSets returns all "service/*" config sets.
func getAllServiceCfgSets(ctx context.Context, cfgLoc *cfgcommonpb.GitilesLocation) ([]string, error) {
	if cfgLoc == nil {
		return nil, nil
	}
	host, project, err := gitiles.ParseRepoURL(cfgLoc.Repo)
	if err != nil {
		return nil, errors.Fmt("invalid gitiles repo: %s: %w", cfgLoc.Repo, err)
	}
	gitilesClient, err := clients.NewGitilesClient(ctx, host, "")
	if err != nil {
		return nil, errors.Fmt("failed to create a gitiles client: %w", err)
	}

	res, err := gitilesClient.ListFiles(ctx, &gitilespb.ListFilesRequest{
		Project:    project,
		Committish: cfgLoc.Ref,
		Path:       cfgLoc.Path,
	})
	if err != nil {
		return nil, errors.Fmt("failed to call Gitiles to list files: %w", err)
	}
	var cfgSets []string
	for _, f := range res.GetFiles() {
		if f.Type != git.File_TREE {
			continue
		}
		cs, err := config.ServiceSet(f.GetPath())
		if err != nil {
			logging.Errorf(ctx, "skip importing service config: %s", err)
			continue
		}
		cfgSets = append(cfgSets, string(cs))
	}
	return cfgSets, nil
}

// ImportConfigSet tries to import a config set.
// TODO(crbug.com/1446839): Optional: for ErrFatalTag errors or errors which are
// retried many times, may send notifications to Config Service owners in future
// after the notification functionality is done.
func (i *Importer) ImportConfigSet(ctx context.Context, cfgSet config.Set) error {
	if sID := cfgSet.Service(); sID != "" {
		globalCfgLoc := settings.GetGlobalConfigLoc(ctx)
		return i.importConfigSet(ctx, cfgSet, &cfgcommonpb.GitilesLocation{
			Repo: globalCfgLoc.Repo,
			Ref:  globalCfgLoc.Ref,
			Path: strings.TrimPrefix(fmt.Sprintf("%s/%s", globalCfgLoc.Path, sID), "/"),
		})
	} else if pID := cfgSet.Project(); pID != "" {
		return i.importProject(ctx, pID)
	}
	return ErrFatalTag.Apply(errors.
		Fmt("Invalid config set: %q", cfgSet))
}

// importProject imports a project config set.
func (i *Importer) importProject(ctx context.Context, projectID string) error {
	projectsCfg := &cfgcommonpb.ProjectsCfg{}
	if err := common.LoadSelfConfig[*cfgcommonpb.ProjectsCfg](ctx, common.ProjRegistryFilePath, projectsCfg); err != nil {
		return ErrFatalTag.Apply(err)
	}
	var projLoc *cfgcommonpb.GitilesLocation
	for _, p := range projectsCfg.GetProjects() {
		if p.Id == projectID {
			projLoc = p.GetGitilesLocation()
			break
		}
	}
	if projLoc == nil {
		return ErrFatalTag.Apply(errors.Fmt("project %q not exist or has no gitiles location", projectID))
	}
	return i.importConfigSet(ctx, config.MustProjectSet(projectID), projLoc)
}

// importConfigSet tries to import the latest version of the given config set.
// TODO(crbug.com/1446839): Add code to report to metrics
func (i *Importer) importConfigSet(ctx context.Context, cfgSet config.Set, loc *cfgcommonpb.GitilesLocation) error {
	ctx = logging.SetFields(ctx, logging.Fields{
		"ConfigSet": string(cfgSet),
		"Location":  common.GitilesURL(loc),
	})
	logging.Infof(ctx, "Start importing configs")
	saveAttempt := func(success bool, msg string, commit *git.Commit) error {
		attempt := &model.ImportAttempt{
			ConfigSet: datastore.KeyForObj(ctx, &model.ConfigSet{ID: cfgSet}),
			Success:   success,
			Message:   msg,
			Revision: model.RevisionInfo{
				Location: &cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: proto.Clone(loc).(*cfgcommonpb.GitilesLocation),
					},
				},
			},
		}
		if commit != nil {
			attempt.Revision.ID = commit.Id
			attempt.Revision.Location.GetGitilesLocation().Ref = commit.Id
			attempt.Revision.CommitTime = commit.Committer.GetTime().AsTime()
			attempt.Revision.CommitterEmail = commit.Committer.GetEmail()
			attempt.Revision.AuthorEmail = commit.Author.GetEmail()
		}
		return datastore.Put(ctx, attempt)
	}

	host, project, err := gitiles.ParseRepoURL(loc.Repo)
	if err != nil {
		err = errors.Fmt("invalid gitiles repo: %s: %w", loc.Repo, err)
		return ErrFatalTag.Apply(errors.Append(err, saveAttempt(false, err.Error(), nil)))
	}
	gtClient, err := clients.NewGitilesClient(ctx, host, cfgSet.Project())
	if err != nil {
		err = errors.Fmt("failed to create a gitiles client: %w", err)
		return ErrFatalTag.Apply(errors.Append(err, saveAttempt(false, err.Error(), nil)))
	}

	logRes, err := gtClient.Log(ctx, &gitilespb.LogRequest{
		Project:    project,
		Committish: loc.Ref,
		Path:       loc.Path,
		PageSize:   1,
	})
	if err != nil {
		err = errors.Fmt("cannot fetch logs from %s: %w", common.GitilesURL(loc), err)
		return errors.Append(err, saveAttempt(false, err.Error(), nil))
	}
	if logRes == nil || len(logRes.GetLog()) == 0 {
		logging.Warningf(ctx, "No commits")
		return saveAttempt(true, "no commit logs", nil)
	}
	latestCommit := logRes.Log[0]

	cfgSetInDB := &model.ConfigSet{ID: cfgSet}
	lastAttempt := &model.ImportAttempt{ConfigSet: datastore.KeyForObj(ctx, cfgSetInDB)}
	switch err := datastore.Get(ctx, cfgSetInDB, lastAttempt); {
	case errors.Contains(err, datastore.ErrNoSuchEntity): // proceed with importing
	case err != nil:
		err = errors.Fmt("failed to load config set %q or its last attempt: %w", cfgSet, err)
		return errors.Append(err, saveAttempt(false, err.Error(), latestCommit))
	case cfgSetInDB.LatestRevision.ID == latestCommit.Id &&
		proto.Equal(cfgSetInDB.Location.GetGitilesLocation(), loc) &&
		cfgSetInDB.Version == model.CurrentCfgSetVersion &&
		lastAttempt.Success && // otherwise, something wrong with lastAttempt, better to import again.
		len(lastAttempt.ValidationResult.GetMessages()) == 0: // avoid overriding lastAttempt's validationResult.
		logging.Debugf(ctx, "Already up-to-date")
		return saveAttempt(true, "Up-to-date", latestCommit)
	}

	logging.Infof(ctx, "Rolling %s => %s", cfgSetInDB.LatestRevision.ID, latestCommit.Id)
	err = i.importRevision(ctx, cfgSet, loc, latestCommit, gtClient, project)
	if err != nil {
		err = errors.Fmt("Failed to import %s revision %s: %w", cfgSet, latestCommit.Id, err)
		return errors.Append(err, saveAttempt(false, err.Error(), latestCommit))
	}
	return nil
}

// importRevision imports a referenced Gitiles revision into a config set.
// It only imports when all files are valid.
// TODO(crbug.com/1446839): send notifications for any validation errors.
func (i *Importer) importRevision(ctx context.Context, cfgSet config.Set, loc *cfgcommonpb.GitilesLocation, commit *git.Commit, gtClient gitilespb.GitilesClient, gitilesProj string) error {
	if loc == nil || commit == nil {
		return nil
	}
	res, err := gtClient.Archive(ctx, &gitilespb.ArchiveRequest{
		Project: gitilesProj,
		Ref:     commit.Id,
		Path:    loc.Path,
		Format:  gitilespb.ArchiveRequest_GZIP,
	})
	if err != nil {
		return err
	}
	rev := model.RevisionInfo{
		ID:             commit.Id,
		CommitTime:     commit.Committer.GetTime().AsTime(),
		CommitterEmail: commit.Committer.GetEmail(),
		AuthorEmail:    commit.Author.GetEmail(),
		Location: &cfgcommonpb.Location{
			Location: &cfgcommonpb.Location_GitilesLocation{
				GitilesLocation: &cfgcommonpb.GitilesLocation{
					Repo: loc.Repo,
					Ref:  commit.Id, // TODO(crbug.com/1446839): rename to committish.
					Path: loc.Path,
				},
			},
		},
	}
	attempt := &model.ImportAttempt{
		ConfigSet: datastore.MakeKey(ctx, model.ConfigSetKind, string(cfgSet)),
		Revision:  rev,
	}
	configSet := &model.ConfigSet{
		ID: cfgSet,
		Location: &cfgcommonpb.Location{
			Location: &cfgcommonpb.Location_GitilesLocation{GitilesLocation: loc},
		},
		LatestRevision: rev,
	}

	if res == nil || len(res.Contents) == 0 {
		logging.Warningf(ctx, "Configs for %s don't exist. They may be deleted", cfgSet)
		attempt.Success = true
		attempt.Message = "No Configs. Imported as empty"
		return datastore.RunInTransaction(ctx, func(c context.Context) error {
			return datastore.Put(ctx, configSet, attempt)
		}, nil)
	}

	var files []*model.File
	gzReader, err := gzip.NewReader(bytes.NewReader(res.Contents))
	if err != nil {
		return errors.Fmt("failed to ungzip gitiles archive: %w", err)
	}
	defer func() {
		// Ignore the error. Failing to close it doesn't impact Luci-config
		// recognize configs correctly, as they are already saved into data storage.
		_ = gzReader.Close()
	}()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Fmt("failed to extract gitiles archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}

		filePath := header.Name
		logging.Infof(ctx, "Processing file: %q", filePath)
		file := &model.File{
			Path:     filePath,
			Revision: datastore.MakeKey(ctx, model.ConfigSetKind, string(cfgSet), model.RevisionKind, commit.Id),
			Size:     header.Size,
			Location: &cfgcommonpb.Location{
				Location: &cfgcommonpb.Location_GitilesLocation{
					GitilesLocation: &cfgcommonpb.GitilesLocation{
						Repo: loc.Repo,
						Ref:  commit.Id,
						Path: strings.TrimPrefix(fmt.Sprintf("%s/%s", loc.Path, filePath), "/"),
					},
				},
			},
		}
		if file.ContentSHA256, file.Content, err = hashAndCompressConfig(tarReader); err != nil {
			return errors.Fmt("filepath: %q: %w", filePath, err)
		}
		gsFileName := fmt.Sprintf("%s/sha256/%s", common.GSProdCfgFolder, file.ContentSHA256)
		_, err = clients.GetGsClient(ctx).UploadIfMissing(ctx, i.GSBucket, gsFileName, file.Content, func(attrs *storage.ObjectAttrs) {
			attrs.ContentEncoding = "gzip"
		})
		if err != nil {
			return errors.Fmt("failed to upload file %s as %s: %w", filePath, gsFileName, err)
		}
		file.GcsURI = gs.MakePath(i.GSBucket, gsFileName)
		if len(file.Content) > compressedContentLimit {
			// Don't save the compressed content if the content is above the limit.
			// Since Datastore has 1MiB entity size limit.
			file.Content = nil
		}
		files = append(files, file)
	}
	if err := i.validateAndPopulateAttempt(ctx, cfgSet, files, attempt); err != nil {
		return err
	}
	if !attempt.Success {
		if author, ok := strings.CutSuffix(commit.Author.GetEmail(), "@google.com"); ok {
			metrics.RejectedCfgImportCounter.Add(ctx, 1, string(cfgSet), commit.Id, author)
		} else {
			metrics.RejectedCfgImportCounter.Add(ctx, 1, string(cfgSet), commit.Id, "")
		}
		return errors.WrapIf(datastore.Put(ctx, attempt), "saving attempt")
	}
	// The rejection event rarely happen. Add by 0 to ensure the corresponding
	// streamz counter can properly handle this metric.
	metrics.RejectedCfgImportCounter.Add(ctx, 0, string(cfgSet), "", "")

	logging.Infof(ctx, "Storing %d files, updating ConfigSet %s and ImportAttempt", len(files), cfgSet)
	now := clock.Now(ctx).UTC()
	for _, f := range files {
		f.CreateTime = now
	}
	// Datastore transaction has a maximum size of 10MB.
	if err := datastore.Put(ctx, files); err != nil {
		return errors.Fmt("failed to store files: %w", err)
	}
	return datastore.RunInTransaction(ctx, func(c context.Context) error {
		return datastore.Put(ctx, configSet, attempt)
	}, nil)
}

// hashAndCompressConfig reads the config and returns the sha256 of the config
// and the gzip-compressed bytes.
func hashAndCompressConfig(reader io.Reader) (string, []byte, error) {
	sha := sha256.New()
	compressed := &bytes.Buffer{}
	gzipWriter := gzip.NewWriter(compressed)
	multiWriter := io.MultiWriter(sha, gzipWriter)
	if _, err := io.Copy(multiWriter, reader); err != nil {
		_ = gzipWriter.Close()
		return "", nil, errors.Fmt("error reading tar file: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return "", nil, errors.Fmt("failed to close gzip writer: %w", err)
	}
	return hex.EncodeToString(sha.Sum(nil)), compressed.Bytes(), nil
}

func (i *Importer) validateAndPopulateAttempt(ctx context.Context, cfgSet config.Set, files []*model.File, attempt *model.ImportAttempt) error {
	vfs := make([]validation.File, len(files))
	for i, f := range files {
		vfs[i] = f
	}
	vr, err := i.Validator.Validate(ctx, cfgSet, vfs)
	if err != nil {
		return errors.Fmt("validating config set %q: %w", cfgSet, err)
	}
	attempt.Success = true // be optimistic
	attempt.Message = "Imported"
	attempt.ValidationResult = vr
	for _, msg := range attempt.ValidationResult.GetMessages() {
		switch sev := msg.GetSeverity(); {
		case sev >= cfgcommonpb.ValidationResult_ERROR:
			attempt.Success = false
			attempt.Message = "Invalid config"
			return nil
		case sev == cfgcommonpb.ValidationResult_WARNING:
			attempt.Message = "Imported with warnings"
		}
	}
	return nil
}

// Reimport handles the HTTP request of reimporting a single config set.
func (i *Importer) Reimport(c *router.Context) {
	ctx := c.Request.Context()
	caller := auth.CurrentIdentity(ctx)
	cs := config.Set(strings.Trim(c.Params.ByName("ConfigSet"), "/"))

	if cs == "" {
		http.Error(c.Writer, "config set is not specified", http.StatusBadRequest)
		return
	} else if err := cs.Validate(); err != nil {
		http.Error(c.Writer, fmt.Sprintf("invalid config set: %s", err), http.StatusBadRequest)
		return
	}

	switch hasPerm, err := acl.CanReimportConfigSet(ctx, cs); {
	case err != nil:
		logging.Errorf(ctx, "cannot check permission for %q: %s", caller, err)
		http.Error(c.Writer, fmt.Sprintf("cannot check permission for %q", caller), http.StatusInternalServerError)
		return
	case !hasPerm:
		logging.Infof(ctx, "%q does not have access to %s", caller, cs)
		http.Error(c.Writer, fmt.Sprintf("%q is not allowed to reimport %s", caller, cs), http.StatusForbidden)
		return
	}

	switch exists, err := datastore.Exists(ctx, &model.ConfigSet{ID: cs}); {
	case err != nil:
		logging.Errorf(ctx, "failed to check existence of %s", cs)
		http.Error(c.Writer, fmt.Sprintf("error when reimporting  %s", cs), http.StatusInternalServerError)
		return
	case !exists.All():
		logging.Infof(ctx, "config set %s doesn't exist", cs)
		http.Error(c.Writer, fmt.Sprintf("%q is not found", cs), http.StatusNotFound)
		return
	}

	if err := i.ImportConfigSet(ctx, cs); err != nil {
		logging.Errorf(ctx, "cannot re-import config set %s: %s", cs, err)
		http.Error(c.Writer, fmt.Sprintf("error when reimporting %q", cs), http.StatusInternalServerError)
		return
	}
	c.Writer.WriteHeader(http.StatusOK)
}
