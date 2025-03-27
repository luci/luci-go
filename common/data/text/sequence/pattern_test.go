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

package sequence_test

import (
	"testing"

	"go.chromium.org/luci/common/data/text/sequence"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestAutoPattern(t *testing.T) {
	t.Parallel()

	ftt.Run(`NewPattern`, t, func(t *ftt.Test) {
		t.Run(`works`, func(t *ftt.Test) {
			t.Run(`with empty input`, func(t *ftt.Test) {
				pat, err := sequence.NewPattern()
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pat, should.BeNil)
			})

			t.Run(`with a literal`, func(t *ftt.Test) {
				pat, err := sequence.NewPattern("hello")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pat, should.Match(sequence.Pattern{sequence.LiteralMatcher("hello")}))
			})

			t.Run(`with a reserved literal`, func(t *ftt.Test) {
				pat, err := sequence.NewPattern("=^", "==")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pat, should.Match(sequence.Pattern{
					sequence.LiteralMatcher("^"),
					sequence.LiteralMatcher("="),
				}))
			})

			t.Run(`with string regexp`, func(t *ftt.Test) {
				pat, err := sequence.NewPattern("/hello/")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pat, should.HaveLength(1))
				assert.Loosely(t, pat[0], should.StaticallyHaveSameTypeAs(sequence.RegexpMatcher{}))
				rem := pat[0].(sequence.RegexpMatcher)
				assert.Loosely(t, rem.R.String(), should.Equal("hello"))
			})

			t.Run(`with string ellipsis`, func(t *ftt.Test) {
				pat, err := sequence.NewPattern("...")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pat, should.Match(sequence.Pattern{sequence.Ellipsis}))
			})

			t.Run(`with string edges`, func(t *ftt.Test) {
				pat, err := sequence.NewPattern("^", "$")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pat, should.Match(sequence.Pattern{sequence.Edge, sequence.Edge}))
			})
		})

		t.Run(`fails`, func(t *ftt.Test) {
			t.Run(`with a bad regexp`, func(t *ftt.Test) {
				_, err := sequence.NewPattern("/unclosed(/")
				assert.Loosely(t, err, should.ErrLike("invalid regexp (i=0)"))
			})

			t.Run(`with out of place edges`, func(t *ftt.Test) {
				_, err := sequence.NewPattern("$", "something")
				assert.Loosely(t, err, should.ErrLike("cannot use `$` for Edge except at end"))

				_, err = sequence.NewPattern("something", "^")
				assert.Loosely(t, err, should.ErrLike("cannot use `^` for Edge except at beginning"))
			})

			t.Run(`with many ellipsis`, func(t *ftt.Test) {
				_, err := sequence.NewPattern("...", "...")
				assert.Loosely(t, err, should.ErrLike("cannot have multiple Ellipsis in a row (i=1)"))
			})
		})
	})
}

