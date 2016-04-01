// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package archivist

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/gcloud/gs"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"github.com/luci/luci-go/server/logdog/archive"
	"github.com/luci/luci-go/server/logdog/storage"
	"golang.org/x/net/context"
)

// Archivist is a stateless configuration capable of archiving individual log
// streams.
type Archivist struct {
	// Service is the client to use to communicate with Coordinator's Services
	// endpoint.
	Service logdog.ServicesClient

	// Storage is the intermediate storage instance to use to pull log entries for
	// archival.
	Storage storage.Storage

	// GSClient is the Google Storage client to for archive generation.
	GSClient gs.Client

	// GSBase is the base Google Storage path. This includes the bucket name
	// and any associated path.
	GSBase gs.Path
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
func (a *Archivist) ArchiveTask(c context.Context, desc []byte) error {
	var task logdog.ArchiveTask
	if err := proto.Unmarshal(desc, &task); err != nil {
		log.WithError(err).Errorf(c, "Failed to decode archive task.")
		return err
	}
	return a.Archive(c, &task)
}

// Archive archives a single log stream. If unsuccessful, an error is returned.
//
// This error may be wrapped in errors.Transient if it is believed to have been
// caused by a transient failure.
//
// If the supplied Context is Done, operation may terminate before completion,
// returning the Context's error.
func (a *Archivist) Archive(c context.Context, t *logdog.ArchiveTask) error {
	// Load the log stream's current state. If it is already archived, we will
	// return an immediate success.
	ls, err := a.Service.LoadStream(c, &logdog.LoadStreamRequest{
		Path: t.Path,
		Desc: true,
	})
	switch {
	case err != nil:
		log.WithError(err).Errorf(c, "Failed to load log stream.")
		return err
	case ls.State == nil:
		return errors.New("missing state")
	case ls.State.ProtoVersion != logpb.Version:
		log.Fields{
			"protoVersion":    ls.State.ProtoVersion,
			"expectedVersion": logpb.Version,
		}.Errorf(c, "Unsupported log stream protobuf version.")
		return errors.New("unsupported protobuf version")
	case ls.Desc == nil:
		return errors.New("missing descriptor")

	case ls.State.Purged:
		log.Warningf(c, "Log stream is purged.")
		return nil
	case ls.State.Archived:
		log.Infof(c, "Log stream is already archived.")
		return nil
	}

	// Deserialize and validate the descriptor protobuf.
	var desc logpb.LogStreamDescriptor
	if err := proto.Unmarshal(ls.Desc, &desc); err != nil {
		log.Fields{
			log.ErrorKey:   err,
			"protoVersion": ls.State.ProtoVersion,
		}.Errorf(c, "Failed to unmarshal descriptor protobuf.")
		return err
	}

	task := &archiveTask{
		Archivist:   a,
		ArchiveTask: t,
		ls:          ls,
		desc:        &desc,
	}
	if err := task.archive(c); err != nil {
		log.WithError(err).Errorf(c, "Failed to perform archival operation.")
		return err
	}
	log.Fields{
		"streamURL":     task.ar.StreamUrl,
		"indexURL":      task.ar.IndexUrl,
		"dataURL":       task.ar.DataUrl,
		"terminalIndex": task.ar.TerminalIndex,
		"complete":      task.ar.Complete,
	}.Debugf(c, "Finished archive construction.")

	if _, err := a.Service.ArchiveStream(c, &task.ar); err != nil {
		log.WithError(err).Errorf(c, "Failed to mark log stream as archived.")
		return err
	}
	return nil
}

// archiveTask is the set of parameters for a single archival.
type archiveTask struct {
	*Archivist
	*logdog.ArchiveTask

	// ls is the log stream state.
	ls *logdog.LoadStreamResponse
	// desc is the unmarshaled log stream descriptor.
	desc *logpb.LogStreamDescriptor

	// ar will be populated during archive construction.
	ar logdog.ArchiveStreamRequest
}

// archiveState performs the archival operation on a stream described by a
// Coordinator State. Upon success, the State will be updated with the result
// of the archival operation.
func (t *archiveTask) archive(c context.Context) (err error) {
	// Generate our archival object managers.
	bext := t.desc.BinaryFileExt
	if bext == "" {
		bext = "bin"
	}

	path := t.Path
	var streamO, indexO, dataO *gsObject
	streamO, err = t.newGSObject(c, path, "logstream.entries")
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create log object.")
		return
	}

	indexO, err = t.newGSObject(c, path, "logstream.index")
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create index object.")
		return
	}

	dataO, err = t.newGSObject(c, path, fmt.Sprintf("data.%s", bext))
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create data object.")
		return
	}

	// Load the URLs into our state.
	t.ar.StreamUrl = streamO.url
	t.ar.IndexUrl = indexO.url
	t.ar.DataUrl = dataO.url

	log.Fields{
		"streamURL": t.ar.StreamUrl,
		"indexURL":  t.ar.IndexUrl,
		"dataURL":   t.ar.DataUrl,
	}.Infof(c, "Archiving log stream...")

	// We want to try and delete any GS objects that were created during a failed
	// archival attempt.
	deleteOnFail := func(o *gsObject) {
		if o == nil || err == nil {
			return
		}
		if ierr := o.delete(); ierr != nil {
			log.Fields{
				log.ErrorKey: ierr,
				"url":        o.url,
			}.Warningf(c, "Failed to clean-up GS object on failure.")
		}
	}
	defer deleteOnFail(streamO)
	defer deleteOnFail(indexO)
	defer deleteOnFail(dataO)

	// Close our GS object managers on exit. If any of them fail to close, marh
	// the archival as a failure.
	closeOM := func(o *gsObject) {
		if o == nil {
			return
		}
		if ierr := o.Close(); ierr != nil {
			err = ierr
		}
	}
	defer closeOM(streamO)
	defer closeOM(indexO)
	defer closeOM(dataO)

	// Read our log entries from intermediate storage.
	ss := storageSource{
		Context:       c,
		st:            t.Storage,
		path:          types.StreamPath(t.Path),
		contiguous:    t.Complete,
		terminalIndex: types.MessageIndex(t.ls.State.TerminalIndex),
		lastIndex:     -1,
	}

	m := archive.Manifest{
		Desc:             t.desc,
		Source:           &ss,
		LogWriter:        streamO,
		IndexWriter:      indexO,
		DataWriter:       dataO,
		StreamIndexRange: t.StreamIndexRange,
		PrefixIndexRange: t.PrefixIndexRange,
		ByteRange:        t.ByteRange,

		Logger: log.Get(c),
	}
	err = archive.Archive(m)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to archive log stream.")
		return
	}

	t.ar.TerminalIndex = int64(ss.lastIndex)
	if tidx := t.ls.State.TerminalIndex; tidx != t.ar.TerminalIndex {
		// Fail, if we were requested to archive only the complete log.
		if t.Complete {
			log.Fields{
				"terminalIndex": tidx,
				"lastIndex":     t.ar.TerminalIndex,
			}.Errorf(c, "Log stream archival stopped prior to terminal index.")
			return errors.New("stream finished short of terminal index")
		}

		if t.ar.TerminalIndex < 0 {
			// If our last log index was <0, then no logs were archived.
			log.Warningf(c, "No log entries were archived.")
		} else {
			// Update our terminal index.
			log.Fields{
				"from": tidx,
				"to":   t.ar.TerminalIndex,
			}.Infof(c, "Updated log stream terminal index.")
		}
	}

	// Update our state with archival results.
	t.ar.Path = t.Path
	t.ar.StreamSize = streamO.Count()
	t.ar.IndexSize = indexO.Count()
	t.ar.DataSize = dataO.Count()
	t.ar.Complete = !ss.hasMissingEntries
	return
}

