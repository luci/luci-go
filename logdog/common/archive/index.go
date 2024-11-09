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

package archive

import (
	"io"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/logdog/api/logpb"
)

// indexBuilder is a stateful engine that constructs an archival index.
type indexBuilder struct {
	*Manifest
	index logpb.LogIndex

	lastPrefixIndex uint64
	lastStreamIndex uint64
	lastBytes       uint64

	latestBufferedEntry *logpb.LogIndex_Entry

	sizeFunc func(proto.Message) int
}

func (i *indexBuilder) addLogEntry(le *logpb.LogEntry, offset int64) {
	// Only calculate the size if we actually use it.
	if i.ByteRange > 0 {
		i.lastBytes += uint64(i.size(le))
	}

	// Update our stream properties.
	i.index.LastPrefixIndex = le.PrefixIndex
	i.index.LastStreamIndex = le.StreamIndex
	i.index.LogEntryCount++

	entry := logpb.LogIndex_Entry{
		Sequence:    le.Sequence,
		PrefixIndex: le.PrefixIndex,
		StreamIndex: le.StreamIndex,
		Offset:      uint64(offset),
		TimeOffset:  le.TimeOffset,
	}

	// Do we index this LogEntry?
	if len(i.index.Entries) > 0 {
		if !((i.StreamIndexRange > 0 && (le.StreamIndex-i.lastStreamIndex) >= uint64(i.StreamIndexRange)) ||
			(i.PrefixIndexRange > 0 && (le.PrefixIndex-i.lastPrefixIndex) >= uint64(i.PrefixIndexRange)) ||
			(i.ByteRange > 0 && i.lastBytes >= uint64(i.ByteRange))) {
			// Not going to index this entry. Buffer it as a terminator.
			i.latestBufferedEntry = &entry
			return
		}

		i.lastBytes = 0
	}

	i.index.Entries = append(i.index.Entries, &entry)
	i.latestBufferedEntry = nil

	// Update our counters.
	i.lastStreamIndex = le.StreamIndex
	i.lastPrefixIndex = le.PrefixIndex
}

func (i *indexBuilder) emit(w io.Writer) error {
	// Always include the last stream entry in the index.
	if i.latestBufferedEntry != nil {
		i.index.Entries = append(i.index.Entries, i.latestBufferedEntry)
	}

	d, err := proto.Marshal(&i.index)
	if err != nil {
		return err
	}

	if _, err := w.Write(d); err != nil {
		return err
	}
	return nil
}

func (i *indexBuilder) size(pb proto.Message) int {
	if f := i.sizeFunc; f != nil {
		return f(pb)
	}
	return proto.Size(pb)
}
