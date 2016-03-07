// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package archive implements a storage.Storage instance that retrieves logs
// from a Google Storage archive.
//
// This is a special implementation of storage.Storage, and does not fully
// conform to the API expecations. Namely:
//	- It is read-only. Mutation methods will return storage.ErrReadOnly.
//	- Storage methods ignore the supplied Path argument, instead opting for
//	  the archive configured in its Options.
package archive

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"sort"
	"strings"
	"sync"

	"golang.org/x/net/context"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/gcloud/gs"
	"github.com/luci/luci-go/common/iotools"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"github.com/luci/luci-go/common/recordio"
	"github.com/luci/luci-go/server/logdog/storage"
)

const (
	// maxStreamRecordSize is the maximum record size we're willing to read from
	// our archived log stream. This will help prevent out-of-memory errors if the
	// arhived log stream is malicious or corrupt.
	maxStreamRecordSize = 16 * 1024 * 1024
)

// Options is the set of configuration options for this Storage instance.
//
// Unlike other Storage instances, this is bound to a single archived stream.
// Path parameters in requests will be ignored in favor of the Google Storage
// URLs.
type Options struct {
	// IndexURL is the Google Storage URL for the stream's index.
	IndexURL string
	// StreamURL is the Google Storage URL for the stream's entries.
	StreamURL string

	// Client is the HTTP client to use for authentication.
	//
	// Closing this Storage instance does not close the underlying Client.
	Client gs.Client
}

type storageImpl struct {
	context.Context
	*Options

	streamBucket string
	streamPath   string
	indexBucket  string
	indexPath    string

	indexMu     sync.Mutex
	index       *logpb.LogIndex
	closeClient bool
}

// New instantiates a new Storage instance, bound to the supplied Options.
func New(c context.Context, o Options) (storage.Storage, error) {
	s := storageImpl{
		Context: c,
		Options: &o,
	}

	s.indexBucket, s.indexPath = splitGSURL(o.IndexURL)
	if s.indexBucket == "" || s.indexPath == "" {
		return nil, errors.New("invalid index URL")
	}

	s.streamBucket, s.streamPath = splitGSURL(o.StreamURL)
	if s.streamBucket == "" || s.streamPath == "" {
		return nil, errors.New("invalid stream URL")
	}
	return &s, nil
}

func (s *storageImpl) Close() {
	if s.closeClient {
		if err := s.Client.Close(); err != nil {
			log.WithError(err).Errorf(s, "Failed to close client.")
		}
	}
}

func (s *storageImpl) Put(*storage.PutRequest) error { return storage.ErrReadOnly }
func (s *storageImpl) Purge(types.StreamPath) error  { return storage.ErrReadOnly }

func (s *storageImpl) Get(req *storage.GetRequest, cb storage.GetCallback) error {
	idx, err := s.getIndex()
	if err != nil {
		return err
	}

	// Identify the byte offsets that we want to fetch from the entries stream.
	st := s.buildGetStrategy(req, idx)
	if st.lastIndex == -1 || req.Index > st.lastIndex {
		// No records to read.
		return nil
	}

	r, err := s.Client.NewReader(s.streamBucket, s.streamPath, gs.Options{
		From: st.from,
		To:   st.to,
	})
	if err != nil {
		log.WithError(err).Errorf(s, "Failed to create stream Reader.")
		return err
	}
	defer func() {
		if err := r.Close(); err != nil {
			log.WithError(err).Warningf(s, "Error closing stream Reader.")
		}
	}()
	cr := iotools.CountingReader{Reader: r}
	rio := recordio.NewReader(&cr, maxStreamRecordSize)

	buf := bytes.Buffer{}
	le := logpb.LogEntry{}
	max := req.Limit
	for {
		offset := st.from + cr.Count()

		sz, r, err := rio.ReadFrame()
		switch err {
		case nil:
			break

		case io.EOF:
			return nil

		default:
			log.Fields{
				log.ErrorKey: err,
				"index":      idx,
				"offset":     offset,
			}.Errorf(s, "Failed to read next frame.")
			return err
		}

		buf.Reset()
		buf.Grow(int(sz))
		if _, err := buf.ReadFrom(r); err != nil {
			log.Fields{
				log.ErrorKey: err,
				"offset":     offset,
				"frameSize":  sz,
			}.Errorf(s, "Failed to read frame data.")
			return err
		}
		if err := proto.Unmarshal(buf.Bytes(), &le); err != nil {
			log.Fields{
				log.ErrorKey: err,
				"offset":     offset,
				"frameSize":  sz,
			}.Errorf(s, "Failed to unmarshal log data.")
			return err
		}

		idx := types.MessageIndex(le.StreamIndex)
		if idx < req.Index {
			// Skip this entry, as it's before the first requested entry.
			continue
		}

		d := make([]byte, buf.Len())
		copy(d, buf.Bytes())
		if !cb(idx, d) {
			break
		}

		// Enforce our limit, if one is supplied.
		if max > 0 {
			max--
			if max == 0 {
				break
			}
		}
	}
	return nil
}

