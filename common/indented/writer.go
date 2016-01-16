// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package indented

import (
	"bytes"
	"io"
)

// Writer inserts indentation before each line.
type Writer struct {
	io.Writer      // underlying writer.
	Level      int // number of times \t must be inserted before each line.
	insideLine bool
}

var indentation []byte

func init() {
	indentation = make([]byte, 256)
	for i := range indentation {
		indentation[i] = '\t'
	}
}

// Write writes data inserting a newline before each line.
// Panics if w.Indent is outside of [0, 256) range.
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
				w.Writer.Write(indentation[:w.Level])
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
