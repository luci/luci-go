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

package storage

import (
	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"
)

// Entry is a logpb.LogEntry wrapper that lazily evaluates / unmarshals the
// underlying LogEntry data as needed.
//
// Entry is not goroutine-safe.
type Entry struct {
	D []byte

	streamIndex types.MessageIndex
	le          *logpb.LogEntry
}

// MakeEntry creates a new Entry.
//
// All Entry must be backed by data. The index, "idx", is optional. If <0, it
// will be calculated by unmarshalling the data into a LogEntry and pulling the
// value from there.
func MakeEntry(d []byte, idx types.MessageIndex) *Entry {
	return &Entry{
		D:           d,
		streamIndex: idx,
	}
}

// GetStreamIndex returns the LogEntry's stream index.
//
// If this needs to be calculated by unmarshalling the LogEntry, this will be
// done. If this fails, an error will be returned.
//
// If GetLogEntry has succeeded, subsequent GetStreamIndex calls will always
// be fast and succeed.
func (e *Entry) GetStreamIndex() (types.MessageIndex, error) {
	if e.streamIndex < 0 {
		le, err := e.GetLogEntry()
		if err != nil {
			return -1, err
		}

		e.streamIndex = types.MessageIndex(le.StreamIndex)
	}

	return e.streamIndex, nil
}

// GetLogEntry returns the unmarshalled LogEntry data.
//
// The first time this is called, the LogEntry will be unmarshalled from its
// underlying data.
func (e *Entry) GetLogEntry() (*logpb.LogEntry, error) {
	if e.le == nil {
		if e.D == nil {
			// This can happen with keys-only results.
			return nil, errors.New("no log entry data")
		}

		var le logpb.LogEntry
		if err := proto.Unmarshal(e.D, &le); err != nil {
			return nil, errors.Annotate(err, "failed to unmarshal").Err()
		}
		e.le = &le
	}

	return e.le, nil
}
