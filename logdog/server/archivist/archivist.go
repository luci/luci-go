// Copyright 2016 The LUCI Authors.
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

package archivist

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"regexp"

	cl "cloud.google.com/go/logging"
	"github.com/golang/protobuf/proto"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	tsmon_types "go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/config"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/archive"
	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/logdog/common/viewer"
)

const (
	tsEntriesField = "entries"
	tsIndexField   = "index"
)

var logIDRe = regexp.MustCompile(`^[[:alnum:]._\-][[:alnum:]./_\-]{0,510}`)

// CLClient is a general interface for CloudLogging client and intended to enable
// unit tests to stub out CloudLogging.
type CLClient interface {
	Close() error
	Logger(logID string, opts ...cl.LoggerOption) *cl.Logger
	Ping(context.Context) error
}

var (
	// tsCount counts the raw number of archival tasks that this instance has
	// processed, regardless of success/failure.
	tsCount = metric.NewCounter("logdog/archivist/archive/count",
		"The number of archival tasks processed.",
		nil,
		field.String("project"),
		field.Bool("successful"))

	// tsSize tracks the archive binary file size distribution of completed
	// archives.
	//
	// The "archive" field is the specific type of archive (entries, index, data)
	// that is being tracked.
	//
	// The "stream" field is the type of log stream that is being archived.
	tsSize = metric.NewCumulativeDistribution("logdog/archivist/archive/size",
		"The size (in bytes) of each archive file.",
		&tsmon_types.MetricMetadata{Units: tsmon_types.Bytes},
		distribution.DefaultBucketer,
		field.String("project"),
		field.String("archive"),
		field.String("stream"))

	// tsTotalBytes tracks the cumulative total number of bytes that have
	// been archived by this instance.
	//
	// The "archive" field is the specific type of archive (entries, index, data)
	// that is being tracked.
	//
	// The "stream" field is the type of log stream that is being archived.
	tsTotalBytes = metric.NewCounter("logdog/archivist/archive/total_bytes",
		"The total number of archived bytes.",
		&tsmon_types.MetricMetadata{Units: tsmon_types.Bytes},
		field.String("project"),
		field.String("archive"),
		field.String("stream"))

	// tsLogEntries tracks the number of log entries per individual
	// archival.
	//
	// The "stream" field is the type of log stream that is being archived.
	tsLogEntries = metric.NewCumulativeDistribution("logdog/archivist/archive/log_entries",
		"The total number of log entries per archive.",
		nil,
		distribution.DefaultBucketer,
		field.String("project"),
		field.String("stream"))

	// tsTotalLogEntries tracks the total number of log entries that have
	// been archived by this instance.
	//
	// The "stream" field is the type of log stream that is being archived.
	tsTotalLogEntries = metric.NewCounter("logdog/archivist/archive/total_log_entries",
		"The total number of log entries.",
		nil,
		field.String("project"),
		field.String("stream"))
)

// Settings defines the archival parameters for a specific archival operation.
//
// In practice, this will be formed from service and project settings.
type Settings struct {
	// GSBase is the base Google Storage path. This includes the bucket name
	// and any associated path.
	GSBase gs.Path
	// GSStagingBase is the base Google Storage path for archive staging. This
	// includes the bucket name and any associated path.
	GSStagingBase gs.Path

	// IndexStreamRange is the maximum number of stream indexes in between index
	// entries. See archive.Manifest for more information.
	IndexStreamRange int
	// IndexPrefixRange is the maximum number of prefix indexes in between index
	// entries. See archive.Manifest for more information.
	IndexPrefixRange int
	// IndexByteRange is the maximum number of stream data bytes in between index
	// entries. See archive.Manifest for more information.
	IndexByteRange int

	// CloudLoggingProjectID is the ID of the Google Cloud Platform project to export
	// logs to.
	//
	// May be empty, if no export is configured.
	CloudLoggingProjectID string
	// CloudLoggingBufferLimit is the maximum number of megabytes that the
	// Cloud Logger will keep in memory per concurrent-task before flushing them
	// out.
	CloudLoggingBufferLimit int
}

