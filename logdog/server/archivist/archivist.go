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
	"fmt"
	"io"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	log "go.chromium.org/luci/common/logging"
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
)

const (
	tsEntriesField = "entries"
	tsIndexField   = "index"
)

var (
	// tsCount counts the raw number of archival tasks that this instance has
	// processed, regardless of success/failure.
	tsCount = metric.NewCounter("logdog/archivist/archive/count",
		"The number of archival tasks processed.",
		nil,
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
		field.String("stream"))

	// tsTotalLogEntries tracks the total number of log entries that have
	// been archived by this instance.
	//
	// The "stream" field is the type of log stream that is being archived.
	tsTotalLogEntries = metric.NewCounter("logdog/archivist/archive/total_log_entries",
		"The total number of log entries.",
		nil,
		field.String("stream"))
)

// Settings defines the archival parameters for a specific archival operation.
//
// In practice, this will be formed from service and project settings.
type Settings struct {
	// GSBase is the base Google Storage path. This includes the bucket name
	// and any associated path.
	//
	// This must be unique to this archival project. In practice, it will be
	// composed of the project's archival bucket and project ID.
	GSBase gs.Path
	// GSStagingBase is the base Google Storage path for archive staging. This
	// includes the bucket name and any associated path.
	//
	// This must be unique to this archival project. In practice, it will be
	// composed of the project's staging archival bucket and project ID.
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
	GSClientFactory func(ctx context.Context, project string) (gs.Client, error)
}

// storageBufferSize is the size, in bytes, of the LogEntry buffer that is used
// to during archival. This should be greater than the maximum LogEntry size.
const storageBufferSize = types.MaxLogEntryDataSize * 64

// ArchiveTask processes and executes a single log stream archive task.
//
// If the supplied Context is Done, operation may terminate before completion,
// returning the Context's error.
func (a *Archivist) ArchiveTask(c context.Context, task *logdog.ArchiveTask) error {
	c = log.SetFields(c, log.Fields{
		"project": task.Project,
		"id":      task.Id,
	})
	log.Debugf(c, "Received archival task.")

	err := a.archiveTaskImpl(c, task)

	failure := isFailure(err)
	log.Fields{
		log.ErrorKey: err,
		"failure":    failure,
	}.Infof(c, "Finished archive task.")

	// Add a result metric.
	tsCount.Add(c, 1, !failure)

	return err
}

