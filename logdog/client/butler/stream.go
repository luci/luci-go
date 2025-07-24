// Copyright 2015 The LUCI Authors.
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

package butler

import (
	"fmt"
	"io"
	"time"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler/bundler"
	"go.chromium.org/luci/logdog/common/archive"
	"go.chromium.org/luci/logdog/common/types"
)

const (
	// maxStreamBytes is the maximum number of bytes that will be saved for a
	// single stream. If a stream exceeds this size, it will be truncated.
	// This limit is 90% of the limit set in LogDog backend (archivist).
	maxStreamBytes = int64(archive.MaxLogContentSize * 0.9)
)

type stream struct {
	log logging.Logger
	now func() time.Time

	name          types.StreamName
	streamType    logpb.StreamType
	totalSize     int64
	truncatedSize int64
	// Set for testing, override the maxStreamBytes.
	maxStreamBytesOverride int64

	r  io.Reader
	c  io.Closer
	bs bundler.Stream
}

// readChunk reads a chunk of data from the stream's reader and appends it to
// the bundler stream.
//
// Returns true if stream could return more data at a later time.
func (s *stream) readChunk() bool {
	d := s.bs.LeaseData()
	defer func() {
		// If we haven't consumed our data, release it.
		if d != nil {
			d.Release()
			d = nil
		}
	}()

	amount, err := s.r.Read(d.Bytes())

	streamBytesLimit := int64(maxStreamBytes)
	if s.maxStreamBytesOverride > 0 {
		streamBytesLimit = s.maxStreamBytesOverride
	}
	// Process the n > 0 bytes returned before considering the error.
	if amount > 0 {
		if s.totalSize+int64(amount) > streamBytesLimit {
			d.Release() // The data we just read is being discarded.
			d = nil
			if s.truncatedSize == 0 {
				// Only log the message at the first truncated chunk.
				truncationMsg := fmt.Sprintf("\nStream size limit of %d bytes reached. Truncating.", streamBytesLimit)
				if appendErr := s.appendText(truncationMsg); appendErr != nil {
					s.log.Errorf("truncation start message: %s", appendErr)
					return false
				}
			}
			s.truncatedSize += int64(amount)
		} else {
			d.Bind(amount, s.now())

			// Add the data to our bundler endpoint. This may block waiting for the
			// bundler to consume data.

			err := s.bs.Append(d)
			d = nil // Append takes ownership of "d" regardless of error.
			if err != nil {
				s.log.Errorf("Failed to append to the stream: %s", err)
				return false
			}
		}
		s.totalSize += int64(amount)
	}

	switch err {
	case nil:
		break

	case io.EOF:
		s.log.Debugf("Stream encountered EOF.")
		if s.truncatedSize > 0 {
			truncationMsg := fmt.Sprintf("\n*** LOG TRUNCATED: Log stream exceeded LogDog's %d-byte limit,"+
				"omitted %d bytes, total stream size %d bytes ***\n", streamBytesLimit, s.truncatedSize, s.totalSize)
			if appendErr := s.appendText(truncationMsg); appendErr != nil {
				s.log.Errorf("truncation summary: %s", appendErr)
				return false
			}
		}
		return false

	default:
		s.log.Errorf("Stream encountered error during Read: %s", err)
		return false
	}

	return true
}

func (s *stream) appendText(text string) error {
	switch s.streamType {
	case logpb.StreamType_TEXT:
		msgData := s.bs.LeaseData()
		n := copy(msgData.Bytes(), text)
		msgData.Bind(n, s.now())
		err := s.bs.Append(msgData)
		msgData = nil
		if err != nil {
			return err
		}
	default:
		// We can't append text to non-text stream, so just log the message here.
		s.log.Warningf("%s streamType: (%s).", text, s.streamType)
	}
	return nil
}

func (s *stream) closeStream() {
	if err := s.c.Close(); err != nil {
		s.log.Warningf("Error closing stream: %s", err)
	}
	s.bs.Close()
}
