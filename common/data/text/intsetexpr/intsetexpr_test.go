// Copyright 2023 The LUCI Authors.
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

package intsetexpr

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestExpand(t *testing.T) {
	t.Parallel()

	Convey("Tokenizer works", t, func() {
		So(tokenize(""), ShouldHaveLength, 0)
		So(tokenize("abc"), ShouldResemble, []token{{TokRunes, "abc"}})
		So(tokenize("01234"), ShouldResemble, []token{{TokNum, "01234"}})
		So(tokenize("{},.."), ShouldResemble, []token{
			{TokLB, "{"},
			{TokRB, "}"},
			{TokComma, ","},
			{TokDots, ".."},
		})
		So(tokenize("{{{}}}"), ShouldResemble, []token{
			{TokRunes, "{"},
			{TokLB, "{"},
			{TokRunes, "}"},
			{TokRB, "}"},
		})
		So(tokenize("ab.{01..02,03}{{c}}de"), ShouldResemble, []token{
			{TokRunes, "ab"},
			{TokRunes, "."},
			{TokLB, "{"},
			{TokNum, "01"},
			{TokDots, ".."},
			{TokNum, "02"},
			{TokComma, ","},
			{TokNum, "03"},
			{TokRB, "}"},
			{TokRunes, "{"},
			{TokRunes, "c"},
			{TokRunes, "}"},
			{TokRunes, "de"},
		})
	})

	Convey("Expand works", t, func() {
		call := func(s string) []string {
			out, err := Expand(s)
			So(err, ShouldBeNil)
			return out
		}

		So(call(""), ShouldResemble, []string{""})
		So(call("abc"), ShouldResemble, []string{"abc"})
		So(call("abc{{}}"), ShouldResemble, []string{"abc{}"})

		So(call("a{1}b"), ShouldResemble, []string{"a1b"})
		So(call("{{a{1}b}}"), ShouldResemble, []string{"{a1b}"})
		So(call("a{1,2}b"), ShouldResemble, []string{"a1b", "a2b"})
		So(call("a{1..2}b"), ShouldResemble, []string{"a1b", "a2b"})
		So(call("a{1..2,3}b"), ShouldResemble, []string{"a1b", "a2b", "a3b"})
		So(call("a{1..2,3,4,8..9}b"), ShouldResemble, []string{"a1b", "a2b", "a3b", "a4b", "a8b", "a9b"})
		So(call("a{}b"), ShouldResemble, []string{"ab"})
		So(call("a{1,}b"), ShouldResemble, []string{"a1b"})
		So(call("a{1..2,}b"), ShouldResemble, []string{"a1b", "a2b"})

		So(call("a...{1..2}.b"), ShouldResemble, []string{"a...1.b", "a...2.b"})

		So(call("{1}"), ShouldResemble, []string{"1"})
		So(call("{}"), ShouldResemble, []string{""})

		// Zero padding works (but only when range number have the same length).
		So(call("a{01,02..03}b"), ShouldResemble, []string{"a01b", "a02b", "a03b"})
		So(call("a{1,02..03}b"), ShouldResemble, []string{"a1b", "a02b", "a03b"})
		So(call("a{001,02..003}b"), ShouldResemble, []string{"a001b", "a2b", "a3b"})
	})

	Convey("Expand handles errors", t, func() {
		call := func(s string) error {
			_, err := Expand(s)
			return err
		}

		So(call("}abc"), ShouldErrLike, `"}" must appear after "{"`)
		So(call("a{}b{}"), ShouldErrLike, `only one "{...}" section is allowed`)
		So(call("a{z}"), ShouldErrLike, `expecting a number or "}", got "z"`)
		So(call("a{12z}"), ShouldErrLike, `expecting ",", ".." or "}", got "z"`)
		So(call("a{1..}"), ShouldErrLike, `expecting a number, got "}"`)
		So(call("a{1..2z}"), ShouldErrLike, `expecting "," or "}", got "z"`)

		So(call("{10000000000000000000000000000000000000000}"), ShouldErrLike, `is too large`)
		So(call("{1..10000000000000000000000000000000000000000}"), ShouldErrLike, `is too large`)

		So(call("{2..1}"), ShouldErrLike, `bad range - 1 is not larger than 2`)
		So(call("{1..1}"), ShouldErrLike, `bad range - 1 is not larger than 1`)
		So(call("{1,1}"), ShouldErrLike, `the set is not in increasing order - 1 is not larger than 1`)
		So(call("{1..10,9,10}"), ShouldErrLike, `the set is not in increasing order - 9 is not larger than 10`)
	})
}