// archiveTaskImpl performs the actual task archival.
//
// Its error return value is used to indicate how the archive failed. isFailure
// will be called to determine if the returned error value is a failure or a
// status error.
func (a *Archivist) archiveTaskImpl(c context.Context, task *logdog.ArchiveTask) error {
	// Validate the project name.
	if err := config.ValidateProjectName(task.Project); err != nil {
		log.WithError(err).Errorf(c, "invalid project name %q: %s", task.Project)
		return nil
	}

	// Load the log stream's current state. If it is already archived, we will
	// return an immediate success.
	ls, err := a.Service.LoadStream(c, &logdog.LoadStreamRequest{
		Project: task.Project,
		Id:      task.Id,
		Desc:    true,
	})
	switch {
	case err != nil:
		log.WithError(err).Errorf(c, "Failed to load log stream.")
		return err

	case ls.State == nil:
		log.Errorf(c, "Log stream did not include state.")
		return errors.New("log stream did not include state")

	case ls.State.Purged:
		log.Warningf(c, "Log stream is purged. Discarding archival request.")
		a.expungeStorage(c, task.Project, ls.Desc, ls.State.TerminalIndex)
		return nil

	case ls.State.Archived:
		log.Infof(c, "Log stream is already archived. Discarding archival request.")
		a.expungeStorage(c, task.Project, ls.Desc, ls.State.TerminalIndex)
		return nil

	case ls.State.ProtoVersion != logpb.Version:
		log.Fields{
			"protoVersion":    ls.State.ProtoVersion,
			"expectedVersion": logpb.Version,
		}.Errorf(c, "Unsupported log stream protobuf version.")
		return errors.New("unsupported log stream protobuf version")

	case ls.Desc == nil:
		log.Errorf(c, "Log stream did not include a descriptor.")
		return errors.New("log stream did not include a descriptor")
	}

	// Load archival settings for this project.
	settings, err := a.loadSettings(c, task.Project)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"project":    task.Project,
		}.Errorf(c, "Failed to load settings for project.")
		// This project has bad or no archival settings, this is non-transient, discard the task.
		return nil
	}

	ar := logdog.ArchiveStreamRequest{
		Project: task.Project,
		Id:      task.Id,
	}

	// Build our staged archival plan. This doesn't actually do any archiving.
	uid := fmt.Sprintf("%d", mathrand.Int63(c))
	staged, err := a.makeStagedArchival(c, task.Project, settings, ls, uid)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create staged archival plan.")
		return err
	}
	defer staged.Close()

	// Archive to staging.
	//
	// If a non-transient failure occurs here, we will report it to the Archivist
	// under the assumption that it will continue occurring.
	//
	// We will handle error creating the plan and executing the plan in the same
	// switch statement below.
	switch err = staged.stage(c); {
	case transient.Tag.In(err):
		// If this is a transient error, exit immediately and do not delete the
		// archival task.
		log.WithError(err).Warningf(c, "TRANSIENT error during archival operation.")
		return err

	case err != nil:
		// This is a non-transient error, so we are confident that any future
		// Archival will also encounter this error. We will mark this archival
		// as an error and report it to the Coordinator.
		log.WithError(err).Errorf(c, "Archival failed with non-transient error.")
		ar.Error = err.Error()
		if ar.Error == "" {
			// This needs to be non-nil, so if our actual error has an empty string,
			// fill in a generic message.
			ar.Error = "archival error"
		}

	default:
		// In case something fails, clean up our staged archival (best effort).
		defer staged.cleanup(c)

		// Finalize the archival.
		if err := staged.finalize(c, &ar); err != nil {
			log.WithError(err).Errorf(c, "Failed to finalize archival.")
			return err
		}

		// Add metrics for this successful archival.
		streamType := staged.desc.StreamType.String()

		staged.stream.addMetrics(c, tsEntriesField, streamType)
		staged.index.addMetrics(c, tsIndexField, streamType)

		tsLogEntries.Add(c, float64(staged.logEntryCount), streamType)
		tsTotalLogEntries.Add(c, staged.logEntryCount, streamType)
	}

	log.Fields{
		"streamURL":     ar.StreamUrl,
		"indexURL":      ar.IndexUrl,
		"terminalIndex": ar.TerminalIndex,
		"logEntryCount": ar.LogEntryCount,
		"hadError":      ar.Error,
		"complete":      ar.Complete(),
	}.Debugf(c, "Finished archival round. Reporting archive state.")

	if _, err := a.Service.ArchiveStream(c, &ar); err != nil {
		log.WithError(err).Errorf(c, "Failed to report archive state.")
		return err
	}
	a.expungeStorage(c, task.Project, ls.Desc, ar.TerminalIndex)

	return nil
}

// expungeStorage does a best-effort expunging of the intermediate storage
// (BigTable) rows after successful archival.
//
// `desc` is a binary-encoded LogStreamDescriptor
// `terminalIndex` should be the terminal index of the archived stream. If it's
//   <0 (an empty stream) we skip the expunge.
func (a *Archivist) expungeStorage(c context.Context, project string, desc []byte, terminalIndex int64) {
	if terminalIndex < 0 {
		// no log rows
		return
	}

	if desc == nil {
		log.Warningf(c, "expungeStorage: nil desc")
		return
	}

	var lsd logpb.LogStreamDescriptor
	if err := proto.Unmarshal(desc, &lsd); err != nil {
		log.WithError(err).Warningf(c, "expungeStorage: decoding desc")
		return
	}

	err := a.Storage.Expunge(c, storage.ExpungeRequest{
		Path:    lsd.Path(),
		Project: project,
	})
	if err != nil {
		log.WithError(err).Warningf(c, "expungeStorage: failed")
	}
}

// loadSettings loads and validates archival settings.
func (a *Archivist) loadSettings(c context.Context, project string) (*Settings, error) {
	if a.SettingsLoader == nil {
		panic("no settings loader configured")
	}

	st, err := a.SettingsLoader(c, project)
	switch {
	case err != nil:
		return nil, err

	case st.GSBase.Bucket() == "":
		log.Fields{
			log.ErrorKey: err,
			"gsBase":     st.GSBase,
		}.Errorf(c, "Invalid storage base.")
		return nil, errors.New("invalid storage base")

	case st.GSStagingBase.Bucket() == "":
		log.Fields{
			log.ErrorKey:    err,
			"gsStagingBase": st.GSStagingBase,
		}.Errorf(c, "Invalid storage staging base.")
		return nil, errors.New("invalid storage staging base")

	default:
		return st, nil
	}
}

