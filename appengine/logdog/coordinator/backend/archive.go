// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package backend

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/julienschmidt/httprouter"
	ds "github.com/luci/gae/service/datastore"
	tq "github.com/luci/gae/service/taskqueue"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/archive"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/gcloud/gs"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"github.com/luci/luci-go/server/logdog/storage"
	"golang.org/x/net/context"
)

var (
	errArchiveFailed = errors.New("archive failed")
)

// storageBufferSize is the size, in bytes, of the LogEntry buffer that is used
// to during archival. This should be greater than the maximum LogEntry size.
const storageBufferSize = types.MaxLogEntryDataSize * 64

func createArchiveTask(ls *coordinator.LogStream) *tq.Task {
	params := map[string]string{
		"path": string(ls.Path()),
	}

	t := createTask("/archive/handle", params)
	t.Name = fmt.Sprintf("archive-%s", ls.HashID())

	return t
}

// HandleArchive is the endpoint for handling a specific log stream's archival
// request.
func (b *Backend) HandleArchive(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	errorWrapper(c, w, func() error {
		return b.handleArchiveTask(c, r)
	})
}

// handleArchiveTask implements the single-task archival workflow.
//
//   1) Load the task description.
//   2) Load the current stream state.
//   3) Perform the archival.
//   4) Post the archival information back to the Coordinator.
func (b *Backend) handleArchiveTask(c context.Context, r *http.Request) error {
	// Get the stream path from our task form. If the path is invalid, log the
	// error, but return nil since the task is junk.
	path := types.StreamPath(r.FormValue("path"))
	if err := path.Validate(); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"path":       path,
		}.Errorf(c, "Invalid stream path.")
		return nil
	}

	cfg, err := config.Load(c)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to load configuration.")
		return err
	}

	ls := coordinator.LogStreamFromPath(path)
	if err := ds.Get(c).Get(ls); err != nil {
		log.WithError(err).Errorf(c, "Failed to load stream from datastore.")
		return err
	}
	if ls.Archived() {
		log.Infof(c, "Log stream is already archived.")
		return nil
	}

	// If the log stream has a terminal index, and its Updated time is less than
	// the maximum archive delay, require this archival to be "complete" (no
	// missing LogEntry).
	//
	// If we're past maximum archive delay, settle for any (even empty) archival.
	// This is a failsafe to prevent logs from sitting in limbo forever.
	now := clock.Now(c).UTC()
	maxDelay := cfg.GetCoordinator().ArchiveDelayMax.Duration()
	complete := !now.After(ls.Updated.Add(maxDelay))
	if !complete {
		log.Fields{
			"path":             ls.Path(),
			"updatedTimestamp": ls.Updated,
			"maxDelay":         maxDelay,
		}.Warningf(c, "Log stream is past maximum archival delay. Dropping completeness requirement.")
	}

	st, err := b.s.Storage(c)
	if err != nil {
		return err
	}
	defer st.Close()

	gsClient, err := b.s.GSClient(c)
	if err != nil {
		return err
	}
	defer func() {
		if err := gsClient.Close(); err != nil {
			log.WithError(err).Warningf(c, "Failed to close Cloud Storage client.")
		}
	}()

	t := &archiveTask{
		Coordinator: cfg.Coordinator,
		st:          st,
		ls:          ls,
		gs:          gsClient,
		complete:    complete,
	}
	if err := t.archive(c); err != nil {
		log.WithError(err).Errorf(c, "Failed to perform archival operation.")
		return err
	}
	log.Fields{
		"streamURL":     t.streamURL,
		"indexURL":      t.indexURL,
		"dataURL":       t.dataURL,
		"terminalIndex": t.terminalIndex,
	}.Debugf(c, "Finished archive construction.")

	// Store the updated archival information.
	now = clock.Now(c)
	err = ds.Get(c).RunInTransaction(func(c context.Context) error {
		di := ds.Get(c)
		if err := di.Get(ls); err != nil {
			return err
		}
		if ls.Archived() {
			log.Warningf(c, "Log stream marked as archived during archival.")
			return nil
		}

		// Update archival information. Make sure this actually marks the stream as
		// archived.
		ls.Updated = now
		ls.State = coordinator.LSArchived
		ls.ArchiveStreamURL = t.streamURL
		ls.ArchiveStreamSize = t.streamSize
		ls.ArchiveIndexURL = t.indexURL
		ls.ArchiveIndexSize = t.indexSize
		ls.ArchiveDataURL = t.dataURL
		ls.ArchiveDataSize = t.dataSize
		ls.TerminalIndex = t.terminalIndex

		// Update the log stream.
		if err := ls.Put(di); err != nil {
			log.WithError(err).Errorf(c, "Failed to update log stream.")
			return err
		}

		return nil
	}, nil)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to mark log stream as archived.")
		return err
	}

	return nil
}

