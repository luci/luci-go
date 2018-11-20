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

// assertGetText panics if the passed LogEntry does not contain Text data, or returns it.
func assertGetText(le *logpb.LogEntry) *logpb.Text {
	if txt := le.GetText(); txt == nil {
		panic(fmt.Sprintf(
				"expected to pass Text to getWrappedCallback for Text, got %v",
				le.Content,
		))
	} else {
		return txt
	}
}

// lineInfo stores length and delimiter of a line.
type lineInfo struct {
	size int
	delimIndex int
	delim string
}

// getWrappedCallback wraps a passed callback meant to be called at the ends of Text lines so that
// it is actually called at the end of Text lines.
// Does not wrap callback to guarantee being called at the end of *every* Text line.
func getWrappedTextCallback(cb bundler.StreamChunkCallback) bundler.StreamChunkCallback {
	if cb == nil {
		return nil
	}

	var buf []*logpb.Text_Line  // array of pointers into all the Lines per Text
	var lineInfos []*lineInfo   // array of info about complete lines
	curLineInfo := &lineInfo{}  // info about current, possibly incomplete, line
	var leFull *logpb.LogEntry
	return func(le *logpb.LogEntry) {
		if le == nil {
			return
		}
		txt := assertGetText(le)

		if leFull == nil {
			leFull = &(*le)
		}

		// The expected case is that each Line is complete and that the buffer is empty (which happens
		// unless we're aggressively flushing), but we don't know this unless we check every line.
		canCallbackDirectly := buf == nil
		for _, curLine := range txt.Lines {
			curLineInfo.size += len(curLine.Value)
			buf = append(buf, curLine)

			if curLine.Delimiter != "" {
				// We have a complete line, so save the line info.
				curLineInfo.delimIndex = len(buf)-1
				curLineInfo.delim = curLine.Delimiter
				lineInfos = append(lineInfos, curLineInfo)
				curLineInfo = &lineInfo{}
			} else {
				// We have an incomplete line, so don't just call the callback.
				canCallbackDirectly = false
			}
		}

		if canCallbackDirectly {
			buf, lineInfos, curLineInfo, leFull = nil, nil, &lineInfo{}, nil
			cb(le)
			return
		}

		// If there are no delimiters, nothing to do.
		if lineInfos == nil {
			return
		}

		// Reconstruct the Text up until the last delimiter available.
		var lineInd, textInd int
		bytes := make([]byte, 0, lineInfos[lineInd].size)
		lines := make([]*logpb.Text_Line, 0, len(lineInfos))

		for i, textLine := range buf {
			bytes = append(bytes, textLine.Value...)

			// If we've reached the expected line size, push it into the LogEntry's Lines.
			if i == lineInfos[lineInd].delimIndex {
				lines = append(lines, &logpb.Text_Line{
					Value: bytes,
					Delimiter: lineInfos[lineInd].delim,
				})
				lineInd++

				if lineInd >= len(lineInfos) {
					textInd = i
					break
				}

				bytes = make([]byte, 0, lineInfos[lineInd].size)
			}
		}

		leFull.Content = &logpb.LogEntry_Text{
			Text: &logpb.Text{
				Lines: lines,
			},
		}

		// Trim out the processed part of the buffer.
		// Keep the curLineInfo though, as we'll need it for the next LogEntry with a delimiter.
		buf, lineInfos = buf[textInd+1:], nil
		cb(leFull)
		leFull = nil
	}
}
