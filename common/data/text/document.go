// Copyright 2019 The LUCI Authors.
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

package text

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Doc transforms doc:
//
//  1. Strips leading and trailing blank lines.
//  2. Removes common '\t' indentation.
//  3. Replaces '\n' not-followed by whitespace with ' '.
//  4. Guarantees '\n' in the end.
//
// See example.
//
// This function is not fast.
func Doc(doc string) string {
	lines := strings.Split(doc, "\n")

	// Strip leading blank lines.
	for len(lines) > 0 && isBlank(lines[0]) {
		lines = lines[1:]
	}
	// Strip trailing blank lines.
	for len(lines) > 0 && isBlank(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}

	// Compute common TAB indentation.
	commonIndent := -1
	for _, line := range lines {
		if isBlank(line) {
			continue
		}

		indent := 0
		for indent < len(line) && line[indent] == '\t' {
			indent++
		}
		if commonIndent == -1 || commonIndent > indent {
			commonIndent = indent
		}
	}
	if commonIndent == -1 {
		commonIndent = 0
	}

	// Combine lines, but replace ('\n' followed by non-whitespace) with one
	// space.
	ret := &strings.Builder{}
	ret.Grow(len(doc))
	newParagraph := true
	for _, line := range lines {
		if isBlank(line) {
			// This is a blank line between two paragraphs.
			// Separate them with two '\n'.
			ret.WriteString("\n\n")
			newParagraph = true
			continue
		}

		line = line[commonIndent:]

		switch first, _ := utf8.DecodeRuneInString(line); {
		// If the line starts with whitespace, preserve the structure.
		case unicode.IsSpace(first):
			ret.WriteRune('\n')

		// Otherwise replace '\n' with ' ', unless it is a new paragraph.
		case !newParagraph:
			ret.WriteRune(' ')
		}

		ret.WriteString(line)
		newParagraph = false
	}
	ret.WriteRune('\n')
	return ret.String()
}

func isBlank(line string) bool {
	return strings.TrimSpace(line) == ""
}
