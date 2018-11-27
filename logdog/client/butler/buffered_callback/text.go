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
	var bufSize int
	var curLogEntryBase *logpb.LogEntry
	return func(le *logpb.LogEntry) {
		if le == nil {
			return
		}
		txt := assertGetText(le)

		if len(txt.Lines) == 0 {
			return
		}
		if curLogEntryBase == nil {
			curLogEntryBase = &(*le)
		}

		toCallback := make([]*logpb.Text_Line, 0, len(txt.Lines))

		// Process the first line, which may be partial.
		firstLine := txt.Lines[0]
		buf = append(buf, firstLine)
		bufSize += len(firstLine.Value)

		if firstLine.Delimiter == "" {
			if len(txt.Lines) > 1 {
				panic(PartialLineNotLast)
			}
			return
		}

		// Convert buf's contents into a single line and store that.
		var lineCompletion *logpb.Text_Line
		if len(buf) == 1 {
			lineCompletion = buf[0]
		} else {
			lineCompletion = &logpb.Text_Line{
				Value:     make([]byte, 0, bufSize),
				Delimiter: firstLine.Delimiter,
			}
			for _, line := range buf {
				lineCompletion.Value = append(lineCompletion.Value, line.Value...)
			}
		}
		toCallback = append(toCallback, lineCompletion)
		buf, bufSize = nil, 0

		// Process the next lines, which should be all complete with at most one partial at the end.
		wholeLines := txt.Lines[1:]
		var lastPartialLine *logpb.Text_Line
		lastLineIdx := len(wholeLines) - 1
		if lastLineIdx >= 0 && wholeLines[lastLineIdx].Delimiter == "" {
			lastPartialLine = wholeLines[lastLineIdx]
			wholeLines = wholeLines[:lastLineIdx]
		}

		for _, line := range wholeLines {
			if line.Delimiter == "" {
				panic(PartialLineNotLast)
			}
			toCallback = append(toCallback, line)
		}

		curLogEntryBase.Content = &logpb.LogEntry_Text{
			Text: &logpb.Text{
				Lines: toCallback,
			},
		}
		curLogEntryBase.PrefixIndex++
		cb(curLogEntryBase)

		// If the last line is partial, record it.
		if lastPartialLine != nil {
			buf = []*logpb.Text_Line{lastPartialLine}
			bufSize = len(lastPartialLine.Value)
		}
		curLogEntryBase = nil
	}
}