func TestPatternIn(t *testing.T) {
	t.Parallel()

	m := func(tokens ...string) sequence.Pattern {
		ret, err := sequence.NewPattern(tokens...)
		if err != nil {
			panic(err)
		}
		return ret
	}

	ftt.Run(`Pattern.In`, t, func(t *ftt.Test) {
		t.Run(`works`, func(t *ftt.Test) {
			t.Run(`no match`, func(t *ftt.Test) {
				assert.Loosely(t, m("narp").In("foo", "bar", "baz"), should.BeFalse)
			})

			t.Run(`finds single strings`, func(t *ftt.Test) {
				assert.Loosely(t, m("foo").In("foo", "bar", "baz"), should.BeTrue)
				assert.Loosely(t, m("bar").In("foo", "bar", "baz"), should.BeTrue)
				assert.Loosely(t, m("baz").In("foo", "bar", "baz"), should.BeTrue)

				assert.Loosely(t, m("narp").In("foo", "bar", "baz"), should.BeFalse)
			})

			t.Run(`finds single regex`, func(t *ftt.Test) {
				assert.Loosely(t, m("/ba./").In("foo", "bar", "baz"), should.BeTrue)
				assert.Loosely(t, m("/a/").In("foo", "bar", "baz"), should.BeTrue)
				assert.Loosely(t, m("/z$/").In("foo", "bar", "baz"), should.BeTrue)
				assert.Loosely(t, m("/^bar$/").In("foo", "bar", "baz"), should.BeTrue)

				assert.Loosely(t, m("/^a$/").In("foo", "bar", "baz"), should.BeFalse)
			})

			t.Run(`finds string sequence`, func(t *ftt.Test) {
				assert.Loosely(t, m().In("foo", "bar", "baz"), should.BeTrue)
				assert.Loosely(t, m("foo", "bar").In("foo", "bar", "baz"), should.BeTrue)
				assert.Loosely(t, m("bar", "baz").In("foo", "bar", "baz"), should.BeTrue)
				assert.Loosely(t, m("foo", "bar", "baz").In("foo", "bar", "baz"), should.BeTrue)

				assert.Loosely(t, m("foo", "baz").In("foo", "bar", "baz"), should.BeFalse)
			})

			t.Run(`finds mixed sequence`, func(t *ftt.Test) {
				assert.Loosely(t, m("/.o./", "bar", "/z/").In("foo", "bar", "baz"), should.BeTrue)

				assert.Loosely(t, m("/f/", "/z/", "/r/").In("foo", "bar", "baz"), should.BeFalse)
			})

			t.Run(`finds ellipsis sequence`, func(t *ftt.Test) {
				assert.Loosely(t, m("a", "foo", "...", "bar").In(
					"a", "foo", "narp", "bar"), should.BeTrue)

				assert.Loosely(t, m("foo", "...", "bar", "a").In(
					"foo", "bar", "a"), should.BeTrue)

				assert.Loosely(t, m("foo", "...", "bar", "...", "/^a/").In(
					"foo", "narp", "bar", "tarp", "stuff", "aardvark"), should.BeTrue)

				assert.Loosely(t, m("foo", "...", "bar").In(
					"foo", "narp"), should.BeFalse)

				assert.Loosely(t, m("foo", "...").In(
					"foo", "narp"), should.BeTrue)

				assert.Loosely(t, m("...").In(
					"foo", "narp"), should.BeTrue)

				assert.Loosely(t, m("...").In(), should.BeTrue)

				assert.Loosely(t, m("^", "foo", "...").In(
					"narp", "foo"), should.BeFalse)

				assert.Loosely(t, m("...", "narp").In(
					"foo", "narp"), should.BeTrue)

				assert.Loosely(t, m("...", "florp").In(
					"foo", "narp"), should.BeFalse)
			})

			t.Run(`respects Edges`, func(t *ftt.Test) {
				assert.Loosely(t, m("^", "foo", "...", "bar").In(
					"foo", "narp", "bar"), should.BeTrue)

				assert.Loosely(t, m("foo", "...", "bar", "$").In(
					"a", "foo", "narp", "bar"), should.BeTrue)

				assert.Loosely(t, m("a", "b", "$").In(
					"extra", "a", "b"), should.BeTrue)

				assert.Loosely(t, m("^", "foo", "...", "bar").In(
					"a", "foo", "narp", "bar"), should.BeFalse)

				assert.Loosely(t, m("foo", "...", "bar", "$").In(
					"a", "foo", "narp", "bar", "stuff"), should.BeFalse)
			})
		})

		t.Run(`fails`, func(t *ftt.Test) {
			t.Run(`bad Ellipsis`, func(t *ftt.Test) {
				assert.Loosely(t, func() {
					sequence.Pattern{sequence.Ellipsis, sequence.Ellipsis}.In("something")
				}, should.PanicLike("cannot have multiple Ellipsis in a row"))
			})

			t.Run(`bad Edge`, func(t *ftt.Test) {
				assert.Loosely(t, func() {
					sequence.Pattern{
						sequence.Edge,
						sequence.LiteralMatcher("stuff"),
						sequence.Edge,
						sequence.LiteralMatcher("more stuff"),
					}.In("something")
				}, should.PanicLike("cannot have Edge in the middle of a Pattern"))
			})
		})
	})

}