// SettingsLoader returns archival Settings for a given project.
type SettingsLoader func(ctx context.Context, project string) (*Settings, error)

// Archivist is a stateless configuration capable of archiving individual log
// streams.
type Archivist struct {
	// Service is the client to use to communicate with Coordinator's Services
	// endpoint.
	Service logdog.ServicesClient

	// SettingsLoader loads archival settings for a specific project.
	SettingsLoader SettingsLoader

	// Storage is the archival source Storage instance.
	Storage storage.Storage

	// GSClientFactory obtains a Google Storage client for archive generation.
	GSClientFactory func(ctx context.Context, luciProject string) (gs.Client, error)

	// CLClientFactory obtains a Cloud Logging client for log exports.
	// `luciProject` is the ID of the LUCI project to export logs from, and
	// `clProject` is the ID of the Google Cloud project to export logs to.
	CLClientFactory func(ctx context.Context, luciProject, clProject string, onError func(err error)) (CLClient, error)
}

// storageBufferSize is the size, in bytes, of the LogEntry buffer that is used
// to during archival. This should be greater than the maximum LogEntry size.
const storageBufferSize = types.MaxLogEntryDataSize * 64

// ArchiveTask processes and executes a single log stream archive task.
//
// If the supplied Context is Done, operation may terminate before completion,
// returning the Context's error.
func (a *Archivist) ArchiveTask(ctx context.Context, task *logdog.ArchiveTask) error {
	err := a.archiveTaskImpl(ctx, task)

	failure := isFailure(err)

	// Add a result metric.
	tsCount.Add(ctx, 1, task.Project, !failure)

	return err
}

