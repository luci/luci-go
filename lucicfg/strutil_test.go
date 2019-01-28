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

package lucicfg

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestExpandIntSet(t *testing.T) {
	t.Parallel()

	Convey("Tokenizer works", t, func() {
		So(tokenize(""), ShouldHaveLength, 0)
		So(tokenize("abc"), ShouldResemble, []token{{TOK_RUNES, "abc"}})
		So(tokenize("01234"), ShouldResemble, []token{{TOK_NUM, "01234"}})
		So(tokenize("{},.."), ShouldResemble, []token{
			{TOK_LB, "{"},
			{TOK_RB, "}"},
			{TOK_COMMA, ","},
			{TOK_DOTS, ".."},
		})
		So(tokenize("{{{}}}"), ShouldResemble, []token{
			{TOK_RUNES, "{"},
			{TOK_LB, "{"},
			{TOK_RUNES, "}"},
			{TOK_RB, "}"},
		})
		So(tokenize("ab.{01..02,03}{{c}}de"), ShouldResemble, []token{
			{TOK_RUNES, "ab"},
			{TOK_RUNES, "."},
			{TOK_LB, "{"},
			{TOK_NUM, "01"},
			{TOK_DOTS, ".."},
			{TOK_NUM, "02"},
			{TOK_COMMA, ","},
			{TOK_NUM, "03"},
			{TOK_RB, "}"},
			{TOK_RUNES, "{"},
			{TOK_RUNES, "c"},
			{TOK_RUNES, "}"},
			{TOK_RUNES, "de"},
		})
	})

	Convey("expandIntSet works", t, func() {
		call := func(s string) []string {
			out, err := expandIntSet(s)
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

	Convey("expandIntSet handles errors", t, func() {
		call := func(s string) error {
			_, err := expandIntSet(s)
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