func (a *Archivist) makeStagedArchival(c context.Context, project string,
	st *Settings, ls *logdog.LoadStreamResponse, uid string) (*stagedArchival, error) {

	gsClient, err := a.GSClientFactory(c, project)
	if err != nil {
		log.Fields{
			log.ErrorKey:   err,
			"protoVersion": ls.State.ProtoVersion,
		}.Errorf(c, "Failed to obtain GSClient.")
		return nil, err
	}

	sa := stagedArchival{
		Archivist: a,
		Settings:  st,
		project:   project,
		gsclient:  gsClient,

		terminalIndex: types.MessageIndex(ls.State.TerminalIndex),
	}

	// Deserialize and validate the descriptor protobuf. If this fails, it is a
	// non-transient error.
	if err := proto.Unmarshal(ls.Desc, &sa.desc); err != nil {
		log.Fields{
			log.ErrorKey:   err,
			"protoVersion": ls.State.ProtoVersion,
		}.Errorf(c, "Failed to unmarshal descriptor protobuf.")
		return nil, err
	}
	sa.path = sa.desc.Path()

	// Construct our staged archival paths.
	sa.stream = sa.makeStagingPaths("logstream.entries", uid)
	sa.index = sa.makeStagingPaths("logstream.index", uid)

	return &sa, nil
}

type stagedArchival struct {
	*Archivist
	*Settings

	project string
	path    types.StreamPath
	desc    logpb.LogStreamDescriptor

	stream stagingPaths
	index  stagingPaths

	finalized     bool
	terminalIndex types.MessageIndex
	logEntryCount int64

	gsclient gs.Client
}

// makeStagingPaths returns a stagingPaths instance for the given path and
// file name. It incorporates a unique ID into the staging name to differentiate
// it from other staging paths for the same path/name.
func (sa *stagedArchival) makeStagingPaths(name, uid string) stagingPaths {
	// Either of these paths may be shared between projects. To enforce
	// an absence of conflicts, we will insert the project name as part of the
	// path.
	return stagingPaths{
		staged: sa.GSStagingBase.Concat(sa.project, string(sa.path), uid, name),
		final:  sa.GSBase.Concat(sa.project, string(sa.path), name),
	}
}

// checkComplete performs a quick scan of intermediate storage to ensure that
// all of the log stream's records are available.
func (sa *stagedArchival) checkComplete(c context.Context) error {
	if sa.terminalIndex < 0 {
		log.Warningf(c, "Cannot archive complete stream with no terminal index.")
		return statusErr(errors.New("completeness required, but stream has no terminal index"))
	}

	sreq := storage.GetRequest{
		Project:  sa.project,
		Path:     sa.path,
		KeysOnly: true,
	}

	nextIndex := types.MessageIndex(0)
	var ierr error
	err := sa.Storage.Get(c, sreq, func(e *storage.Entry) bool {
		idx, err := e.GetStreamIndex()
		if err != nil {
			ierr = errors.Annotate(err, "could not get stream index").Err()
			return false
		}

		switch {
		case idx != nextIndex:
			ierr = fmt.Errorf("missing log entry index %d (next %d)", nextIndex, idx)
			return false

		case idx == sa.terminalIndex:
			// We have hit our terminal index, so all of the log data is here!
			return false

		default:
			nextIndex++
			return true
		}
	})
	if ierr != nil {
		return ierr
	}
	if err != nil {
		return err
	}
	return nil
}