// archiveTaskImpl performs the actual task archival.
//
// Its error return value is used to indicate how the archive failed. isFailure
// will be called to determine if the returned error value is a failure or a
// status error.
func (a *Archivist) archiveTaskImpl(ctx context.Context, task *logdog.ArchiveTask) error {
	// Validate the project name.
	if err := config.ValidateProjectName(task.Project); err != nil {
		logging.WithError(err).Errorf(ctx, "invalid project name %q: %s", task.Project)
		return nil
	}

	// Load archival settings for this project.
	settings, err := a.loadSettings(ctx, task.Project)
	switch {
	case err == config.ErrNoConfig:
		logging.WithError(err).Errorf(ctx, "The project config doesn't exist; discarding the task.")
		return nil
	case transient.Tag.In(err):
		// If this is a transient error, exit immediately and do not delete the
		// archival task.
		logging.WithError(err).Warningf(ctx, "TRANSIENT error during loading the project config.")
		return err
	case err != nil:
		// This project has bad or no archival settings, this is non-transient,
		// discard the task.
		logging.WithError(err).Errorf(ctx, "Failed to load settings for project.")
		return nil
	}

	// Load the log stream's current state. If it is already archived, we will
	// return an immediate success.
	ls, err := a.Service.LoadStream(ctx, &logdog.LoadStreamRequest{
		Project: task.Project,
		Id:      task.Id,
		Desc:    true,
	})
	switch {
	case err != nil:
		logging.WithError(err).Errorf(ctx, "Failed to load log stream.")
		return err

	case ls.State == nil:
		logging.Errorf(ctx, "Log stream did not include state.")
		return errors.New("log stream did not include state")

	case ls.State.Purged:
		logging.Warningf(ctx, "Log stream is purged. Discarding archival request.")
		a.expungeStorage(ctx, task.Project, ls.Desc, ls.State.TerminalIndex)
		return nil

	case ls.State.Archived:
		logging.Infof(ctx, "Log stream is already archived. Discarding archival request.")
		a.expungeStorage(ctx, task.Project, ls.Desc, ls.State.TerminalIndex)
		return nil

	case ls.State.ProtoVersion != logpb.Version:
		logging.Fields{
			"protoVersion":    ls.State.ProtoVersion,
			"expectedVersion": logpb.Version,
		}.Errorf(ctx, "Unsupported log stream protobuf version.")
		return errors.New("unsupported log stream protobuf version")

	case ls.Desc == nil:
		logging.Errorf(ctx, "Log stream did not include a descriptor.")
		return errors.New("log stream did not include a descriptor")
	}

	ar := logdog.ArchiveStreamRequest{
		Project: task.Project,
		Id:      task.Id,
	}

	// Build our staged archival plan. This doesn't actually do any archiving.
	staged, err := a.makeStagedArchival(ctx, task.Project, task.Realm, settings, ls)
	if err != nil {
		logging.WithError(err).Errorf(ctx, "Failed to create staged archival plan.")
		return err
	}

	// TODO(crbug.com/1164124) - handle the error from clClient.Close()
	defer staged.Close()

	// Archive to staging.
	//
	// If a non-transient failure occurs here, we will report it to the Archivist
	// under the assumption that it will continue occurring.
	//
	// We will handle error creating the plan and executing the plan in the same
	// switch statement below.
	switch err = staged.stage(); {
	case transient.Tag.In(err):
		// If this is a transient error, exit immediately and do not delete the
		// archival task.
		logging.WithError(err).Warningf(ctx, "TRANSIENT error during archival operation.")
		return err

	case err != nil:
		// This is a non-transient error, so we are confident that any future
		// Archival will also encounter this error. We will mark this archival
		// as an error and report it to the Coordinator.
		logging.WithError(err).Errorf(ctx, "Archival failed with non-transient error.")
		ar.Error = err.Error()
		if ar.Error == "" {
			// This needs to be non-nil, so if our actual error has an empty string,
			// fill in a generic message.
			ar.Error = "archival error"
		}

	default:
		// In case something fails, clean up our staged archival (best effort).
		defer staged.cleanup()

		// Finalize the archival.
		if err := staged.finalize(&ar); err != nil {
			logging.WithError(err).Errorf(ctx, "Failed to finalize archival.")
			return err
		}

		// Add metrics for this successful archival.
		streamType := staged.desc.StreamType.String()

		staged.stream.addMetrics(ctx, task.Project, tsEntriesField, streamType)
		staged.index.addMetrics(ctx, task.Project, tsIndexField, streamType)

		tsLogEntries.Add(ctx, float64(staged.logEntryCount), task.Project, streamType)
		tsTotalLogEntries.Add(ctx, staged.logEntryCount, task.Project, streamType)
	}

	if _, err := a.Service.ArchiveStream(ctx, &ar); err != nil {
		logging.WithError(err).Errorf(ctx, "Failed to report archive state.")
		return err
	}
	a.expungeStorage(ctx, task.Project, ls.Desc, ar.TerminalIndex)

	return nil
}

// expungeStorage does a best-effort expunging of the intermediate storage
// (BigTable) rows after successful archival.
//
// `desc` is a binary-encoded LogStreamDescriptor
// `terminalIndex` should be the terminal index of the archived stream. If it's
//
//	<0 (an empty stream) we skip the expunge.
func (a *Archivist) expungeStorage(ctx context.Context, project string, desc []byte, terminalIndex int64) {
	if terminalIndex < 0 {
		// no log rows
		return
	}

	if desc == nil {
		logging.Warningf(ctx, "expungeStorage: nil desc")
		return
	}

	var lsd logpb.LogStreamDescriptor
	if err := proto.Unmarshal(desc, &lsd); err != nil {
		logging.WithError(err).Warningf(ctx, "expungeStorage: decoding desc")
		return
	}

	err := a.Storage.Expunge(ctx, storage.ExpungeRequest{
		Path:    lsd.Path(),
		Project: project,
	})
	if err != nil {
		logging.WithError(err).Warningf(ctx, "expungeStorage: failed")
	}
}

