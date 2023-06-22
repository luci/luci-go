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
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/logging"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/internal/taskpb"
)

const (
	// compressedContentLimit is the maximum allowed compressed content size to
	// store into Datastore, in order to avoid exceeding 1MiB limit per entity.
	compressedContentLimit = 800 * 1024
)

var (
	// ErrFatalTag is an error tag to indicate an unrecoverable error in the configs
	// importing flow.
	ErrFatalTag = errors.BoolTag{Key: errors.NewTagKey("A config importing unrecoverable error")}
)

func init() {
	tq.RegisterTaskClass(tq.TaskClass{
		ID:        "import-configs",
		Kind:      tq.NonTransactional,
		Prototype: (*taskpb.ImportConfigs)(nil),
		Queue:     "backend-v2",
		Handler: func(ctx context.Context, payload proto.Message) error {
			return errors.Reason("import-configs handler hasn't implemented").Err()
		},
	})
}

// ImportAllConfigs schedules a task for each service and project config set to
// import configs from Gitiles and clean up stale config sets.
func ImportAllConfigs(ctx context.Context) error {
	cfgLoc, _ := getGlobalConfigLoc()

	// Get all config sets.
	cfgSets, err := getAllServiceCfgSets(ctx, cfgLoc)
	if err != nil {
		return errors.Annotate(err, "failed to load service config sets").Err()
	}
	sets, err := getAllProjCfgSets(ctx)
	if err != nil {
		return errors.Annotate(err, "failed to load project config sets").Err()
	}
	cfgSets = append(cfgSets, sets...)

	// Enqueue tasks
	err = parallel.WorkPool(8, func(workCh chan<- func() error) {
		for _, cs := range cfgSets {
			cs := cs
			workCh <- func() error {
				err := tq.AddTask(ctx, &tq.Task{
					Payload: &taskpb.ImportConfigs{ConfigSet: cs}},
				)
				return errors.Annotate(err, "failed to enqueue ImportConfigs task for %q: %s", cs, err).Err()
			}
		}
	})
	if err != nil {
		return err
	}

	// Delete stale config sets.
	var keys []*datastore.Key
	if err := datastore.GetAll(ctx, datastore.NewQuery(model.ConfigSetKind).KeysOnly(true), &keys); err != nil {
		return errors.Annotate(err, "failed to fetch all config sets from Datastore").Err()
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
		return nil, errors.Annotate(err, "invalid gitiles repo: %s", cfgLoc.Repo).Err()
	}
	gitilesClient, err := clients.NewGitilesClient(ctx, host, "")
	if err != nil {
		return nil, errors.Annotate(err, "failed to create a gitiles client").Err()
	}

	res, err := gitilesClient.ListFiles(ctx, &gitilespb.ListFilesRequest{
		Project:    project,
		Committish: cfgLoc.Ref,
		Path:       cfgLoc.Path,
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to call Gitiles to list files").Err()
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

// getGlobalConfigLoc return the root of gitiles location where it stores all
// registered services in Luci Config.
// TODO(crbug.com/1446839): implement this func once we figure out how to port
// GlobalConfig and GlobalConfigRoot related functionality to v2.
func getGlobalConfigLoc() (*cfgcommonpb.GitilesLocation, error) {
	return &cfgcommonpb.GitilesLocation{
		Repo: "https://chrome-internal.googlesource.com/infradata/config",
		Ref:  "main",
		Path: "dev-configs",
	}, nil
}

// ImportConfigSet tries to import a config set.
// TODO(crbug.com/1446839): Optional: for ErrFatalTag errors or errors which are
// retried many times, may send notifications to Config Service owners in future
// after the notification functionality is done.
func ImportConfigSet(ctx context.Context, cfgSet config.Set) error {
	if sID := cfgSet.Service(); sID != "" {
		globalCfgLoc, err := getGlobalConfigLoc()
		if err != nil {
			return errors.Annotate(err, "errors on getting the global config location").Err()
		}
		return importConfigSet(ctx, cfgSet, &cfgcommonpb.GitilesLocation{
			Repo: globalCfgLoc.Repo,
			Ref:  globalCfgLoc.Ref,
			Path: strings.TrimPrefix(fmt.Sprintf("%s/%s", globalCfgLoc.Path, sID), "/"),
		})
	} else if pID := cfgSet.Project(); pID != "" {
		return importProject(ctx, pID)
	}
	return errors.Reason("Invalid config set: %q", cfgSet).Tag(ErrFatalTag).Err()
}

// importProject imports a project config set.
func importProject(ctx context.Context, projectID string) error {
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
		return errors.Reason("project %q not exist or has no gitiles location", projectID).Tag(ErrFatalTag).Err()
	}
	return importConfigSet(ctx, config.MustProjectSet(projectID), projLoc)
}

// importConfigSet tries to import the latest version of the given config set.
// TODO(crbug.com/1446839): Add code to report to metrics
func importConfigSet(ctx context.Context, cfgSet config.Set, loc *cfgcommonpb.GitilesLocation) error {
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
		}
		return datastore.Put(ctx, attempt)
	}

	host, project, err := gitiles.ParseRepoURL(loc.Repo)
	if err != nil {
		err = errors.Annotate(err, "invalid gitiles repo: %s", loc.Repo).Err()
		return ErrFatalTag.Apply(errors.Append(err, saveAttempt(false, err.Error(), nil)))
	}
	gtClient, err := clients.NewGitilesClient(ctx, host, cfgSet.Project())
	if err != nil {
		err = errors.Annotate(err, "failed to create a gitiles client").Err()
		return ErrFatalTag.Apply(errors.Append(err, saveAttempt(false, err.Error(), nil)))
	}

	logRes, err := gtClient.Log(ctx, &gitilespb.LogRequest{
		Project:    project,
		Committish: loc.Ref,
		Path:       loc.Path,
		PageSize:   1,
	})
	if err != nil {
		err = errors.Annotate(err, "cannot fetch logs from %s", common.GitilesURL(loc)).Err()
		return errors.Append(err, saveAttempt(false, err.Error(), nil))
	}
	if logRes == nil || len(logRes.GetLog()) == 0 {
		logging.Warningf(ctx, "No commits")
		return saveAttempt(true, "no commit logs", nil)
	}
	latestCommit := logRes.Log[0]

	cfgSetInDB := &model.ConfigSet{ID: cfgSet}
	switch err := datastore.Get(ctx, cfgSetInDB); {
	case err == datastore.ErrNoSuchEntity: // proceed with importing
	case err != nil:
		return errors.Annotate(err, "failed to load config set %q", cfgSet).Err()
	case cfgSetInDB.LatestRevision.ID == latestCommit.Id && proto.Equal(cfgSetInDB.Location.GetGitilesLocation(), loc):
		logging.Debugf(ctx, "Already up-to-date")
		return saveAttempt(true, "Up-to-date", nil)
	}

	logging.Infof(ctx, "Rolling %s => %s", cfgSetInDB.LatestRevision.ID, latestCommit.Id)
	err = importRevision(ctx, cfgSet, loc, latestCommit, gtClient, project)
	if err != nil {
		err = errors.Annotate(err, "Failed to import %s revision %s", cfgSet, latestCommit.Id).Err()
		return errors.Append(err, saveAttempt(false, err.Error(), latestCommit))
	}
	logging.Infof(ctx, "Successfully imported revision %s", latestCommit.Id)
	return nil
}