// stage executes the archival process, archiving to the staged storage paths.
//
// If stage fails, it may return a transient error.
func (sa *stagedArchival) stage(c context.Context) (err error) {
	log.Fields{
		"streamURL": sa.stream.staged,
		"indexURL":  sa.index.staged,
	}.Debugf(c, "Staging log stream...")

	// Group any transient errors that occur during cleanup. If we aren't
	// returning a non-transient error, return a transient "terr".
	var terr errors.MultiError
	defer func() {
		if err == nil && len(terr) > 0 {
			err = transient.Tag.Apply(terr)
		}
	}()

	// Close our writers on exit. If any of them fail to close, mark the archival
	// as a transient failure.
	closeWriter := func(closer io.Closer, path gs.Path) {
		// Close the Writer. If this results in an error, append it to our transient
		// error MultiError.
		if ierr := closer.Close(); ierr != nil {
			terr = append(terr, ierr)
		}

		// If we have an archival error, also delete the path associated with this
		// stream. This is a non-fatal failure, since we've already hit a fatal
		// one.
		if err != nil || len(terr) > 0 {
			if ierr := sa.gsclient.Delete(path); ierr != nil {
				log.Fields{
					log.ErrorKey: ierr,
					"path":       path,
				}.Warningf(c, "Failed to delete stream on error.")
			}
		}
	}

	// createWriter is a shorthand function for creating a writer to a path and
	// reporting an error if it failed.
	createWriter := func(p gs.Path) (gs.Writer, error) {
		w, ierr := sa.gsclient.NewWriter(p)
		if ierr != nil {
			log.Fields{
				log.ErrorKey: ierr,
				"path":       p,
			}.Errorf(c, "Failed to create writer.")
			return nil, ierr
		}
		return w, nil
	}

	var streamWriter, indexWriter gs.Writer
	if streamWriter, err = createWriter(sa.stream.staged); err != nil {
		return
	}
	defer closeWriter(streamWriter, sa.stream.staged)

	if indexWriter, err = createWriter(sa.index.staged); err != nil {
		return err
	}
	defer closeWriter(indexWriter, sa.index.staged)

	// Read our log entries from intermediate storage.
	ss := storageSource{
		Context:       c,
		st:            sa.Storage,
		project:       sa.project,
		path:          sa.path,
		terminalIndex: sa.terminalIndex,
		lastIndex:     -1,
	}

	m := archive.Manifest{
		Desc:             &sa.desc,
		Source:           &ss,
		LogWriter:        streamWriter,
		IndexWriter:      indexWriter,
		StreamIndexRange: sa.IndexStreamRange,
		PrefixIndexRange: sa.IndexPrefixRange,
		ByteRange:        sa.IndexByteRange,

		Logger: log.Get(c),
	}
	if err = archive.Archive(m); err != nil {
		log.WithError(err).Errorf(c, "Failed to archive log stream.")
		return
	}

	switch {
	case ss.logEntryCount == 0:
		// If our last log index was <0, then no logs were archived.
		log.Warningf(c, "No log entries were archived.")

	default:
		// Update our terminal index.
		log.Fields{
			"terminalIndex": ss.lastIndex,
			"logEntryCount": ss.logEntryCount,
		}.Debugf(c, "Finished archiving log stream.")
	}

	// Update our state with archival results.
	sa.terminalIndex = ss.lastIndex
	sa.logEntryCount = ss.logEntryCount
	sa.stream.bytesWritten = streamWriter.Count()
	sa.index.bytesWritten = indexWriter.Count()
	return
}

type stagingPaths struct {
	staged       gs.Path
	final        gs.Path
	bytesWritten int64
}

func (d *stagingPaths) clearStaged() { d.staged = "" }

func (d *stagingPaths) enabled() bool { return d.final != "" }

func (d *stagingPaths) addMetrics(c context.Context, archiveField, streamField string) {
	tsSize.Add(c, float64(d.bytesWritten), archiveField, streamField)
	tsTotalBytes.Add(c, d.bytesWritten, archiveField, streamField)
}

func (sa *stagedArchival) finalize(c context.Context, ar *logdog.ArchiveStreamRequest) error {
	err := parallel.FanOutIn(func(taskC chan<- func() error) {
		for _, d := range sa.getStagingPaths() {
			d := d

			// Don't finalize zero-sized streams.
			if !d.enabled() || d.bytesWritten == 0 {
				continue
			}

			taskC <- func() error {
				if err := sa.gsclient.Rename(d.staged, d.final); err != nil {
					log.Fields{
						log.ErrorKey: err,
						"stagedPath": d.staged,
						"finalPath":  d.final,
					}.Errorf(c, "Failed to rename GS object.")
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
	return sa.gsclient.Close()
}

func (sa *stagedArchival) cleanup(c context.Context) {
	for _, d := range sa.getStagingPaths() {
		if d.staged == "" {
			continue
		}

		if err := sa.gsclient.Delete(d.staged); err != nil {
			log.Fields{
				log.ErrorKey: err,
				"path":       d.staged,
			}.Warningf(c, "Failed to clean up staged path.")
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

func statusErr(inner error) error {
	return &statusErrorWrapper{inner}
}

func (e *statusErrorWrapper) Error() string {
	if e.inner != nil {
		return e.inner.Error()
	}
	return ""
}

func (e *statusErrorWrapper) InnerError() error {
	return e.inner
}

func isFailure(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*statusErrorWrapper)
	return !ok
}