// loadSettings loads and validates archival settings.
func (a *Archivist) loadSettings(ctx context.Context, project string) (*Settings, error) {
	if a.SettingsLoader == nil {
		panic("no settings loader configured")
	}

	st, err := a.SettingsLoader(ctx, project)
	switch {
	case err != nil:
		return nil, err

	case st.GSBase.Bucket() == "":
		logging.Fields{
			logging.ErrorKey: err,
			"gsBase":         st.GSBase,
		}.Errorf(ctx, "Invalid storage base.")
		return nil, errors.New("invalid storage base")

	case st.GSStagingBase.Bucket() == "":
		logging.Fields{
			logging.ErrorKey: err,
			"gsStagingBase":  st.GSStagingBase,
		}.Errorf(ctx, "Invalid storage staging base.")
		return nil, errors.New("invalid storage staging base")

	default:
		return st, nil
	}
}

func (a *Archivist) makeStagedArchival(ctx context.Context, project string, realm string,
	st *Settings, ls *logdog.LoadStreamResponse) (*stagedArchival, error) {

	gsClient, err := a.GSClientFactory(ctx, project)
	if err != nil {
		logging.Fields{
			logging.ErrorKey: err,
			"protoVersion":   ls.State.ProtoVersion,
		}.Errorf(ctx, "Failed to obtain GSClient.")
		return nil, err
	}

	sa := stagedArchival{
		Archivist: a,
		Settings:  st,

		ctx:      ctx,
		project:  project,
		realm:    realm,
		gsclient: gsClient,

		terminalIndex: types.MessageIndex(ls.State.TerminalIndex),
	}

	// Deserialize and validate the descriptor protobuf. If this fails, it is a
	// non-transient error.
	if err := proto.Unmarshal(ls.Desc, &sa.desc); err != nil {
		logging.Fields{
			logging.ErrorKey: err,
			"protoVersion":   ls.State.ProtoVersion,
		}.Errorf(ctx, "Failed to unmarshal descriptor protobuf.")
		return nil, err
	}
	sa.path = sa.desc.Path()

	// Construct staged archival paths sa.stream and sa.index. The path length
	// must not exceed 1024 bytes, it is GCS limit.
	if err = sa.makeStagingPaths(1024); err != nil {
		return nil, err
	}

	// Construct a CloudLogging client, if the config is set and the input
	// stream type is TEXT.
	if st.CloudLoggingProjectID != "" && sa.desc.StreamType == logpb.StreamType_TEXT {
		// Validate the project ID, and ping the project to verify the auth.
		if err = gcloud.ValidateProjectID(st.CloudLoggingProjectID); err != nil {
			return nil, errors.Annotate(err, "CloudLoggingProjectID %q", st.CloudLoggingProjectID).Err()
		}
		onError := func(err error) {
			logging.Fields{
				"luciProject":  project,
				"cloudProject": st.CloudLoggingProjectID,
				"path":         sa.path,
			}.Errorf(ctx, "archiving log to Cloud Logging: %v", err)
		}

		clc, err := a.CLClientFactory(ctx, project, st.CloudLoggingProjectID, onError)
		if err != nil {
			logging.Fields{
				logging.ErrorKey: err,
				"protoVersion":   ls.State.ProtoVersion,
			}.Errorf(ctx, "Failed to obtain CloudLogging client.")
			return nil, err
		}
		if err = clc.Ping(ctx); err != nil {
			return nil, errors.Annotate(
				err, "failed to ping CloudProject %q for Cloud Logging export",
				st.CloudLoggingProjectID).Err()
		}
		sa.clclient = clc
	}

	return &sa, nil
}

type stagedArchival struct {
	*Archivist
	*Settings

	ctx     context.Context
	project string
	realm   string
	path    types.StreamPath
	desc    logpb.LogStreamDescriptor

	stream stagingPaths
	index  stagingPaths

	terminalIndex types.MessageIndex
	logEntryCount int64

	gsclient gs.Client
	clclient CLClient
}

func base64Hash(p types.StreamName) string {
	hasher := sha256.New()
	hasher.Write([]byte(p))
	return base64.RawURLEncoding.EncodeToString(hasher.Sum(nil))
}

