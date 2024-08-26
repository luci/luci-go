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
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestWriter(t *testing.T) {
	t.Parallel()

	ftt.Run("Writer", t, func(t *ftt.Test) {
		var buf bytes.Buffer
		w := &Writer{Writer: &buf}

		print := func(s string) {
			fmt.Fprint(w, s)
		}
		expect := func(s string) {
			assert.Loosely(t, buf.String(), should.Equal(s))
		}

		t.Run("Without indentation", func(t *ftt.Test) {
			t.Run("Print once", func(t *ftt.Test) {
				t.Run("Line", func(t *ftt.Test) {
					print("abc\n")
					expect("abc\n")
				})
				t.Run("Unfinished line", func(t *ftt.Test) {
					print("abc")
					expect("abc")
				})
				t.Run("Blank line", func(t *ftt.Test) {
					print("\n")
					expect("\n")
				})
				t.Run("Line and empty", func(t *ftt.Test) {
					print("abc\nabc\n")
					print("")
					expect("abc\nabc\n")
				})
				t.Run("Two lines", func(t *ftt.Test) {
					print("abc\nabc\n")
					expect("abc\nabc\n")
				})
				t.Run("Line and unfinished", func(t *ftt.Test) {
					print("abc\nabc")
					expect("abc\nabc")
				})
			})
			t.Run("Print twice", func(t *ftt.Test) {
				t.Run("Line", func(t *ftt.Test) {
					print("abc\n")
					print("def\n")
					expect("abc\ndef\n")
				})
				t.Run("Unfinished line", func(t *ftt.Test) {
					print("abc")
					print("def")
					expect("abcdef")
				})
				t.Run("Blank line", func(t *ftt.Test) {
					print("\n")
					print("\n")
					expect("\n\n")
				})
				t.Run("Line and empty", func(t *ftt.Test) {
					print("abc\nabc\n")
					print("")
					print("def\ndef\n")
					print("")
					expect("abc\nabc\ndef\ndef\n")
				})
				t.Run("Two lines", func(t *ftt.Test) {
					print("abc\nabc\n")
					print("def\ndef\n")
					expect("abc\nabc\ndef\ndef\n")
				})
				t.Run("Line and unfinished", func(t *ftt.Test) {
					print("abc\nabc")
					print("def\ndef")
					expect("abc\nabcdef\ndef")
				})
			})
		})

		t.Run("With indentation", func(t *ftt.Test) {
			w.Level++

			t.Run("Print once", func(t *ftt.Test) {
				t.Run("Line", func(t *ftt.Test) {
					print("abc\n")
					expect("\tabc\n")
				})
				t.Run("Unfinished line", func(t *ftt.Test) {
					print("abc")
					expect("\tabc")
				})
				t.Run("Blank line", func(t *ftt.Test) {
					print("\n")
					expect("\n")
				})
				t.Run("Line and empty", func(t *ftt.Test) {
					print("abc\nabc\n")
					print("")
					expect("\tabc\n\tabc\n")
				})
				t.Run("Two lines", func(t *ftt.Test) {
					print("abc\nabc\n")
					expect("\tabc\n\tabc\n")
				})
				t.Run("Line and unfinished", func(t *ftt.Test) {
					print("abc\nabc")
					expect("\tabc\n\tabc")
				})
			})
			t.Run("Print twice", func(t *ftt.Test) {
				t.Run("Line", func(t *ftt.Test) {
					print("abc\n")
					print("def\n")
					expect("\tabc\n\tdef\n")
				})
				t.Run("Unfinished line", func(t *ftt.Test) {
					print("abc")
					print("def")
					expect("\tabcdef")
				})
				t.Run("Blank line", func(t *ftt.Test) {
					print("\n")
					print("\n")
					expect("\n\n")
				})
				t.Run("Line and empty", func(t *ftt.Test) {
					print("abc\nabc\n")
					print("")
					print("def\ndef\n")
					print("")
					expect("\tabc\n\tabc\n\tdef\n\tdef\n")
				})
				t.Run("Two lines", func(t *ftt.Test) {
					print("abc\nabc\n")
					print("def\ndef\n")
					expect("\tabc\n\tabc\n\tdef\n\tdef\n")
				})
				t.Run("Line and unfinished", func(t *ftt.Test) {
					print("abc\nabc")
					print("def\ndef")
					expect("\tabc\n\tabcdef\n\tdef")
				})
			})
		})
	})
}
