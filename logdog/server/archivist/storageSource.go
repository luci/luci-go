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
	"io"

	"go.chromium.org/luci/common/errors"
	log "go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/types"
)

// storageSource is a renderer.Source that pulls log entries from intermediate
// storage via its storage.Storage instance.
type storageSource struct {
	context.Context

	st            storage.Storage    // the storage instance to read from
	project       string             // the project of the log stream
	path          types.StreamPath   // the path of the log stream
	terminalIndex types.MessageIndex // if >= 0, discard logs beyond this

	buf           []*logpb.LogEntry
	lastIndex     types.MessageIndex
	logEntryCount int64
}

func (s *storageSource) bufferEntries(start types.MessageIndex) error {
	bytes := 0

	req := storage.GetRequest{
		Project: s.project,
		Path:    s.path,
		Index:   start,
	}
	return s.st.Get(s, req, func(e *storage.Entry) bool {
		le, err := e.GetLogEntry()
		if err != nil {
			log.WithError(err).Errorf(s, "Failed to unmarshal LogEntry.")
			return false
		}
		s.buf = append(s.buf, le)

		// Stop loading if we've reached or exceeded our buffer size.
		bytes += len(e.D)
		return bytes < storageBufferSize
	})
}

func (s *storageSource) NextLogEntry() (*logpb.LogEntry, error) {
	if len(s.buf) == 0 {
		s.buf = s.buf[:0]
		if err := s.bufferEntries(s.lastIndex + 1); err != nil {
			if err == storage.ErrDoesNotExist {
				log.Warningf(s, "Archive target stream does not exist in intermediate storage.")
				return nil, io.EOF
			}

			return nil, errors.Fmt("retrieve from storage: %w", err)
		}
	}

	// If we have no more buffered entries, we have exhausted our log stream.
	if len(s.buf) == 0 {
		// If we have a terminal index, but we didn't actually emit that index,
		// mark that we have missing entries.
		if s.terminalIndex >= 0 && s.lastIndex != s.terminalIndex {
			log.Fields{
				"terminalIndex": s.terminalIndex,
				"lastIndex":     s.lastIndex,
			}.Warningf(s, "Log stream stopped before terminal index.")
		} else {
			// Encountered end of stream.
		}

		return nil, io.EOF
	}

	// Pop the next log entry and advance the stream.
	var le *logpb.LogEntry
	le, s.buf = s.buf[0], s.buf[1:]

	// If we're enforcing a maximum terminal index, return end of stream if this
	// LogEntry exceeds that index.
	sidx := types.MessageIndex(le.StreamIndex)
	if s.terminalIndex >= 0 && sidx > s.terminalIndex {
		log.Fields{
			"index":         sidx,
			"terminalIndex": s.terminalIndex,
		}.Warningf(s, "Discarding log entries beyond expected terminal index.")
		return nil, io.EOF
	}

	s.lastIndex = sidx
	s.logEntryCount++
	return le, nil
}