// archiveTask is the set of parameters for a single archival.
type archiveTask struct {
	*svcconfig.Coordinator
	st       storage.Storage
	ls       *coordinator.LogStream
	gs       gs.Client
	complete bool

	streamURL  string
	streamSize int64
	indexURL   string
	indexSize  int64
	dataURL    string
	dataSize   int64

	terminalIndex int64
}

// archiveState performs the archival operation on a stream described by a
// Coordinator State. Upon success, the State will be updated with the result
// of the archival operation.
func (t *archiveTask) archive(c context.Context) (err error) {
	var desc *logpb.LogStreamDescriptor
	desc, err = t.ls.DescriptorProto()
	if err != nil {
		return
	}

	// Generate our archival object managers.
	bext := desc.BinaryFileExt
	if bext == "" {
		bext = "bin"
	}

	path := t.ls.Path()
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
	t.streamURL = streamO.url
	t.indexURL = indexO.url
	t.dataURL = dataO.url

	log.Fields{
		"streamURL": t.streamURL,
		"indexURL":  t.indexURL,
		"dataURL":   t.dataURL,
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
		st:            t.st,
		path:          path,
		contiguous:    t.complete,
		terminalIndex: types.MessageIndex(t.ls.TerminalIndex),
		lastIndex:     -1,
	}

	m := archive.Manifest{
		Desc:             desc,
		Source:           &ss,
		LogWriter:        streamO,
		IndexWriter:      indexO,
		DataWriter:       dataO,
		StreamIndexRange: int(t.ArchiveStreamIndexRange),
		PrefixIndexRange: int(t.ArchivePrefixIndexRange),
		ByteRange:        int(t.ArchiveByteRange),

		Logger: log.Get(c),
	}
	err = archive.Archive(m)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to archive log stream.")
		return
	}

	t.terminalIndex = int64(ss.lastIndex)
	if t.ls.TerminalIndex != t.terminalIndex {
		if t.complete {
			log.Fields{
				"terminalIndex": t.ls.TerminalIndex,
				"lastIndex":     t.terminalIndex,
			}.Errorf(c, "Log stream archival stopped prior to terminal index.")
			return errArchiveFailed
		}

		if t.terminalIndex < 0 {
			// If our last log index was <0, then no logs were archived.
			log.Warningf(c, "No log entries were archived.")
		} else {
			// Update our terminal index.
			log.Fields{
				"from": t.ls.TerminalIndex,
				"to":   t.terminalIndex,
			}.Infof(c, "Updated log stream terminal index.")
		}
	}

	// Update our state with archival results.
	t.streamSize = streamO.Count()
	t.indexSize = indexO.Count()
	t.dataSize = dataO.Count()
	return
}

func (t *archiveTask) newGSObject(c context.Context, path types.StreamPath, name string) (*gsObject, error) {
	o := gsObject{
		gs:     t.gs,
		bucket: cleanGSPath(t.ArchiveGsBucket),
		path:   buildGSPath(cleanGSPath(t.ArchiveGsBasePath), cleanGSPath(string(path)), name),
	}

	// Build our GS URL. Note that since buildGSPath joins with "/", the initial
	// token, "gs:/", will become "gs://".
	o.url = buildGSPath("gs:/", cleanGSPath(o.bucket), o.path)

	var err error
	o.Writer, err = t.gs.NewWriter(o.bucket, o.path)
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

	buf       []*logpb.LogEntry
	lastIndex types.MessageIndex
}

func (s *storageSource) bufferEntries(start types.MessageIndex) error {
	bytes := 0

	req := storage.GetRequest{
		Path:  s.path,
		Index: start,
	}
	return s.st.Get(&req, func(idx types.MessageIndex, d []byte) bool {
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
	if s.contiguous {
		nidx := (s.lastIndex + 1)
		if sidx != nidx {
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

func cleanGSPath(p string) string {
	return strings.Trim(p, "/")
}

// buildGSPath constructs a Google Storage path from a set of components.
//
// If one of the components is empty, it will be ignored.
func buildGSPath(parts ...string) string {
	path := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) > 0 {
			path = append(path, p)
		}
	}
	return strings.Join(path, "/")
}
