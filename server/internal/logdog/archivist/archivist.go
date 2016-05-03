// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package archivist

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/gcloud/gs"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/parallel"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"github.com/luci/luci-go/common/tsmon/distribution"
	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/metric"
	"github.com/luci/luci-go/server/logdog/archive"
	"github.com/luci/luci-go/server/logdog/storage"
	"golang.org/x/net/context"
)

const (
	tsEntriesField = "entries"
	tsIndexField   = "index"
	tsDataField    = "data"
)

var (
	// tsCount counts the raw number of archival tasks that this instance has
	// processed, regardless of success/failure.
	tsCount = metric.NewCounter("logdog/archivist/archive/count",
		"The number of archival tasks processed.",
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
		field.String("archive"),
		field.String("stream"))

	// tsLogEntries tracks the number of log entries per individual
	// archival.
	//
	// The "stream" field is the type of log stream that is being archived.
	tsLogEntries = metric.NewCumulativeDistribution("logdog/archivist/archive/log_entries",
		"The total number of log entries per archive.",
		distribution.DefaultBucketer,
		field.String("stream"))

	// tsTotalLogEntries tracks the total number of log entries that have
	// been archived by this instance.
	//
	// The "stream" field is the type of log stream that is being archived.
	tsTotalLogEntries = metric.NewCounter("logdog/archivist/archive/total_log_entries",
		"The total number of log entries.",
		field.String("stream"))
)

// Task is a single archive task.
type Task interface {
	// UniqueID returns a task-unique value. Other tasks, and other retries of
	// this task, should (try to) not reuse this ID.
	UniqueID() string

	// Task is the archive task to execute.
	Task() *logdog.ArchiveTask

	// Consume marks that this task's processing is complete and that it should be
	// consumed. This may be called multiple times with no additional effect.
	Consume()

	// AssertLease asserts that the lease for this Task is still held.
	//
	// On failure, it will return an error. If successful, the Archivist may
	// assume that it holds the lease longer.
	AssertLease(context.Context) error
}

// Archivist is a stateless configuration capable of archiving individual log
// streams.
type Archivist struct {
	// Service is the client to use to communicate with Coordinator's Services
	// endpoint.
	Service logdog.ServicesClient

	// Storage is the archival source Storage instance.
	Storage storage.Storage
	// GSClient is the Google Storage client to for archive generation.
	GSClient gs.Client

	// GSBase is the base Google Storage path. This includes the bucket name
	// and any associated path.
	GSBase gs.Path
	// GSStagingBase is the base Google Storage path for archive staging. This
	// includes the bucket name and any associated path.
	GSStagingBase gs.Path

	// PrefixIndexRange is the maximum number of stream indexes in between index
	// entries. See archive.Manifest for more information.
	StreamIndexRange int
	// PrefixIndexRange is the maximum number of prefix indexes in between index
	// entries. See archive.Manifest for more information.
	PrefixIndexRange int
	// ByteRange is the maximum number of stream data bytes in between index
	// entries. See archive.Manifest for more information.
	ByteRange int
}

// storageBufferSize is the size, in bytes, of the LogEntry buffer that is used
// to during archival. This should be greater than the maximum LogEntry size.
const storageBufferSize = types.MaxLogEntryDataSize * 64

// ArchiveTask processes and executes a single log stream archive task.
//
// During processing, the Task's Consume method may be called to indicate that
// it should be consumed.
//
// If the supplied Context is Done, operation may terminate before completion,
// returning the Context's error.
func (a *Archivist) ArchiveTask(c context.Context, task Task) {
	err := a.archiveTaskImpl(c, task)

	failure := isFailure(err)
	log.Fields{
		log.ErrorKey: err,
		"failure":    failure,
		"project":    task.Task().Project,
		"path":       task.Task().Path,
	}.Infof(c, "Finished archive task.")

	// Add a result metric.
	tsCount.Add(c, 1, !failure)
}