// makeStagingPaths populates `staged` and `final` fields in sa.stream and
// sa.index.
//
// It prefixes the staging GCS paths with a hash of stream's Logdog prefix to
// make sure we spread the load across GCS namespace to avoid hotspotting its
// metadata server.
//
// These paths may be shared between projects. To enforce an absence of
// conflicts, we will insert the project name as part of the path.
func (sa *stagedArchival) makeStagingPaths(maxGSFilenameLength int) error {
	// "<prefix>/+/<name>" => (<prefix>, <name>).
	prefix, name := sa.path.Split()
	if name == "" {
		return errors.Reason("got prefix-only path %q, don't know how to stage it", sa.path).Err()
	}

	// base64 encoded SHA256 hash of the prefix.
	prefixHash := "p/" + base64Hash(prefix)

	// GCS paths we need to generate are:
	//   <GSStagingBase>/<project>/<prefixHash>/+/<name>/logstream.entries
	//   <GSStagingBase>/<project>/<prefixHash>/+/<name>/logstream.index
	//   <GSBase>/<project>/<prefix>/+/<name>/logstream.entries
	//   <GSBase>/<project>/<prefix>/+/<name>/logstream.index
	//
	// Each path length must be less than maxGSFilenameLength bytes. And we want
	// <name> component in all paths to be identical. If some path doesn't fit
	// the limit, we replace <name> with "<name-prefix>-TRUNCATED-<hash>"
	// everywhere, making it fit the limit.

	// Note: len("logstream.entries") > len("logstream.index"), use it for max len.
	maxStagingLen := len(sa.GSStagingBase.Concat(sa.project, prefixHash, "+", string(name), "logstream.entries").Filename())
	maxFinalLen := len(sa.GSBase.Concat(sa.project, string(prefix), "+", string(name), "logstream.entries").Filename())

	// See if we need to truncate <name> to fit GCS paths into limits.
	//
	// The sa.path is user-provided and is unlimited. It is known to be large
	// enough to exceed max ID length (https://crbug.com/1138017).
	// So, truncate it if needed while avoiding overwrites by using crypto hash.
	maxPathLen := maxStagingLen
	if maxFinalLen > maxStagingLen {
		maxPathLen = maxFinalLen
	}
	if bytesToCut := maxPathLen - maxGSFilenameLength; bytesToCut > 0 {
		nameSuffix := types.StreamName("-TRUNCATED-" + base64Hash(name)[:16])
		// Replace last len(nameSuffix)+bytesToCut bytes with nameSuffix. It will
		// reduce the overall name size by `bytesToCut` bytes, as we need.
		if len(nameSuffix)+bytesToCut > len(name) {
			// There's no enough space even to fit nameSuffix. The prefix is too
			// huge. This should be rare, abort.
			return errors.Reason("can't stage %q of project %q, prefix is too long", sa.path, sa.project).Err()
		}
		name = name[:len(name)-len(nameSuffix)-bytesToCut] + nameSuffix
	}

	// Everything should fit into the limits now.
	nameMap := map[string]*stagingPaths{
		"logstream.entries": &sa.stream,
		"logstream.index":   &sa.index,
	}
	for file, spaths := range nameMap {
		spaths.staged = sa.GSStagingBase.Concat(sa.project, prefixHash, "+", string(name), file)
		spaths.final = sa.GSBase.Concat(sa.project, string(prefix), "+", string(name), file)
	}
	return nil
}

