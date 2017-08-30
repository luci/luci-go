// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package access

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestParsePattern(t *testing.T) {
	t.Parallel()

	Convey("ParsePattern", t, func() {
		cases := []struct {
			pattern  string
			expected []patternTuple
			err      string
		}{
			{
				pattern: "a/{b}",
				expected: []patternTuple{
					{
						kind:      "a",
						parameter: "b",
					},
				},
			},
			{
				pattern: "a/{a}",
				expected: []patternTuple{
					{
						kind:      "a",
						parameter: "a",
					},
				},
			},
			{
				pattern: "a/{a}/b/{b}",
				expected: []patternTuple{
					{
						kind:      "a",
						parameter: "a",
					},
					{
						kind:      "b",
						parameter: "b",
					},
				},
			},
			{
				pattern: "a",
				err:     "the number of slashes is not odd",
			},
			{
				pattern: "a/a",
				err:     `parameter "a" does not match regex`,
			},
			{
				pattern: "a/{A}",
				err:     `parameter "{A}" does not match regex`,
			},
			{
				pattern: "B/{b}",
				err:     `kind "B" does not match regex`,
			},
			{
				pattern: "a/{a}/a/{a}",
				err:     `duplicate kind "a"`,
			},
			{
				pattern: "a/{a}/b/{a}",
				err:     `duplicate parameter "a"`,
			},
		}

		for _, c := range cases {
			c := c
			Convey(c.pattern, func() {
				actual, err := parsePattern(c.pattern)
				So(actual, ShouldResemble, c.expected)
				if c.err == "" {
					So(err, ShouldBeNil)
				} else {
					So(err, ShouldErrLike, c.err)
				}
			})
		}
	})
}

func TestPatternNode(t *testing.T) {
	t.Parallel()

	res := func(pattern string) *Resource {
		r := &Resource{}
		r.Description.Pattern = pattern
		err := r.parseDescription()
		So(err, ShouldBeNil)
		return r
	}

	tree := patternNode{}

	Convey("add", t, func() {
		err := tree.add(res("a/{a}"))
		So(err, ShouldBeNil)
		So(tree.children, ShouldHaveLength, 1)
		So(tree.children, ShouldContainKey, "a")
		So(tree.children["a"].children, ShouldHaveLength, 0)

		err = tree.add(res("b/{b}"))
		So(err, ShouldBeNil)
		So(tree.children, ShouldHaveLength, 2)
		So(tree.children, ShouldContainKey, "b")

		err = tree.add(res("a/{a}/b/{b}"))
		So(err, ShouldBeNil)
		So(tree.children, ShouldHaveLength, 2)
		So(tree.children["a"].children, ShouldHaveLength, 1)
		So(tree.children["a"].children, ShouldContainKey, "b")

		err = tree.add(res("a/{b}"))
		So(err, ShouldErrLike, `a resource with kinds ["a"] already exists`)
		So(tree.children, ShouldHaveLength, 2)
		So(tree.children["a"].children, ShouldHaveLength, 1)
	})

	Convey("resolve", t, func() {
		tree.add(res("a/{a}"))
		tree.add(res("b/{b}"))
		tree.add(res("a/{a}/b/{b}"))

		res, args, err := tree.resolve("a/a")
		So(err, ShouldBeNil)
		So(res.Description.Pattern, ShouldEqual, "a/{a}")
		So(args, ShouldResemble, []string{"a"})

		res, args, err = tree.resolve("a/a%2Fb")
		So(err, ShouldBeNil)
		So(res.Description.Pattern, ShouldEqual, "a/{a}")
		So(args, ShouldResemble, []string{"a/b"})

		_, _, err = tree.resolve("a")
		So(err, ShouldErrLike, "invalid resource")

		_, _, err = tree.resolve("a/%ZZ")
		So(err, ShouldErrLike, "invalid arg")

		_, _, err = tree.resolve("c/c")
		So(err, ShouldErrLike, "invalid resource")
	})
}
