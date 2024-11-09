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
	"io"
	"time"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/logdog/client/butler/bundler"
	"go.chromium.org/luci/logdog/common/types"
)

type stream struct {
	log logging.Logger
	now func() time.Time

	name types.StreamName

	r  io.Reader
	c  io.Closer
	bs bundler.Stream
}

func (s *stream) readChunk() bool {
	d := s.bs.LeaseData()

	amount, err := s.r.Read(d.Bytes())
	if amount > 0 {
		d.Bind(amount, s.now())

		s.log.Debugf("Read %d bytes", amount)

		// Add the data to our bundler endpoint. This may block waiting for the
		// bundler to consume data.

		err := s.bs.Append(d)
		d = nil // Append takes ownership of "d" regardless of error.
		if err != nil {
			s.log.Errorf("Failed to Append to the stream: %s", err)
			return false
		}
	}

	// If we haven't consumed our data, release it.
	if d != nil {
		d.Release()
		d = nil
	}

	switch err {
	case nil:
		break

	case io.EOF:
		s.log.Debugf("Stream encountered EOF.")
		return false

	default:
		s.log.Errorf("Stream encountered error during Read: %s", err)
		return false
	}

	return true
}

func (s *stream) closeStream() {
	if err := s.c.Close(); err != nil {
		s.log.Warningf("Error closing stream: ", err)
	}
	s.bs.Close()
}
