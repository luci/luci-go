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

import "unicode/utf8"

// MaxMessageLength is the max message length for Gerrit as of Jun 2020
// based on error messages.
const MaxMessageLength = 16384

// PlaceHolder is added to the message when the length of the message
// exceeds `MaxMessageLength` and human message gets truncated.
const PlaceHolder = "\n...[truncated too long message]"

// TruncateMessage truncates the message and appends `PlaceHolder` to conform
// `MaxMessageLength`.
//
// It will make sure the last rune is a valid utf-8 character.
func TruncateMessage(msg string) string {
	if len(msg) <= MaxMessageLength {
		return msg
	}
	msg = msg[:MaxMessageLength-len(PlaceHolder)]
	var last int
	for i := len(msg) - 1; i >= 0; i-- {
		if r, size := utf8.DecodeRuneInString(msg[i:]); r != utf8.RuneError {
			last = i + size
			break
		}
	}
	if last > 0 {
		msg = msg[:last]
	}
	return msg + PlaceHolder
}
