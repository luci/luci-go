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

	streamStart     time.Time
	streamID        string
	lastTimeOffset  *durationpb.Duration // TimeOffset of the latest LogEntry passed to Append.
	hasCompleteLine bool
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
	var ret []*cl.Entry
	flushIf := func(cond bool) {
		if !cond {
			return
		}
		if e := eb.flush(); e != nil {
			ret = append(ret, e)
		}
	}
	eb.lastTimeOffset = e.TimeOffset
	lines := e.GetText().GetLines()
	for i, l := range lines {
		lastLineAndIncomplete := i == len(lines)-1 && l.Delimiter == ""
		flushIf(eb.hasCompleteLine && (eb.isExceeding(l) || lastLineAndIncomplete))
		eb.safeAdd(l)
	}
	flushIf(eb.hasCompleteLine)
	return ret
}

func (eb *entryBuffer) safeAdd(line *logpb.Text_Line) {
	remaining := eb.maxPayload - eb.buf.Len()
	n, _ := eb.buf.Write(line.Value[:min(remaining, len(line.Value))])
	if d := line.Delimiter; d != "" {
		remaining -= n
		eb.buf.WriteString(d[:min(remaining, len(d))])
		eb.hasCompleteLine = true
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

	if eb.hasCompleteLine {
		eb.hasCompleteLine = false
	}
	// Trim line end delimiters because it Cloud Logging UI renders it as an extra
	// empty line at the end of the log message.
	payload := strings.TrimRight(eb.buf.String(), "\n\r")
	if payload == "" {
		return nil
	}

	ret := &cl.Entry{
		Payload:   payload,
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

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}
