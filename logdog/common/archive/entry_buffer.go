// Copyright 2021 The LUCI Authors.
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
	"fmt"
	"strings"
	"time"

	cl "cloud.google.com/go/logging"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/logdog/api/logpb"
)

// entryBuffer buffers the lines from a series of LogEntry(s) and constructs logging.Entry(s)
// with them.
type entryBuffer struct {
	buf        strings.Builder
	seq        int
	maxPayload int

	streamStart time.Time
	streamID    string
}

// NewEntryBuffer constructs and returns entryBuffer.
func NewEntryBuffer(maxPayload int, streamID string, desc *logpb.LogStreamDescriptor) *entryBuffer {
	return &entryBuffer{
		maxPayload:  maxPayload,
		streamStart: desc.Timestamp.AsTime(),
		streamID:    streamID,
	}
}

// Append appends a new LogEntry into the buffer and returns 1 or more of CloudLogging Entry(s), if
// the LogEntry contains at least one complete line.
func (eb *entryBuffer) Append(e *logpb.LogEntry) (entries []*cl.Entry) {
	offset := e.TimeOffset
	lines := e.GetText().GetLines()
	if len(lines) == 0 {
		return
	}

	// If the first line is a partial line, add it to the buffer as much as possible, and
	// return. The buffer should be flushed out when the first complete line is encountered.
	if lines[0].Delimiter == "" {
		eb.addLine(lines[0], true)
		return
	}

	// There are 2 types of complete lines.
	// a) a complete line that ends buffered partial lines.
	// b) self-contained complete line.
	//
	// Both are sementically complete lines, but (a) is actually a partial line.
	// they are handled differently, when it is going to exceed the length limit with
	// the buffered lines.
	//
	// If the current line is (a), buffer(a) as much as possible, and then flush().
	// : truncation always occurs on (a).
	// If the current line is (b), flush() and then buffer(b) as much as possible.
	// : truncation does not occur on (b), unless (b) alone exceeds the length limit.
	if eb.buf.Len() > 0 && eb.isExceeding(lines[0]) {
		eb.addLine(lines[0], false)
		entries = append(entries, eb.Flush(offset))
		lines = lines[1:]
	}

	// Keep the last line in case it is a partial line. It should be carried over to the next
	// LogEntry.
	var pl *logpb.Text_Line
	if len(lines) > 0 {
		if last := lines[len(lines)-1]; last.Delimiter == "" {
			pl = last
			lines = lines[:len(lines)-1]
		}
	}
	for i, line := range lines {
		if eb.buf.Len() > 0 && eb.isExceeding(line) {
			entry := eb.Flush(offset)
			if i > 0 {
				// Trim out the delimiter from the last element.
				p := entry.Payload.(string)
				entry.Payload = p[:len(p)-len(lines[i-1].Delimiter)]
			}
			entries = append(entries, entry)
		}

		if eb.isExceeding(line) || i == len(lines)-1 {
			// If the last line has an empty value, discard the line.
			if len(line.Value) > 0 {
				eb.addLine(line, false)
			}
			if eb.buf.Len() > 0 {
				entries = append(entries, eb.Flush(offset))
			}
		} else {
			eb.addLine(line, true)
		}
	}
	if pl != nil {
		eb.addLine(pl, true)
	}
	return
}

func (eb *entryBuffer) addLine(line *logpb.Text_Line, addDelimiter bool) {
	writable := eb.maxPayload - eb.buf.Len()
	toWrite := len(line.Value)
	if writable < toWrite {
		toWrite = writable
	}

	if writable > 0 {
		eb.buf.Write(line.Value[:toWrite])
	}
	if addDelimiter && writable-toWrite >= len(line.Delimiter) {
		eb.buf.WriteString(line.Delimiter)
	}
}

// isExceeding returns true if a given line with the buffered lines is going to exceed maxPayload.
// False, otherwise.
func (eb *entryBuffer) isExceeding(l *logpb.Text_Line) bool {
	return l != nil && eb.buf.Len()+len(l.Value)+len(l.Delimiter) > eb.maxPayload
}

// Flush the buffer to a logging.Entry with a given TimeOffset.
// If the buffer contains no content, returns nil.
func (eb *entryBuffer) Flush(offset *durationpb.Duration) *cl.Entry {
	if eb.buf.Len() == 0 {
		return nil
	}

	ts := eb.streamStart
	if offset != nil {
		ts = eb.streamStart.Add(offset.AsDuration())
	}
	defer func() {
		eb.buf.Reset()
		eb.seq += 1
	}()
	return &cl.Entry{
		Payload:   eb.buf.String(),
		Timestamp: ts,

		// InsertID uniquely identifies each LogEntry to dedup entries in CloudLogging.
		InsertID: fmt.Sprintf("%s/%d", eb.streamID, eb.seq),

		// Set the Trace with the streamID so that Log entries from the same Log stream
		// can be grouped together in CloudLogging UI.
		Trace: eb.streamID,
	}
}
