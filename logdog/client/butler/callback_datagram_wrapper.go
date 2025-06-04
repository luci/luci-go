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

package butler

import (
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/logdog/api/logpb"
)

// assertGetDatagram panics if the passed LogEntry does not contain Datagram data, or returns it.
func assertGetDatagram(le *logpb.LogEntry) *logpb.Datagram {
	if dg := le.GetDatagram(); dg == nil {
		panic(errors.Fmt("wrong StreamType: got %T, expected *logpb.LogEntry_Datagram", le.Content))
	} else {
		return dg
	}
}

// getWrappedDatagramCallback wraps a passed callback meant to be called on complete Datagrams so
// that it is actually called on complete Datagrams.
//
// The wrapped callback panics if:
// - the passed LogEntry is not a Datagram LogEntry
// - it receives a complete Datagram while partial Datagrams are still buffered
func getWrappedDatagramCallback(cb StreamChunkCallback) StreamChunkCallback {
	if cb == nil {
		return nil
	}

	var flushed bool
	var buf [][]byte
	var streamIdx uint64

	flushData := func(data []byte) {
		cb(&logpb.LogEntry{
			Content: &logpb.LogEntry_Datagram{
				Datagram: &logpb.Datagram{
					Data: data,
				},
			},
			StreamIndex: streamIdx,
			Sequence:    streamIdx, // same as StreamIndex for buffered datagrams
		})
		streamIdx++
	}

	flushBuffer := func() {
		if len(buf) == 0 {
			return
		}

		bufSize := 0
		for _, chunk := range buf {
			bufSize += len(chunk)
		}
		rawData := make([]byte, 0, bufSize)
		for _, chunk := range buf {
			rawData = append(rawData, chunk...)
		}

		flushData(rawData)

		buf = nil
	}

	return func(le *logpb.LogEntry) {
		if le == nil && !flushed { // "flush"
			flushed = true

			// if we have buffered data, just ignore it. This means that we're being
			// flushed in the middle of a partial datagram.
			buf = nil

			cb(nil)
			return
		}
		if flushed {
			panic(errors.New("called with nil multiple times"))
		}

		dg := assertGetDatagram(le)

		// If we're a complete Datagram and the buffer is empty, which is the
		// expected case except when aggressively flushing the stream, just call the
		// callback and be done.
		if dg.Partial == nil {
			if buf != nil {
				panic(errors.New(
					"got self-contained Datagram LogEntry while buffered LogEntries exist",
				))
			}
			flushData(dg.Data)
			return
		}

		buf = append(buf, dg.Data)

		// We're a partial Datagram; if we're not the last chunk, just return.
		if !dg.Partial.Last {
			return
		}

		// We're either already a full Datagram, or the end of one, so send it.
		flushBuffer()
	}
}