func (t *archiveTask) newGSObject(c context.Context, path string, name string) (*gsObject, error) {
	p := t.GSBase.Concat(path, name)
	o := gsObject{
		gs:     t.GSClient,
		bucket: p.Bucket(),
		path:   p.Filename(),
	}

	// Build our GS URL. Note that since buildGSPath joins with "/", the initial
	// token, "gs:/", will become "gs://".
	o.url = string(p)

	var err error
	o.Writer, err = t.GSClient.NewWriter(o.bucket, o.path)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"url":        o.url,
		}.Errorf(c, "Failed to create Writer.")
		return nil, err
	}

	// Delete any existing object at this path.
	if err := o.delete(); err != nil {
		closeErr := o.Close()

		log.Fields{
			log.ErrorKey: err,
			"closeErr":   closeErr,
			"url":        o.url,
		}.Errorf(c, "Could not delete object during creation.")
		return nil, err
	}
	return &o, nil
}

// gsObjectManger wraps a gsObject instance with metadata.
type gsObject struct {
	gs.Writer

	// gs is the Client instance.
	gs gs.Client
	// bucket is the name of the object's bucket.
	bucket string
	// path is the bucket-relative path of the object.
	path string
	// url is the Google Storage URL (gs://) of this object.
	url string
}

func (o *gsObject) delete() error {
	return o.gs.Delete(o.bucket, o.path)
}