// archiveTaskImpl performs the actual task archival.
//
// Its error return value is used to indicate how the archive failed. isFailure
// will be called to determine if the returned error value is a failure or a
// status error.
func (a *Archivist) archiveTaskImpl(c context.Context, task Task) error {
	at := task.Task()
	log.Fields{
		"project": at.Project,
		"path":    at.Path,
	}.Debugf(c, "Received archival task.")

	if err := types.StreamPath(at.Path).Validate(); err != nil {
		task.Consume()
		return fmt.Errorf("invalid path %q: %s", at.Path, err)
	}

	// TODO(dnj): Remove empty project exemption, make empty project invalid.
	if at.Project != "" {
		if err := config.ProjectName(at.Project).Validate(); err != nil {
			task.Consume()
			return fmt.Errorf("invalid project name %q: %s", at.Project, err)
		}
	}

	// Load the log stream's current state. If it is already archived, we will
	// return an immediate success.
	ls, err := a.Service.LoadStream(c, &logdog.LoadStreamRequest{
		Project: at.Project,
		Path:    at.Path,
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
		task.Consume()
		return statusErr(errors.New("log stream is purged"))

	case ls.State.Archived:
		log.Infof(c, "Log stream is already archived. Discarding archival request.")
		task.Consume()
		return statusErr(errors.New("log stream is archived"))

	case !bytes.Equal(ls.ArchivalKey, at.Key):
		if len(ls.ArchivalKey) == 0 {
			// The log stream is not registering as "archive pending" state.
			//
			// This can happen if the eventually-consistent datastore hasn't updated
			// its log stream state by the time this Pub/Sub task is received. In
			// this case, we will continue retrying the task until datastore registers
			// that some key is associated with it.
			log.Infof(c, "Archival request received before log stream has its key.")
			return statusErr(errors.New("premature archival request"))
		}

		// This can happen if a Pub/Sub message is dispatched during a transaction,
		// but that specific transaction failed. In this case, the Pub/Sub message
		// will have a key that doesn't match the key that was transactionally
		// encoded, and can be discarded.
		log.Fields{
			"logStreamArchivalKey": hex.EncodeToString(ls.ArchivalKey),
			"requestArchivalKey":   hex.EncodeToString(at.Key),
		}.Infof(c, "Superfluous archival request (keys do not match). Discarding.")
		task.Consume()
		return statusErr(errors.New("superfluous archival request"))

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

	// If the archival request is younger than the settle delay, kick it back to
	// retry later.
	age := ls.Age.Duration()
	if age < at.SettleDelay.Duration() {
		log.Fields{
			"age":         age,
			"settleDelay": at.SettleDelay.Duration(),
		}.Infof(c, "Log stream is younger than the settle delay. Returning task to queue.")
		return statusErr(errors.New("log stream is within settle delay"))
	}

	// Are we required to archive a complete log stream?
	if age <= at.CompletePeriod.Duration() {
		tidx := ls.State.TerminalIndex

		if tidx < 0 {
			log.Warningf(c, "Cannot archive complete stream with no terminal index.")
			return statusErr(errors.New("completeness required, but stream has no terminal index"))
		}

		// If we're requiring completeness, perform a keys-only scan of intermediate
		// storage to ensure that we have all of the records before we bother
		// streaming to storage only to find that we are missing data.
		if err := a.checkComplete(config.ProjectName(at.Project), types.StreamPath(at.Path), types.MessageIndex(tidx)); err != nil {
			return err
		}
	}

	ar := logdog.ArchiveStreamRequest{
		Project: at.Project,
		Path:    at.Path,
	}

	// Archive to staging.
	//
	// If a non-transient failure occurs here, we will report it to the Archivist
	// under the assumption that it will continue occurring.
	//
	// We will handle error creating the plan and executing the plan in the same
	// switch statement below.
	staged, err := a.makeStagedArchival(c, config.ProjectName(at.Project), types.StreamPath(at.Path), ls, task.UniqueID())
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create staged archival plan.")
	} else {
		err = staged.stage(c)
	}

	switch {
	case errors.IsTransient(err):
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
			// This needs to be non-nil, so if our acutal error has an empty string,
			// fill in a generic message.
			ar.Error = "archival error"
		}

	default:
		// In case something fails, clean up our staged archival (best effort).
		defer staged.cleanup(c)

		// Finalize the archival. First, extend our lease to confirm that we still
		// hold it.
		if err := task.AssertLease(c); err != nil {
			log.WithError(err).Errorf(c, "Failed to extend task lease before finalizing.")
			return err
		}

		// Finalize the archival.
		if err := staged.finalize(c, a.GSClient, &ar); err != nil {
			log.WithError(err).Errorf(c, "Failed to finalize archival.")
			return err
		}

		// Add metrics for this successful archival.
		streamType := staged.desc.StreamType.String()

		staged.stream.addMetrics(c, tsEntriesField, streamType)
		staged.index.addMetrics(c, tsIndexField, streamType)
		staged.data.addMetrics(c, tsDataField, streamType)

		tsLogEntries.Add(c, float64(staged.logEntryCount), streamType)
		tsTotalLogEntries.Add(c, staged.logEntryCount, streamType)
	}

	log.Fields{
		"streamURL":     ar.StreamUrl,
		"indexURL":      ar.IndexUrl,
		"dataURL":       ar.DataUrl,
		"terminalIndex": ar.TerminalIndex,
		"logEntryCount": ar.LogEntryCount,
		"hadError":      ar.Error,
		"complete":      ar.Complete(),
	}.Debugf(c, "Finished archival round. Reporting archive state.")

	// Extend the lease again to confirm that we still hold it.
	if err := task.AssertLease(c); err != nil {
		log.WithError(err).Errorf(c, "Failed to extend task lease before reporting.")
		return err
	}

	if _, err := a.Service.ArchiveStream(c, &ar); err != nil {
		log.WithError(err).Errorf(c, "Failed to report archive state.")
		return err
	}

	// Archival is complete and acknowledged by Coordinator. Consume the archival
	// task.
	task.Consume()
	return nil
}

