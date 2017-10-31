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

	. "github.com/smartystreets/goconvey/convey"
)

func TestWriter(t *testing.T) {
	t.Parallel()

	Convey("Writer", t, func() {
		var buf bytes.Buffer
		w := &Writer{Writer: &buf}

		print := func(s string) {
			fmt.Fprint(w, s)
		}
		expect := func(s string) {
			So(buf.String(), ShouldEqual, s)
		}

		Convey("Without indentation", func() {
			Convey("Print once", func() {
				Convey("Line", func() {
					print("abc\n")
					expect("abc\n")
				})
				Convey("Unfinished line", func() {
					print("abc")
					expect("abc")
				})
				Convey("Blank line", func() {
					print("\n")
					expect("\n")
				})
				Convey("Line and empty", func() {
					print("abc\nabc\n")
					print("")
					expect("abc\nabc\n")
				})
				Convey("Two lines", func() {
					print("abc\nabc\n")
					expect("abc\nabc\n")
				})
				Convey("Line and unfinished", func() {
					print("abc\nabc")
					expect("abc\nabc")
				})
			})
			Convey("Print twice", func() {
				Convey("Line", func() {
					print("abc\n")
					print("def\n")
					expect("abc\ndef\n")
				})
				Convey("Unfinished line", func() {
					print("abc")
					print("def")
					expect("abcdef")
				})
				Convey("Blank line", func() {
					print("\n")
					print("\n")
					expect("\n\n")
				})
				Convey("Line and empty", func() {
					print("abc\nabc\n")
					print("")
					print("def\ndef\n")
					print("")
					expect("abc\nabc\ndef\ndef\n")
				})
				Convey("Two lines", func() {
					print("abc\nabc\n")
					print("def\ndef\n")
					expect("abc\nabc\ndef\ndef\n")
				})
				Convey("Line and unfinished", func() {
					print("abc\nabc")
					print("def\ndef")
					expect("abc\nabcdef\ndef")
				})
			})
		})

		Convey("With indentation", func() {
			w.Level++

			Convey("Print once", func() {
				Convey("Line", func() {
					print("abc\n")
					expect("\tabc\n")
				})
				Convey("Unfinished line", func() {
					print("abc")
					expect("\tabc")
				})
				Convey("Blank line", func() {
					print("\n")
					expect("\n")
				})
				Convey("Line and empty", func() {
					print("abc\nabc\n")
					print("")
					expect("\tabc\n\tabc\n")
				})
				Convey("Two lines", func() {
					print("abc\nabc\n")
					expect("\tabc\n\tabc\n")
				})
				Convey("Line and unfinished", func() {
					print("abc\nabc")
					expect("\tabc\n\tabc")
				})
			})
			Convey("Print twice", func() {
				Convey("Line", func() {
					print("abc\n")
					print("def\n")
					expect("\tabc\n\tdef\n")
				})
				Convey("Unfinished line", func() {
					print("abc")
					print("def")
					expect("\tabcdef")
				})
				Convey("Blank line", func() {
					print("\n")
					print("\n")
					expect("\n\n")
				})
				Convey("Line and empty", func() {
					print("abc\nabc\n")
					print("")
					print("def\ndef\n")
					print("")
					expect("\tabc\n\tabc\n\tdef\n\tdef\n")
				})
				Convey("Two lines", func() {
					print("abc\nabc\n")
					print("def\ndef\n")
					expect("\tabc\n\tabc\n\tdef\n\tdef\n")
				})
				Convey("Line and unfinished", func() {
					print("abc\nabc")
					print("def\ndef")
					expect("\tabc\n\tabcdef\n\tdef")
				})
			})
		})
	})
}