// storageSource is an archive.LogEntrySource that pulls log entries from
// intermediate storage via its storage.Storage instance.
type storageSource struct {
	context.Context

	st            storage.Storage    // the storage instance to read from
	path          types.StreamPath   // the path of the log stream
	contiguous    bool               // if true, enforce contiguous entries
	terminalIndex types.MessageIndex // if >= 0, discard logs beyond this

	buf               []*logpb.LogEntry
	lastIndex         types.MessageIndex
	hasMissingEntries bool // true if some log entries were missing.
}

func (s *storageSource) bufferEntries(start types.MessageIndex) error {
	bytes := 0

	req := storage.GetRequest{
		Path:  s.path,
		Index: start,
	}
	return s.st.Get(req, func(idx types.MessageIndex, d []byte) bool {
		le := logpb.LogEntry{}
		if err := proto.Unmarshal(d, &le); err != nil {
			log.Fields{
				log.ErrorKey:  err,
				"streamIndex": idx,
			}.Errorf(s, "Failed to unmarshal LogEntry.")
			return false
		}
		s.buf = append(s.buf, &le)

		// Stop loading if we've reached or exceeded our buffer size.
		bytes += len(d)
		return bytes < storageBufferSize
	})
}

func (s *storageSource) NextLogEntry() (*logpb.LogEntry, error) {
	if len(s.buf) == 0 {
		s.buf = s.buf[:0]
		if err := s.bufferEntries(s.lastIndex + 1); err != nil {
			if err == storage.ErrDoesNotExist {
				log.Warningf(s, "Archive target stream does not exist in intermediate storage.")
				return nil, archive.ErrEndOfStream
			}

			log.WithError(err).Errorf(s, "Failed to retrieve log stream from storage.")
			return nil, err
		}
	}

	if len(s.buf) == 0 {
		log.Fields{
			"lastIndex": s.lastIndex,
		}.Debugf(s, "Encountered end of stream.")
		return nil, archive.ErrEndOfStream
	}

	var le *logpb.LogEntry
	le, s.buf = s.buf[0], s.buf[1:]

	// If we're enforcing a contiguous log stream, error if this LogEntry is not
	// contiguous.
	sidx := types.MessageIndex(le.StreamIndex)
	nidx := (s.lastIndex + 1)
	if sidx != nidx {
		s.hasMissingEntries = true

		if s.contiguous {
			log.Fields{
				"index":     sidx,
				"nextIndex": nidx,
			}.Errorf(s, "Non-contiguous log stream while enforcing.")
			return nil, errors.New("non-contiguous log stream")
		}
	}

	// If we're enforcing a maximum terminal index, return end of stream if this
	// LogEntry exceeds that index.
	if s.terminalIndex >= 0 && sidx > s.terminalIndex {
		log.Fields{
			"index":         sidx,
			"terminalIndex": s.terminalIndex,
		}.Warningf(s, "Discarding log entries beyond expected terminal index.")
		return nil, archive.ErrEndOfStream
	}

	s.lastIndex = sidx
	return le, nil
}
