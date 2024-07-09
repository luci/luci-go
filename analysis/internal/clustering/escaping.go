// Copyright 2022 The LUCI Authors.
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

package clustering

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// EscapeToGraphical escapes the input so that it only contains graphic unicode characters.
// Use on test names and failure reasons before presenting to any UI context.
func EscapeToGraphical(value string) string {
	quotedEscaped := strconv.QuoteToGraphic(value)
	// Remove starting end ending double-quotes.
	return quotedEscaped[1 : len(quotedEscaped)-1]
}

// QuoteForRule escapes the input to a double-quoted string literal suitable for use
// in a failure association rule.
func QuoteForRule(value string) string {
	// Double-quoted go string literals are also valid GoogleSQL string literals.
	quotedEscaped := strconv.QuoteToGraphic(value)

	// Also escape the "replacement character" rune (U+FFFD),
	// which although valid unicode and printable are not allowed
	// in failure association rules as it creates the appearance
	// of unicode errors.
	quotedEscaped = strings.ReplaceAll(quotedEscaped, string(utf8.RuneError), `\ufffd`)

	return quotedEscaped
}
