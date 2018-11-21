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

// assertGetText panics if the passed LogEntry does not contain Text data, or returns it.
func assertGetText(le *logpb.LogEntry) *logpb.Text {
	if txt := le.GetText(); txt == nil {
		panic(
			errors.Annotate(
				InvalidStreamType,
				fmt.Sprintf("got %T, expected *logpb.LogEntry_Text", le.Content),
			).Err(),
		)
	} else {
		return txt
	}
}

// GetWrappedCallback wraps a passed callback meant to be called at the ends of Text lines so that
// it is actually called at the end of Text lines.
// Does not wrap callback to guarantee being called at the end of *every* Text line.
//
// The wrapped callback panics if:
// - the passed LogEntry is not a Text LogEntry
// - the passed LogEntry has lines in a form other than described in log.proto
func GetWrappedTextCallback(cb bundler.StreamChunkCallback) bundler.StreamChunkCallback {
	if cb == nil {
		return nil
	}

	var buf []*logpb.Text_Line
	var curLineSize int
	var nextLogEntryBase *logpb.LogEntry
	return func(le *logpb.LogEntry) {
		if le == nil {
			return
		}
		txt := assertGetText(le)

		if len(txt.Lines) <= 0 {
			return
		}
		if nextLogEntryBase == nil {
			nextLogEntryBase = &(*le)
		}

		var curLine *logpb.Text_Line

		// Process the first line, which may be partial.
		curLine = txt.Lines[0]
		buf = append(buf, curLine)
		curLineSize += len(curLine.Value)

		if curLine.Delimiter == "" {
			if len(txt.Lines) > 1 {
				panic(PartialLineNotLast)
			}
			return
		}

		// Replace buf with a compressed complete line of its contents.
		if len(buf) > 1 {
			lineCompletion := &logpb.Text_Line{
				Value:     make([]byte, 0, curLineSize),
				Delimiter: curLine.Delimiter,
			}
			for _, line := range buf {
				lineCompletion.Value = append(lineCompletion.Value, line.Value...)
			}

			buf = make([]*logpb.Text_Line, 0, len(txt.Lines))
			buf = append(buf, lineCompletion)
			curLineSize = 0
		}

		// Process the next lines, which should be all complete with at most one partial at the end.
		var i int
		for i, curLine = range txt.Lines[1:] {
			if curLine.Delimiter == "" {
				break
			}
			buf = append(buf, curLine)
		}

		if i+1 < len(txt.Lines)-1 {
			panic(PartialLineNotLast)
		}

		nextLogEntryBase.Content = &logpb.LogEntry_Text{
			Text: &logpb.Text{
				Lines: buf,
			},
		}
		cb(nextLogEntryBase)
		buf, curLineSize = nil, 0

		// If the last line is partial, record it.
		if i+1 == len(txt.Lines)-1 {
			buf = []*logpb.Text_Line{curLine}
			curLineSize = len(curLine.Value)
		}
		nextLogEntryBase = nil
	}
}
