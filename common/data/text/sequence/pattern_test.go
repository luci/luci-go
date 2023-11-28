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

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/data/text/sequence"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestAutoPattern(t *testing.T) {
	t.Parallel()

	Convey(`NewPattern`, t, func() {
		Convey(`works`, func() {
			Convey(`with empty input`, func() {
				pat, err := sequence.NewPattern()
				So(err, ShouldBeNil)
				So(pat, ShouldBeNil)
			})

			Convey(`with a literal`, func() {
				pat, err := sequence.NewPattern("hello")
				So(err, ShouldBeNil)
				So(pat, ShouldResemble, sequence.Pattern{sequence.LiteralMatcher("hello")})
			})

			Convey(`with a reserved literal`, func() {
				pat, err := sequence.NewPattern("=^", "==")
				So(err, ShouldBeNil)
				So(pat, ShouldResemble, sequence.Pattern{
					sequence.LiteralMatcher("^"),
					sequence.LiteralMatcher("="),
				})
			})

			Convey(`with string regexp`, func() {
				pat, err := sequence.NewPattern("/hello/")
				So(err, ShouldBeNil)
				So(pat, ShouldHaveLength, 1)
				So(pat[0], ShouldHaveSameTypeAs, sequence.RegexpMatcher{})
				rem := pat[0].(sequence.RegexpMatcher)
				So(rem.R.String(), ShouldEqual, "hello")
			})

			Convey(`with string ellipsis`, func() {
				pat, err := sequence.NewPattern("...")
				So(err, ShouldBeNil)
				So(pat, ShouldResemble, sequence.Pattern{sequence.Ellipsis})
			})

			Convey(`with string edges`, func() {
				pat, err := sequence.NewPattern("^", "$")
				So(err, ShouldBeNil)
				So(pat, ShouldResemble, sequence.Pattern{sequence.Edge, sequence.Edge})
			})
		})

		Convey(`fails`, func() {
			Convey(`with a bad regexp`, func() {
				_, err := sequence.NewPattern("/unclosed(/")
				So(err, ShouldErrLike, "invalid regexp (i=0)")
			})

			Convey(`with out of place edges`, func() {
				_, err := sequence.NewPattern("$", "something")
				So(err, ShouldErrLike, "cannot use `$` for Edge except at end")

				_, err = sequence.NewPattern("something", "^")
				So(err, ShouldErrLike, "cannot use `^` for Edge except at beginning")
			})

			Convey(`with many ellipsis`, func() {
				_, err := sequence.NewPattern("...", "...")
				So(err, ShouldErrLike, "cannot have multiple Ellipsis in a row (i=1)")
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

	Convey(`Pattern.In`, t, func() {
		Convey(`works`, func() {
			Convey(`no match`, func() {
				So(m("narp").In("foo", "bar", "baz"), ShouldBeFalse)
			})

			Convey(`finds single strings`, func() {
				So(m("foo").In("foo", "bar", "baz"), ShouldBeTrue)
				So(m("bar").In("foo", "bar", "baz"), ShouldBeTrue)
				So(m("baz").In("foo", "bar", "baz"), ShouldBeTrue)

				So(m("narp").In("foo", "bar", "baz"), ShouldBeFalse)
			})

			Convey(`finds single regex`, func() {
				So(m("/ba./").In("foo", "bar", "baz"), ShouldBeTrue)
				So(m("/a/").In("foo", "bar", "baz"), ShouldBeTrue)
				So(m("/z$/").In("foo", "bar", "baz"), ShouldBeTrue)
				So(m("/^bar$/").In("foo", "bar", "baz"), ShouldBeTrue)

				So(m("/^a$/").In("foo", "bar", "baz"), ShouldBeFalse)
			})

			Convey(`finds string sequence`, func() {
				So(m().In("foo", "bar", "baz"), ShouldBeTrue)
				So(m("foo", "bar").In("foo", "bar", "baz"), ShouldBeTrue)
				So(m("bar", "baz").In("foo", "bar", "baz"), ShouldBeTrue)
				So(m("foo", "bar", "baz").In("foo", "bar", "baz"), ShouldBeTrue)

				So(m("foo", "baz").In("foo", "bar", "baz"), ShouldBeFalse)
			})

			Convey(`finds mixed sequence`, func() {
				So(m("/.o./", "bar", "/z/").In("foo", "bar", "baz"), ShouldBeTrue)

				So(m("/f/", "/z/", "/r/").In("foo", "bar", "baz"), ShouldBeFalse)
			})

			Convey(`finds ellipsis sequence`, func() {
				So(m("a", "foo", "...", "bar").In(
					"a", "foo", "narp", "bar"), ShouldBeTrue)

				So(m("foo", "...", "bar", "a").In(
					"foo", "bar", "a"), ShouldBeTrue)

				So(m("foo", "...", "bar", "...", "/^a/").In(
					"foo", "narp", "bar", "tarp", "stuff", "aardvark"), ShouldBeTrue)

				So(m("foo", "...", "bar").In(
					"foo", "narp"), ShouldBeFalse)
			})

			Convey(`respects Edges`, func() {
				So(m("^", "foo", "...", "bar").In(
					"foo", "narp", "bar"), ShouldBeTrue)

				So(m("foo", "...", "bar", "$").In(
					"a", "foo", "narp", "bar"), ShouldBeTrue)

				So(m("a", "b", "$").In(
					"extra", "a", "b"), ShouldBeTrue)

				So(m("^", "foo", "...", "bar").In(
					"a", "foo", "narp", "bar"), ShouldBeFalse)

				So(m("foo", "...", "bar", "$").In(
					"a", "foo", "narp", "bar", "stuff"), ShouldBeFalse)

			})
		})

		Convey(`fails`, func() {
			Convey(`bad Ellipsis`, func() {
				So(func() {
					sequence.Pattern{sequence.Ellipsis, sequence.Ellipsis}.In("something")
				}, ShouldPanicLike, "cannot have multiple Ellipsis in a row")
			})

			Convey(`bad Edge`, func() {
				So(func() {
					sequence.Pattern{
						sequence.Edge,
						sequence.LiteralMatcher("stuff"),
						sequence.Edge,
						sequence.LiteralMatcher("more stuff"),
					}.In("something")
				}, ShouldPanicLike, "cannot have Edge in the middle of a Pattern")
			})
		})
	})

}