func (s *storageImpl) Tail(types.StreamPath) ([]byte, types.MessageIndex, error) {
	idx, err := s.getIndex()
	if err != nil {
		return nil, 0, err
	}

	// Get the offset of the last record.
	if len(idx.Entries) == 0 {
		return nil, 0, nil
	}
	lle := idx.Entries[len(idx.Entries)-1]

	// Get a Reader for the Tail entry.
	r, err := s.Client.NewReader(s.streamBucket, s.streamPath, gs.Options{
		From: int64(lle.Offset),
	})
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
			"offset":     lle.Offset,
		}.Errorf(s, "Failed to create reader.")
		return nil, 0, err
	}
	defer func() {
		if err := r.Close(); err != nil {
			log.WithError(err).Warningf(s, "Failed to close Reader.")
		}
	}()

	rio := recordio.NewReader(r, maxStreamRecordSize)
	d, err := rio.ReadFrameAll()
	if err != nil {
		log.WithError(err).Errorf(s, "Failed to read log frame.")
		return nil, 0, err
	}

	return d, types.MessageIndex(lle.StreamIndex), nil
}

// getIndex returns the cached log stream index, fetching it if necessary.
func (s *storageImpl) getIndex() (*logpb.LogIndex, error) {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()

	if s.index == nil {
		r, err := s.Client.NewReader(s.indexBucket, s.indexPath, gs.Options{})
		if err != nil {
			log.WithError(err).Errorf(s, "Failed to create index Reader.")
			return nil, err
		}
		defer func() {
			if err := r.Close(); err != nil {
				log.WithError(err).Warningf(s, "Error closing index Reader.")
			}
		}()
		indexData, err := ioutil.ReadAll(r)
		if err != nil {
			log.WithError(err).Errorf(s, "Failed to read index.")
			return nil, err
		}

		index := logpb.LogIndex{}
		if err := proto.Unmarshal(indexData, &index); err != nil {
			log.WithError(err).Errorf(s, "Failed to unmarshal index.")
			return nil, err
		}

		s.index = &index
	}
	return s.index, nil
}

func splitGSURL(u string) (string, string) {
	parts := strings.SplitN(strings.TrimPrefix(u, "gs://"), "/", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

type getStrategy struct {
	// from is the beginning byte offset of the log entry stream.
	from int64
	// to is the ending byte offset of the log entry stream.
	to int64

	// lastIndex is the last log entry index in the stream. This will be -1 if
	// there are no entries in the stream.
	lastIndex types.MessageIndex
}

func (s *storageImpl) buildGetStrategy(req *storage.GetRequest, idx *logpb.LogIndex) *getStrategy {
	st := getStrategy{}

	if len(idx.Entries) == 0 {
		st.lastIndex = -1
		return &st
	}

	st.lastIndex = types.MessageIndex(idx.Entries[len(idx.Entries)-1].StreamIndex)
	startIdx := indexEntryFor(idx.Entries, req.Index)
	if startIdx < 0 {
		startIdx = 0
	}
	le := idx.Entries[startIdx]
	st.from = int64(le.Offset)

	// If we have a limit, and we have enough index entries to upper-bound our
	// stream based on that limit, use that.
	//
	// Note that this may overshoot if the index and/or stream is sparse. We know
	// for sure that we have one LogEntry per index entry, so that's the best we
	// can do.
	if req.Limit > 0 {
		if ub := startIdx + req.Limit; ub < len(idx.Entries) {
			st.to = int64(idx.Entries[ub].Offset)
		}
	}
	return &st
}

// indexEntryFor identifies the log index entry closest (<=) to the specified
// index.
//
// If the first index entry is greater than our search index, -1 will be
// returned. This should never happen in practice, though, since our index
// construction always indexes log entry #0.
//
// It does this by performing a binary search over the index entries.
func indexEntryFor(entries []*logpb.LogIndex_Entry, i types.MessageIndex) int {
	ui := uint64(i)
	s := sort.Search(len(entries), func(i int) bool {
		return entries[i].StreamIndex > ui
	})

	// The returned index is the one immediately after the index that we want. If
	// our search returned 0, the first index entry is > our search entry, and we
	// will return nil.
	return s - 1
}