// importRevision imports a referenced Gitiles revision into a config set.
// It only imports when all files are valid.
// TODO(crbug.com/1446839): send notifications for any validation errors.
func importRevision(ctx context.Context, cfgSet config.Set, loc *cfgcommonpb.GitilesLocation, commit *git.Commit, gtClient gitilespb.GitilesClient, gitilesProj string) error {
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
		return errors.Annotate(err, "failed to ungzip gitiles archive").Err()
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
			return errors.Annotate(err, "failed to extract gitiles archive").Err()
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}

		filePath := header.Name
		logging.Infof(ctx, "Processing file: %q", filePath)
		file := &model.File{
			Path:     filePath,
			Revision: datastore.MakeKey(ctx, model.ConfigSetKind, string(cfgSet), model.RevisionKind, commit.Id),
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

		sha := sha256.New()
		compressed := &bytes.Buffer{}
		gzipWriter := gzip.NewWriter(compressed)
		multiWriter := io.MultiWriter(sha, gzipWriter)
		if _, err := io.Copy(multiWriter, tarReader); err != nil {
			_ = gzipWriter.Close()
			return errors.Annotate(err, "error reading tar file: %s", filePath).Err()
		}
		if err := gzipWriter.Close(); err != nil {
			return errors.Annotate(err, "failed to close gzip writer for file: %s", filePath).Err()
		}
		file.ContentHash = fmt.Sprintf("sha256:%s", hex.EncodeToString(sha.Sum(nil)[:]))
		if compressed.Len() < compressedContentLimit {
			file.Content = compressed.Bytes()
		} else {
			gsFileName := fmt.Sprintf("%s/%s", common.GSProdCfgFolder, file.ContentHash)
			_, err := clients.GetGsClient(ctx).UploadIf(ctx, common.BucketName(ctx), gsFileName, compressed.Bytes(), storage.Conditions{DoesNotExist: true})
			if err != nil {
				return errors.Annotate(err, "failed to upload file %s as %s", filePath, gsFileName).Err()
			}
			// TODO(crbug.com/1446839): call validateConfig func
			file.GcsURI = gs.MakePath(common.BucketName(ctx), gsFileName)
		}
		files = append(files, file)
	}

	// TODO(crbug.com/1446839): change them based on validateConfig response
	attempt.Success = true
	attempt.Message = "Imported"
	for _, f := range files {
		f.CreateTime = clock.Now(ctx).UTC()
	}
	logging.Infof(ctx, "Storing %d files, updating ConfigSet %s and ImportAttempt", len(files), cfgSet)
	// Datastore transaction has a maximum size of 10MB.
	if err := datastore.Put(ctx, files); err != nil {
		return errors.Annotate(err, "failed to store files").Err()
	}
	return datastore.RunInTransaction(ctx, func(c context.Context) error {
		return datastore.Put(ctx, configSet, attempt)
	}, nil)
}
