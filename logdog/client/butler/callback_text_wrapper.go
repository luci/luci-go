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

// assertGetText panics if the passed LogEntry does not contain Text data, or returns it.
func assertGetText(le *logpb.LogEntry) *logpb.Text {
	if txt := le.GetText(); txt == nil {
		panic(errors.Reason(
			"wrong StreamType: got %T, expected *logpb.LogEntry_Text", le.Content,
		).Err())
	} else {
		return txt
	}
}

// getWrappedTextCallback wraps a passed callback meant to be called at the
// ends of Text lines so that it is actually called at the end of Text lines.
//
// Does not wrap callback to guarantee being called at the end of *every* Text
// line.
//
// The wrapped callback panics if:
// - the passed LogEntry is not a Text LogEntry
// - the passed LogEntry has lines in a form other than described in log.proto
func getWrappedTextCallback(cb StreamChunkCallback) StreamChunkCallback {
	if cb == nil {
		return nil
	}

	var flushed bool
	var buf []*logpb.Text_Line
	var streamIdx uint64
	var sequence uint64

	flushBuffer := func() {
		if len(buf) == 0 {
			return
		}

		data := &logpb.LogEntry{
			Content: &logpb.LogEntry_Text{
				Text: &logpb.Text{
					Lines: buf,
				},
			},
			StreamIndex: streamIdx,
			Sequence:    sequence,
		}

		cb(data)

		streamIdx++
		sequence += uint64(len(buf))
		buf = nil
	}

	return func(le *logpb.LogEntry) {
		if le == nil && !flushed { // "flush"
			flushed = true
			flushBuffer()
			cb(nil)
			return
		}
		if flushed {
			panic(errors.New("called with nil multiple times"))
		}

		txt := assertGetText(le)

		if len(txt.Lines) == 0 {
			panic(errors.New("called with no lines"))
		}

		// Process the first line, which may be partial.
		firstLine := txt.Lines[0]
		buf = append(buf, firstLine)

		if firstLine.Delimiter == "" {
			if len(txt.Lines) > 1 {
				panic(errors.New("partial line not last in LogEntry"))
			}
			return
		}

		// Convert buf's contents into a single line and store that.
		if len(buf) > 1 {
			bufSize := 0
			for _, line := range buf {
				bufSize += len(line.Value)
			}
			wholeFirstLine := &logpb.Text_Line{
				Value:     make([]byte, 0, bufSize),
				Delimiter: firstLine.Delimiter,
			}
			for _, line := range buf {
				wholeFirstLine.Value = append(wholeFirstLine.Value, line.Value...)
			}
			buf = []*logpb.Text_Line{wholeFirstLine}
		}

		// Process the next lines, which should be all complete with at most one partial at the end.
		wholeLines := txt.Lines[1:]
		var lastPartialLine *logpb.Text_Line
		if lastIdx := len(wholeLines) - 1; lastIdx >= 0 && wholeLines[lastIdx].Delimiter == "" {
			lastPartialLine = wholeLines[lastIdx]
			wholeLines = wholeLines[:lastIdx]
		}

		for _, line := range wholeLines {
			if line.Delimiter == "" {
				panic(errors.New("partial line not last in LogEntry"))
			}
			buf = append(buf, line)
		}

		flushBuffer()

		// If the last line is partial, record it.
		if lastPartialLine != nil {
			buf = []*logpb.Text_Line{lastPartialLine}
		}
	}
}
