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

package reassembler

import (
	"fmt"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler/bundler"
)

// assertGetDatagram panics if the passed LogEntry does not contain Datagram data, or returns it.
func assertGetDatagram(le *logpb.LogEntry) *logpb.Datagram {
	if dg := le.GetDatagram(); dg == nil {
		panic(fmt.Sprintf(
				"expected to pass Datagrams to getWrappedCallback for Datagrams, got %v",
				le.Content,
		))
	} else {
		return dg
	}
}

// getWrappedDatagramCallback wraps a passed callback meant to be called on complete Datagrams so
// that it is actually called on complete Datagrams.
func getWrappedDatagramCallback(cb bundler.StreamChunkCallback) bundler.StreamChunkCallback {
	if cb == nil {
		return nil
	}

	var buf []*[]byte
	var bufSize int
	var leFull logpb.LogEntry
	return func(le *logpb.LogEntry) {
		if le == nil {
			return
		}
		dg := assertGetDatagram(le)

		// If we're a complete Datagram and the buffer is empty, which is the expected case except
		// when aggressively flushing the stream, just call the callback and be done.
		if dg.Partial == nil {
			if buf != nil {
				panic(fmt.Sprintf("got self-contained Datagram LogEntry while buffered LogEntries exist"))
			}
			cb(le)
			return
		}

		if buf == nil {
			leFull = *le
		}
		buf = append(buf, &dg.Data)
		bufSize += len(dg.Data)

		// Otherwise, we're a partial Datagram, so if we're not the last chunk, just return.
		// We don't check order because the LogEntries on which this is called should already be checked
		// for order by fixupLogEntry.
		if !dg.Partial.Last {
			return
		}

		// Otherwise, we're either already a full Datagram, or the end of one, so reconstruct.
		bytes := make([]byte, 0, bufSize)
		for _, bytesPart := range buf {
			bytes = append(bytes, *bytesPart...)
		}

		// Use the first LogEntry as the source for indices etc. in the full one.
		leFull.Content = &logpb.LogEntry_Datagram{
				Datagram: &logpb.Datagram{
						Data: bytes,
				},
		}

		// Reset the buffer and invoke callback.
		buf, bufSize = nil, 0
		cb(&leFull)
	}
}
