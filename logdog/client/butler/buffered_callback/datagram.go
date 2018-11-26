// Copyright 2018 The LUCI Authors.
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

package buffered_callback

import (
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler/bundler"
)

// assertGetDatagram panics if the passed LogEntry does not contain Datagram data, or returns it.
func assertGetDatagram(le *logpb.LogEntry) *logpb.Datagram {
	if dg := le.GetDatagram(); dg == nil {
		panic(
			errors.Annotate(
				InvalidStreamType,
				fmt.Sprintf("got %T, expected *logpb.LogEntry_Datagram", le.Content),
			).Err(),
		)
	} else {
		return dg
	}
}

// GetWrappedDatagramCallback wraps a passed callback meant to be called on complete Datagrams so
// that it is actually called on complete Datagrams.
//
// The wrapped callback panics if:
// - the passed LogEntry is not a Datagram LogEntry
// - it receives a complete Datagram while partial Datagrams are still buffered
func GetWrappedDatagramCallback(cb bundler.StreamChunkCallback) bundler.StreamChunkCallback {
	if cb == nil {
		return nil
	}

	var buf [][]byte
	var bufSize int
	var curLogEntryBase logpb.LogEntry
	return func(le *logpb.LogEntry) {
		if le == nil {
			return
		}
		dg := assertGetDatagram(le)

		// If we're a complete Datagram and the buffer is empty, which is the expected case except
		// when aggressively flushing the stream, just call the callback and be done.
		if dg.Partial == nil {
			if buf != nil {
				panic(LostDatagramChunk)
			}
			cb(le)
			return
		}

		if buf == nil {
			curLogEntryBase = *le
		}
		buf = append(buf, dg.Data)
		bufSize += len(dg.Data)

		// We're a partial Datagram; if we're not the last chunk, just return.
		// We don't check order because the LogEntries on which this is called should already be checked
		// for order by fixupLogEntry.
		if !dg.Partial.Last {
			return
		}

		// We're either already a full Datagram, or the end of one, so reconstruct.
		bytes := make([]byte, 0, bufSize)
		for _, bytesPart := range buf {
			bytes = append(bytes, bytesPart...)
		}

		// Use the first LogEntry as the source for indices etc. in the full one.
		curLogEntryBase.Content = &logpb.LogEntry_Datagram{
			Datagram: &logpb.Datagram{
				Data: bytes,
			},
		}

		// Reset the buffer and invoke callback.
		buf, bufSize = nil, 0
		cb(&curLogEntryBase)
	}
}
