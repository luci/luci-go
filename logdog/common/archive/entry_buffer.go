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

	streamStart    time.Time
	streamID       string
	lastTimeOffset *durationpb.Duration // TimeOffset of the latest LogEntry passed to Append.
}

// newEntryBuffer constructs and returns entryBuffer.
func newEntryBuffer(maxPayload int, streamID string, desc *logpb.LogStreamDescriptor) *entryBuffer {
	return &entryBuffer{
		maxPayload:  maxPayload,
		streamStart: desc.Timestamp.AsTime(),
		streamID:    streamID,
	}
}

// append appends a new LogEntry into the buffer and returns CloudLogging entries ready for sending.
func (eb *entryBuffer) append(e *logpb.LogEntry) []*cl.Entry {
	lines := e.GetText().GetLines()
	if len(lines) == 0 {
		return nil
	}

	// If the first line is a partial line, add it to the buffer as much as possible, and
	// return. The buffer should be flushed out when the first complete line is encountered.
	if lines[0].Delimiter == "" {
		eb.safeAdd(lines[0].Value)
		return nil
	}

	eb.lastTimeOffset = e.TimeOffset

	// There are 2 types of complete lines.
	// a) a complete line that ends the buffered partial lines from the previous entries.
	// b) self-contained complete line.
	//
	// Both are semantically complete lines, but (a) is actually a partial line.
	// They are handled differently, when the resulting complete line is larger than maxPayload.
	//
	// If the current line is (a), buffer(a) as much as possible, and then flush().
	// : truncation always occurs on (a).
	// If the current line is (b), flush() and then buffer(b) as much as possible.
	// : truncation does not occur on (b), unless (b) alone is larger than maxPayload.
	//
	// This handles (a), so that the below loop only needs to handle (b).
	var ret []*cl.Entry
	if eb.buf.Len() > 0 && eb.isExceeding(lines[0]) {
		eb.safeAdd(lines[0].Value)
		ret = append(ret, eb.flush())
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
			entry := eb.flush()
			if i > 0 {
				// Trim out the delimiter from the last element.
				p := entry.Payload.(string)
				entry.Payload = p[:len(p)-len(lines[i-1].Delimiter)]
			}
			ret = append(ret, entry)
		}

		eb.safeAdd(line.Value)

		// If the line itself is too long or the last line in the entry, flush it out.
		//
		// It's important to skip the delimiter of the last line. Otherwise,
		// all the CloudLogging entries will have an ending delimiter, such as \n,
		// rendered in the CloudLogging UI.
		//
		// e.g.,
		// Entry1: "line1
		// "
		// Entry2: "line2
		// "
		// Entry3: "line3
		// "
		if eb.isExceeding(line) || i == len(lines)-1 {
			if eb.buf.Len() > 0 {
				ret = append(ret, eb.flush())
			}
		} else {
			// Add the delimiter so that lines can be distinguished when they got
			// joined into a single Payload.
			eb.buf.WriteString(line.Delimiter)
		}
	}
	if pl != nil {
		eb.safeAdd(pl.Value)
	}
	return ret
}

func (eb *entryBuffer) safeAdd(line []byte) {
	writable := eb.maxPayload - eb.buf.Len()
	toWrite := len(line)
	if writable < toWrite {
		toWrite = writable
	}
	if writable > 0 {
		eb.buf.Write(line[:toWrite])
	}
}

// isExceeding returns true if a given line with the buffered lines is going to exceed maxPayload.
// False, otherwise.
func (eb *entryBuffer) isExceeding(l *logpb.Text_Line) bool {
	return l != nil && eb.buf.Len()+len(l.Value)+len(l.Delimiter) > eb.maxPayload
}

// flush flushes the buffer and return a logging.Entry with the content.
// If the buffer contains no content, returns nil.
func (eb *entryBuffer) flush() *cl.Entry {
	if eb.buf.Len() == 0 {
		return nil
	}

	ts := eb.streamStart
	if eb.lastTimeOffset != nil {
		ts = eb.streamStart.Add(eb.lastTimeOffset.AsDuration())
	}
	ret := &cl.Entry{
		Payload:   eb.buf.String(),
		Timestamp: ts,

		// InsertID uniquely identifies each LogEntry to dedup entries in CloudLogging.
		InsertID: fmt.Sprintf("%s/%d", eb.streamID, eb.seq),

		// Set the Trace with the streamID so that Log entries from the same Log stream
		// can be grouped together in CloudLogging UI.
		Trace: eb.streamID,
	}
	eb.buf.Reset()
	eb.seq += 1
	return ret
}
