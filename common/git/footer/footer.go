// Copyright 2020 The LUCI Authors.
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

package footer

import (
	"regexp"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/strpair"
)

var (
	// Regexp pattern for git commit message footer.
	footerPattern = regexp.MustCompile(`^\s*([\w-]+): *(.*)$`)
	// Strings that won't be treated as footer keys.
	footerKeyIgnorelist = stringset.NewFromSlice("Http", "Https")
)

// NormalizeKey normalizes a git footer key string.
// It removes leading and trailing spaces and converts each segment (separated
// by `-`) to title case.
func NormalizeKey(footerKey string) string {
	segs := strings.Split(strings.TrimSpace(footerKey), "-")
	caser := cases.Title(language.English)
	for i, seg := range segs {
		segs[i] = caser.String(strings.ToLower(seg))
	}
	return strings.Join(segs, "-")
}

// ParseLine tries to extract a git footer from a commit message line.
// Returns a normalized key and value (with surrounding space trimmed) if
// the line represents a valid footer. Returns empty strings otherwise.
func ParseLine(line string) (string, string) {
	res := footerPattern.FindStringSubmatch(line)
	if len(res) == 3 {
		if key := NormalizeKey(res[1]); !footerKeyIgnorelist.Has(key) {
			return key, strings.TrimSpace(res[2])
		}
	}
	return "", ""
}

// ParseMessage extracts all footers from the footer lines of given message.
// A shorthand for `SplitLines` + `ParseLines`.
func ParseMessage(message string) strpair.Map {
	_, footerLines := SplitLines(message)
	return ParseLines(footerLines)
}

// ParseLines extracts all footers from the given lines.
// Returns a multimap as a footer key may map to multiple values. The
// footer in a latter line takes precedence and shows up at the front of the
// value slice. Ideally, this function should be called with the `footerLines`
// part of the return values of `SplitLines`.
func ParseLines(lines []string) strpair.Map {
	ret := strpair.Map{}
	for i := len(lines) - 1; i >= 0; i-- {
		if k, v := ParseLine(lines[i]); k != "" {
			ret.Add(k, v)
		}
	}
	return ret
}

// SplitLines splits a commit message to non-footer and footer lines.
//
// Footer lines are all lines in the last paragraph of the message if it:
//   - contains at least one valid footer (it may contains lines that are not
//     valid footers in the middle).
//   - is not the only paragraph in the message.
//
// One exception is that if the last paragraph starts with text then followed
// by valid footers, footer lines will only contain all lines after the first
// valid footer, all the lines above will be included in non-footer lines and
// a new line will be appended to separate them from footer lines.
//
// The leading and trailing whitespaces (including new lines) of the given
// message will be trimmed before splitting.
func SplitLines(message string) (nonFooterLines, footerLines []string) {
	lines := strings.Split(strings.TrimSpace(message), "\n")
	var maybeFooterLines []string
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			break
		}
		if k, _ := ParseLine(line); k != "" {
			footerLines = append(footerLines, maybeFooterLines...)
			maybeFooterLines = maybeFooterLines[0:0]
			footerLines = append(footerLines, line)
		} else {
			maybeFooterLines = append(maybeFooterLines, line)
		}
	}
	if len(footerLines)+len(maybeFooterLines) == len(lines) {
		// The entire message is consists of footers which means those lines
		// are not footers.
		return lines, nil
	}

	nonFooterLines = lines[:len(lines)-len(footerLines)]
	reverse(footerLines)
	if len(maybeFooterLines) > 0 {
		// If there're some malformed lines leftover, add a new line to separate
		// them from valid footer lines.
		// This mutates `lines` slice but it's okay.
		nonFooterLines = append(nonFooterLines, "")
	}
	return
}

// reverse reverses a slice of strings in place.
func reverse(s []string) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}
