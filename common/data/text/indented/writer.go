// Copyright 2016 The LUCI Authors.
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

package indented

import (
	"bytes"
	"io"
)

// Writer inserts indentation before each line.
type Writer struct {
	io.Writer       // underlying writer.
	Level      int  // number of times \t must be inserted before each line.
	UseSpaces  bool // true iff Level is the number of spaces instead of tabs.
	Width      int  // number of whitespace characters in each level. Defaults to 1.
	insideLine bool
}

// Limit is the maximum value of Level.
const Limit = 256

var indentationTabs = bytes.Repeat([]byte{'\t'}, Limit)
var indentationSpaces = bytes.Repeat([]byte{' '}, Limit)

// prefix returns the indentation prefix.
func (w *Writer) prefix() []byte {
	if w.Level == 0 {
		return nil
	}

	buf := indentationTabs
	if w.UseSpaces {
		buf = indentationSpaces
	}

	width := w.Width
	if width <= 0 {
		width = 1
	}
	return buf[:width*w.Level]
}

// Write writes data inserting a newline before each line.
// Panics if w.Indent is outside of [0, Limit) range.
func (w *Writer) Write(data []byte) (n int, err error) {
	// Do not print indentation if there is no data.
	for len(data) > 0 {
		var printUntil int
		endsWithNewLine := false

		lineBeginning := !w.insideLine
		if data[0] == '\n' && lineBeginning {
			// This is a blank line. Do not indent it, just print as is.
			printUntil = 1
		} else {
			if lineBeginning {
				// Print indentation.
				w.Writer.Write(w.prefix())
				w.insideLine = true
			}

			lineEnd := bytes.IndexRune(data, '\n')
			if lineEnd < 0 {
				// Print the whole thing.
				printUntil = len(data)
			} else {
				// Print until the newline inclusive.
				printUntil = lineEnd + 1
				endsWithNewLine = true
			}
		}
		toPrint := data[:printUntil]
		data = data[printUntil:]

		// Assertion: none of the runes in toPrint
		// can be newline except the last rune.
		// The last rune is newline iff endsWithNewLine==true.

		m, err := w.Writer.Write(toPrint)
		n += m

		if m == len(toPrint) && endsWithNewLine {
			// We've printed the newline, so we are the line beginning again.
			w.insideLine = false
		}
		if err != nil {
			return n, err
		}
	}
	return n, nil
}