// checkComplete performs a quick scan of intermediate storage to ensure that
// all of the log stream's records are available.
func (a *Archivist) checkComplete(project config.ProjectName, path types.StreamPath, tidx types.MessageIndex) error {
	sreq := storage.GetRequest{
		Project:  project,
		Path:     path,
		KeysOnly: true,
	}

	nextIndex := types.MessageIndex(0)
	var ierr error
	err := a.Storage.Get(sreq, func(idx types.MessageIndex, d []byte) bool {
		switch {
		case idx != nextIndex:
			ierr = statusErr(fmt.Errorf("missing log entry index %d (next %d)", nextIndex, idx))
			return false

		case idx == tidx:
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

func (a *Archivist) makeStagedArchival(c context.Context, project config.ProjectName, path types.StreamPath,
	ls *logdog.LoadStreamResponse, uid string) (*stagedArchival, error) {
	sa := stagedArchival{
		Archivist: a,
		project:   project,
		path:      path,

		terminalIndex: ls.State.TerminalIndex,
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

	bext := sa.desc.BinaryFileExt
	if bext == "" {
		bext = "bin"
	}

	// Construct our staged archival paths.
	sa.stream = a.makeStagingPaths(project, path, "logstream.entries", uid)
	sa.index = a.makeStagingPaths(project, path, "logstream.index", uid)
	sa.data = a.makeStagingPaths(project, path, fmt.Sprintf("data.%s", bext), uid)
	return &sa, nil
}

// makeStagingPaths returns a stagingPaths instance for the given path and
// file name. It incorporates a unique ID into the staging name to differentiate
// it from other staging paths for the same path/name.
func (a *Archivist) makeStagingPaths(project config.ProjectName, path types.StreamPath, name, uid string) stagingPaths {
	// TODO(dnj): This won't be necessary when empty project is invalid.
	if project == "" {
		project = "_"
	}

	return stagingPaths{
		staged: a.GSStagingBase.Concat(string(project), string(path), uid, name),
		final:  a.GSBase.Concat(string(project), string(path), name),
	}
}

type stagedArchival struct {
	*Archivist

	project config.ProjectName
	path    types.StreamPath
	desc    logpb.LogStreamDescriptor

	stream stagingPaths
	index  stagingPaths
	data   stagingPaths

	finalized     bool
	terminalIndex int64
	logEntryCount int64
}

// stage executes the archival process, archiving to the staged storage paths.
//
// If stage fails, it may return a transient error.
func (sa *stagedArchival) stage(c context.Context) (err error) {
	log.Fields{
		"streamURL": sa.stream.staged,
		"indexURL":  sa.index.staged,
		"dataURL":   sa.data.staged,
	}.Debugf(c, "Staging log stream...")

	// Group any transient errors that occur during cleanup. If we aren't
	// returning a non-transient error, return a transient "terr".
	var terr errors.MultiError
	defer func() {
		if err == nil && len(terr) > 0 {
			err = errors.WrapTransient(terr)
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
			if ierr := sa.GSClient.Delete(path); ierr != nil {
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
		w, ierr := sa.GSClient.NewWriter(p)
		if ierr != nil {
			log.Fields{
				log.ErrorKey: ierr,
				"path":       p,
			}.Errorf(c, "Failed to create writer.")
			return nil, ierr
		}
		return w, nil
	}

	var streamWriter, indexWriter, dataWriter gs.Writer
	if streamWriter, err = createWriter(sa.stream.staged); err != nil {
		return
	}
	defer closeWriter(streamWriter, sa.stream.staged)

	if indexWriter, err = createWriter(sa.index.staged); err != nil {
		return err
	}
	defer closeWriter(indexWriter, sa.index.staged)

	if dataWriter, err = createWriter(sa.data.staged); err != nil {
		return err
	}
	defer closeWriter(dataWriter, sa.data.staged)

	// Read our log entries from intermediate storage.
	ss := storageSource{
		Context:       c,
		st:            sa.Storage,
		project:       sa.project,
		path:          sa.path,
		terminalIndex: types.MessageIndex(sa.terminalIndex),
		lastIndex:     -1,
	}

	m := archive.Manifest{
		Desc:             &sa.desc,
		Source:           &ss,
		LogWriter:        streamWriter,
		IndexWriter:      indexWriter,
		DataWriter:       dataWriter,
		StreamIndexRange: sa.StreamIndexRange,
		PrefixIndexRange: sa.PrefixIndexRange,
		ByteRange:        sa.ByteRange,

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
	sa.terminalIndex = int64(ss.lastIndex)
	sa.logEntryCount = ss.logEntryCount
	sa.stream.count = streamWriter.Count()
	sa.index.count = indexWriter.Count()
	sa.data.count = dataWriter.Count()
	return
}

type stagingPaths struct {
	staged gs.Path
	final  gs.Path
	count  int64
}

func (d *stagingPaths) clearStaged() {
	d.staged = ""
}

func (d *stagingPaths) addMetrics(c context.Context, archiveField, streamField string) {
	tsSize.Add(c, float64(d.count), archiveField, streamField)
	tsTotalBytes.Add(c, d.count, archiveField, streamField)
}

func (sa *stagedArchival) finalize(c context.Context, client gs.Client, ar *logdog.ArchiveStreamRequest) error {
	err := parallel.FanOutIn(func(taskC chan<- func() error) {
		for _, d := range sa.getStagingPaths() {
			d := d

			// Don't copy zero-sized streams.
			if d.count == 0 {
				continue
			}

			taskC <- func() error {
				if err := client.Rename(d.staged, d.final); err != nil {
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

	ar.TerminalIndex = sa.terminalIndex
	ar.LogEntryCount = sa.logEntryCount
	ar.StreamUrl = string(sa.stream.final)
	ar.StreamSize = sa.stream.count
	ar.IndexUrl = string(sa.index.final)
	ar.IndexSize = sa.index.count
	ar.DataUrl = string(sa.data.final)
	ar.DataSize = sa.data.count
	return nil
}

func (sa *stagedArchival) cleanup(c context.Context) {
	for _, d := range sa.getStagingPaths() {
		if d.staged == "" {
			continue
		}

		if err := sa.GSClient.Delete(d.staged); err != nil {
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
		&sa.data,
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