// stage executes the archival process, archiving to the staged storage paths.
//
// If stage fails, it may return a transient error.
func (sa *stagedArchival) stage() (err error) {
	// Group any transient errors that occur during cleanup. If we aren't
	// returning a non-transient error, return a transient "terr".
	var terr errors.MultiError
	defer func() {
		if err == nil && len(terr) > 0 {
			logging.Errorf(sa.ctx, "Encountered transient errors: %s", terr)
			err = transient.Tag.Apply(terr)
		}
	}()

	// Close our writers on exit. If any of them fail to close, mark the archival
	// as a transient failure.
	closeWriter := func(closer io.Closer, path gs.Path) {
		// Close the Writer. If this results in an error, append it to our transient
		// error MultiError.
		if ierr := closer.Close(); ierr != nil {
			logging.Warningf(sa.ctx, "Error closing writer to %s: %s", path, ierr)
			terr = append(terr, ierr)
		}

		// If we have an archival error, also delete the path associated with this
		// stream. This is a non-fatal failure, since we've already hit a fatal
		// one.
		if err != nil || len(terr) > 0 {
			logging.Warningf(sa.ctx, "Cleaning up %s after error", path)
			if ierr := sa.gsclient.Delete(path); ierr != nil {
				logging.Fields{
					logging.ErrorKey: ierr,
					"path":           path,
				}.Warningf(sa.ctx, "Failed to delete stream on error.")
			}
		}
	}

	// createWriter is a shorthand function for creating a writer to a path and
	// reporting an error if it failed.
	createWriter := func(p gs.Path) (gs.Writer, error) {
		w, ierr := sa.gsclient.NewWriter(p)
		if ierr != nil {
			logging.Fields{
				logging.ErrorKey: ierr,
				"path":           p,
			}.Errorf(sa.ctx, "Failed to create writer.")
			return nil, ierr
		}
		return w, nil
	}

	var streamWriter, indexWriter gs.Writer
	if streamWriter, err = createWriter(sa.stream.staged); err != nil {
		return err
	}
	defer closeWriter(streamWriter, sa.stream.staged)

	if indexWriter, err = createWriter(sa.index.staged); err != nil {
		return err
	}
	defer closeWriter(indexWriter, sa.index.staged)

	// Read our log entries from intermediate storage.
	ss := storageSource{
		Context:       sa.ctx,
		st:            sa.Storage,
		project:       sa.project,
		path:          sa.path,
		terminalIndex: sa.terminalIndex,
		lastIndex:     -1,
	}

	m := archive.Manifest{
		LUCIProject:      sa.project,
		Desc:             &sa.desc,
		Source:           &ss,
		LogWriter:        streamWriter,
		IndexWriter:      indexWriter,
		StreamIndexRange: sa.IndexStreamRange,
		PrefixIndexRange: sa.IndexPrefixRange,
		ByteRange:        sa.IndexByteRange,

		Logger: logging.Get(sa.ctx),
	}

	if sa.clclient != nil {
		logID := "luci-logs"
		tags := sa.desc.GetTags()
		if tags == nil {
			tags = map[string]string{}
		}
		if sa.realm != "" {
			tags["realm"] = sa.realm
		}

		// bbagent adds viewer.LogDogViewerURLTag to log streams for
		// "back to build" link in UI
		//
		// This URL isn't useful in Cloud Logging UI, and doesn't add any value
		// to search capabilities. So, remove it.
		delete(tags, viewer.LogDogViewerURLTag)

		switch val, ok := tags["luci.CloudLogExportID"]; {
		case !ok, len(val) == 0: // skip

		// len(LogID) must be < 512, and allows ./_- and alphanumerics.
		// If CloudLogExportID is too long or contains unsupported chars, fall back to
		// the default LogID.
		case len(val) > 511:
			logging.Errorf(sa.ctx, "CloudLogExportID: too long - %d", len(val))

		case !logIDRe.MatchString(val):
			logging.Errorf(sa.ctx, "CloudLogExportID(%s): does not match %s", val, logIDRe)

		default:
			logID = val
		}

		m.CloudLogger = sa.clclient.Logger(
			logID,
			cl.CommonLabels(tags),
			cl.CommonResource(&mrpb.MonitoredResource{
				Type: "generic_task",
				Labels: map[string]string{
					"project_id": sa.project,
					"location":   sa.desc.GetName(),
					"namespace":  sa.desc.GetPrefix(),
					"job":        "cloud-logging-export",
				},
			}),
			cl.BufferedByteLimit(sa.CloudLoggingBufferLimit*1024*1024),
		)
	}

	if err = archive.Archive(m); err != nil {
		logging.WithError(err).Errorf(sa.ctx, "Failed to archive log stream.")
		return err
	}

	if ss.logEntryCount == 0 {
		// If our last log index was <0, then no logs were archived.
		logging.Warningf(sa.ctx, "No log entries were archived.")
	}

	// Update our state with archival results.
	sa.terminalIndex = ss.lastIndex
	sa.logEntryCount = ss.logEntryCount
	sa.stream.bytesWritten = streamWriter.Count()
	sa.index.bytesWritten = indexWriter.Count()
	return nil
}

