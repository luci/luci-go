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

// Package shell contains functions for quoting arguments to commands.
package shell

// QuoteUnix takes an array of strings representing individual arguments of a
// command and returns a space-delimited sequence of double-quoted string literals
// quoted correctly for any POSIX-compatible shell.
//
// It does not produce pretty output. QuoteUnix("a", "cool", "bear") becomes `"a" "cool" "bear"`, not
// `a cool bear`. This behavior simplifies the implementaiton, but also bypasses aliases if we are
// handing our string to a login shell.
//
// QuoteUnix(s) is a valid UTF-8 string if and only if s is a valid UTF-8 string.
// Proof of this fact is left as an exercise to the reader.
//
// When given a single argument s, QuoteUnix(s) quotes that argument alone.
//
// When given multiple arguments, QuoteUnix(...) produces a space delimited sequence of quoted
// strings, which is interpreted as a command with arguments by every POSIX-compatible shell.
//
// See https://pubs.opengroup.org/onlinepubs/9699919799/utilities/V3_chap02.html for more information.
func QuoteUnix(args ...string) string {
	out := []byte{}
	for i, s := range args {
		if i != 0 {
			out = append(out, ' ')
		}
		out = append(out, '"')
		// UTF-8 is self-synchronizing and furthermore, single-byte characters like a, $, and ` never
		// appear within multibyte characters. Therefore, iterating one byte at a time and inserting
		// backslashes precisely when we have to is correct.
		for _, ch := range []byte(s) {
			switch ch {
			// According to the documentation, inside a POSIX shell only five characters
			// may be quoted inside a double-quoted string. The extra character is the newline.
			// However, quoting or failing to quote the newline does not affect the semantics of
			// the string literal, so we do not do it to reduce the length of the output string.
			case '"', '\\', '$', '`':
				out = append(out, '\\', ch)
			default:
				out = append(out, ch)
			}
		}
		out = append(out, '"')
	}
	return string(out)
}
