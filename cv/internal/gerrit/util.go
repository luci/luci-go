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

package gerrit

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxMessageLength is the max message length for Gerrit as of Jun 2020
// based on error messages.
const MaxMessageLength = 16384

// PlaceHolder is added to the message when the length of the message
// exceeds `MaxMessageLength` and human message gets truncated.
const PlaceHolder = "\n...[truncated too long message]"

// TruncateMessage truncates the message and appends `PlaceHolder` so that
// the string doesn't exceed `MaxMessageLength`.
//
// If the input message is a valid utf-8 string, the result string will also
// be a valid utf-8 string
func TruncateMessage(msg string) string {
	return truncate(msg, MaxMessageLength)
}

func truncate(msg string, maxLen int) string {
	if len(msg) <= maxLen {
		return msg
	}
	ret := msg[:maxLen-len(PlaceHolder)]
	if utf8.ValidString(msg) {
		end := len(ret)
		start := end - 1
		for ; start >= 0; start-- {
			if !utf8.RuneStart(ret[start]) {
				continue
			}
			if r, size := utf8.DecodeRuneInString(ret[start:end]); r != utf8.RuneError {
				ret = ret[:start+size]
				break
			}
			// non-first bytes for utf-8 encoding always have the top two bits
			// set to 10. So a valid start bytes will never be in the middle of
			// a valid utf-8 character.
			end = start
		}
	}
	return ret + PlaceHolder
}

// Tag constructs a CV specific Gerrit Tag for SetReview request.
//
// See: https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#review-input
// Panics if name is not provided or name contains "~". suffix is optional.
func Tag(name string, suffix string) string {
	switch {
	case name == "":
		panic(errors.New("tag name MUST NOT be empty"))
	case strings.Contains(name, "~"):
		panic(errors.New("tag name MUST NOT contain `~`"))
	case suffix != "":
		return fmt.Sprintf("autogenerated:luci-cv:%s~%s", name, suffix)
	default:
		return fmt.Sprintf("autogenerated:luci-cv:%s", name)
	}
}