type stagingPaths struct {
	staged       gs.Path
	final        gs.Path
	bytesWritten int64
}

func (d *stagingPaths) clearStaged() { d.staged = "" }

func (d *stagingPaths) enabled() bool { return d.final != "" }

func (d *stagingPaths) addMetrics(ctx context.Context, projectField, archiveField, streamField string) {
	tsSize.Add(ctx, float64(d.bytesWritten), projectField, archiveField, streamField)
	tsTotalBytes.Add(ctx, d.bytesWritten, projectField, archiveField, streamField)
}

func (sa *stagedArchival) finalize(ar *logdog.ArchiveStreamRequest) error {
	err := parallel.FanOutIn(func(taskC chan<- func() error) {
		for _, d := range sa.getStagingPaths() {
			// Don't finalize zero-sized streams.
			if !d.enabled() || d.bytesWritten == 0 {
				continue
			}

			taskC <- func() error {
				if err := sa.gsclient.Rename(d.staged, d.final); err != nil {
					logging.Fields{
						logging.ErrorKey: err,
						"stagedPath":     d.staged,
						"finalPath":      d.final,
					}.Errorf(sa.ctx, "Failed to rename GS object.")
					return err
				}

				// Clear the staged value to indicate that it no longer exists.
				d.clearStaged()
				return nil
			}
		}
	})
	if err != nil {
		return err
	}

	ar.TerminalIndex = int64(sa.terminalIndex)
	ar.LogEntryCount = sa.logEntryCount
	ar.StreamUrl = string(sa.stream.final)
	ar.StreamSize = sa.stream.bytesWritten
	ar.IndexUrl = string(sa.index.final)
	ar.IndexSize = sa.index.bytesWritten
	return nil
}

func (sa *stagedArchival) Close() error {
	var clErr error
	if sa.clclient != nil {
		clErr = errors.WrapIf(sa.clclient.Close(),
			"while closing CloudLogging client for (%s/%s/+/%s)",
			sa.project, sa.desc.GetPrefix(), sa.desc.GetName())
	}
	return errors.Flatten(errors.MultiError{sa.gsclient.Close(), clErr})
}

func (sa *stagedArchival) cleanup() {
	for _, d := range sa.getStagingPaths() {
		if d.staged == "" {
			continue
		}

		logging.Warningf(sa.ctx, "Cleaning up staged path %s", d.staged)
		if err := sa.gsclient.Delete(d.staged); err != nil {
			logging.Fields{
				logging.ErrorKey: err,
				"path":           d.staged,
			}.Warningf(sa.ctx, "Failed to clean up staged path.")
		}

		d.clearStaged()
	}
}

func (sa *stagedArchival) getStagingPaths() []*stagingPaths {
	return []*stagingPaths{
		&sa.stream,
		&sa.index,
	}
}

// statusErrorWrapper is an error wrapper. It is detected by IsFailure and used to
// determine whether the supplied error represents a failure or just a status
// error.
type statusErrorWrapper struct {
	inner error
}

var _ interface {
	error
	errors.Wrapped
} = (*statusErrorWrapper)(nil)

func (e *statusErrorWrapper) Error() string {
	if e.inner != nil {
		return e.inner.Error()
	}
	return ""
}

func (e *statusErrorWrapper) Unwrap() error {
	return e.inner
}

func isFailure(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*statusErrorWrapper)
	return !ok
}
