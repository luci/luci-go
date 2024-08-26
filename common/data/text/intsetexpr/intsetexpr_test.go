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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestExpand(t *testing.T) {
	t.Parallel()

	ftt.Run("Tokenizer works", t, func(t *ftt.Test) {
		assert.Loosely(t, tokenize(""), should.HaveLength(0))
		assert.Loosely(t, tokenize("abc"), should.Resemble([]token{{TokRunes, "abc"}}))
		assert.Loosely(t, tokenize("01234"), should.Resemble([]token{{TokNum, "01234"}}))
		assert.Loosely(t, tokenize("{},.."), should.Resemble([]token{
			{TokLB, "{"},
			{TokRB, "}"},
			{TokComma, ","},
			{TokDots, ".."},
		}))
		assert.Loosely(t, tokenize("{{{}}}"), should.Resemble([]token{
			{TokRunes, "{"},
			{TokLB, "{"},
			{TokRunes, "}"},
			{TokRB, "}"},
		}))
		assert.Loosely(t, tokenize("ab.{01..02,03}{{c}}de"), should.Resemble([]token{
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
		}))
	})

	ftt.Run("Expand works", t, func(t *ftt.Test) {
		call := func(s string) []string {
			out, err := Expand(s)
			assert.Loosely(t, err, should.BeNil)
			return out
		}

		assert.Loosely(t, call(""), should.Resemble([]string{""}))
		assert.Loosely(t, call("abc"), should.Resemble([]string{"abc"}))
		assert.Loosely(t, call("abc{{}}"), should.Resemble([]string{"abc{}"}))

		assert.Loosely(t, call("a{1}b"), should.Resemble([]string{"a1b"}))
		assert.Loosely(t, call("{{a{1}b}}"), should.Resemble([]string{"{a1b}"}))
		assert.Loosely(t, call("a{1,2}b"), should.Resemble([]string{"a1b", "a2b"}))
		assert.Loosely(t, call("a{1..2}b"), should.Resemble([]string{"a1b", "a2b"}))
		assert.Loosely(t, call("a{1..2,3}b"), should.Resemble([]string{"a1b", "a2b", "a3b"}))
		assert.Loosely(t, call("a{1..2,3,4,8..9}b"), should.Resemble([]string{"a1b", "a2b", "a3b", "a4b", "a8b", "a9b"}))
		assert.Loosely(t, call("a{}b"), should.Resemble([]string{"ab"}))
		assert.Loosely(t, call("a{1,}b"), should.Resemble([]string{"a1b"}))
		assert.Loosely(t, call("a{1..2,}b"), should.Resemble([]string{"a1b", "a2b"}))

		assert.Loosely(t, call("a...{1..2}.b"), should.Resemble([]string{"a...1.b", "a...2.b"}))

		assert.Loosely(t, call("{1}"), should.Resemble([]string{"1"}))
		assert.Loosely(t, call("{}"), should.Resemble([]string{""}))

		// Zero padding works (but only when range number have the same length).
		assert.Loosely(t, call("a{01,02..03}b"), should.Resemble([]string{"a01b", "a02b", "a03b"}))
		assert.Loosely(t, call("a{1,02..03}b"), should.Resemble([]string{"a1b", "a02b", "a03b"}))
		assert.Loosely(t, call("a{001,02..003}b"), should.Resemble([]string{"a001b", "a2b", "a3b"}))
	})

	ftt.Run("Expand handles errors", t, func(t *ftt.Test) {
		call := func(s string) error {
			_, err := Expand(s)
			return err
		}

		assert.Loosely(t, call("}abc"), should.ErrLike(`"}" must appear after "{"`))
		assert.Loosely(t, call("a{}b{}"), should.ErrLike(`only one "{...}" section is allowed`))
		assert.Loosely(t, call("a{z}"), should.ErrLike(`expecting a number or "}", got "z"`))
		assert.Loosely(t, call("a{12z}"), should.ErrLike(`expecting ",", ".." or "}", got "z"`))
		assert.Loosely(t, call("a{1..}"), should.ErrLike(`expecting a number, got "}"`))
		assert.Loosely(t, call("a{1..2z}"), should.ErrLike(`expecting "," or "}", got "z"`))

		assert.Loosely(t, call("{10000000000000000000000000000000000000000}"), should.ErrLike(`is too large`))
		assert.Loosely(t, call("{1..10000000000000000000000000000000000000000}"), should.ErrLike(`is too large`))

		assert.Loosely(t, call("{2..1}"), should.ErrLike(`bad range - 1 is not larger than 2`))
		assert.Loosely(t, call("{1..1}"), should.ErrLike(`bad range - 1 is not larger than 1`))
		assert.Loosely(t, call("{1,1}"), should.ErrLike(`the set is not in increasing order - 1 is not larger than 1`))
		assert.Loosely(t, call("{1..10,9,10}"), should.ErrLike(`the set is not in increasing order - 9 is not larger than 10`))
	})
}
