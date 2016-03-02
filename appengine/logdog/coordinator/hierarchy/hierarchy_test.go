// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hierarchy

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/logdog/types"

	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestHierarchy(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c, _ := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		c = memory.Use(c)

		di := ds.Get(c)
		tds := ds.Get(c).Testable()

		norm := func(l *List) []string {
			result := make([]string, len(l.Comp))
			for i, e := range l.Comp {
				result[i] = e.Name
				if e.Stream != "" {
					result[i] += "$"
				}
			}
			return result
		}

		req := func(r Request) []string {
			l, err := Get(di, r)
			if err != nil {
				panic(err)
			}
			return norm(l)
		}

		Convey(`Get will return nothing when no components are registered.`, func() {
			h, err := Get(di, Request{})
			So(err, ShouldBeNil)
			So(h, ShouldResemble, &List{})

			h, err = Get(di, Request{Base: "foo/bar"})
			So(err, ShouldBeNil)
			So(h, ShouldResemble, &List{Base: "foo/bar"})

			h, err = Get(di, Request{Recursive: true})
			So(err, ShouldBeNil)
			So(h, ShouldResemble, &List{})

			h, err = Get(di, Request{Base: "foo/bar", Recursive: true})
			So(err, ShouldBeNil)
			So(h, ShouldResemble, &List{Base: "foo/bar"})
		})

		Convey(`Can register a hierarchy of name components.`, func() {
			for _, p := range []types.StreamPath{
				"foo/+/baz",
				"foo/+/qux",
				"foo/+/qux",
				"foo/+/bar",
				"foo/+/bar/baz",
				"foo/bar/+/baz",
				"bar/+/baz",
				"bar/+/baz/qux",
			} {
				So(Put(ds.Get(c), p), ShouldBeNil)
			}
			tds.CatchupIndexes()

			Convey(`Can list the hierarchy immediate paths.`, func() {
				list := func(p string) []string {
					return req(Request{Base: p})
				}

				So(list(""), ShouldResemble, []string{"bar", "foo"})
				So(list("foo"), ShouldResemble, []string{"+", "bar"})
				So(list("foo/+"), ShouldResemble, []string{"bar$", "baz$", "qux$", "bar"})
				So(list("foo/+/bar"), ShouldResemble, []string{"baz$"})
				So(list("foo/bar"), ShouldResemble, []string{"+"})
				So(list("foo/bar/+"), ShouldResemble, []string{"baz$"})
				So(list("bar"), ShouldResemble, []string{"+"})
				So(list("bar/+/"), ShouldResemble, []string{"baz$", "baz"})
				So(list("baz"), ShouldResemble, []string{})
			})

			Convey(`When listing "foo/+"`, func() {
				r := Request{
					Base: "foo/+",
				}

				Convey(`Can list the first 2 elements.`, func() {
					r.Limit = 2
					So(req(r), ShouldResemble, []string{"bar$", "baz$"})
				})

				Convey(`Can list 3 elements, skipping the first.`, func() {
					r.Limit = 3
					r.Skip = 1
					So(req(r), ShouldResemble, []string{"baz$", "qux$", "bar"})
				})
			})

			Convey(`Can list the hierarchy streams recursively.`, func() {
				list := func(p string) []string {
					return req(Request{Base: p, Recursive: true, StreamOnly: true})
				}

				So(list(""), ShouldResemble, []string{
					"bar/+/baz$",
					"bar/+/baz/qux$",
					"foo/+/bar$",
					"foo/+/baz$",
					"foo/+/qux$",
					"foo/+/bar/baz$",
					"foo/bar/+/baz$",
				})

				So(list("foo"), ShouldResemble, []string{
					"+/bar$",
					"+/baz$",
					"+/qux$",
					"+/bar/baz$",
					"bar/+/baz$",
				})

				So(list("foo/+/"), ShouldResemble, []string{
					"bar$",
					"baz$",
					"qux$",
					"bar/baz$",
				})
			})

			Convey(`Can list the immediate hierarchy iteratively.`, func() {
				r := Request{
					Base:  "foo/+",
					Limit: 2,
				}

				l, err := Get(di, r)
				So(err, ShouldBeNil)
				So(l.Base, ShouldEqual, "foo/+")
				So(norm(l), ShouldResemble, []string{"bar$", "baz$"})
				So(l.Next, ShouldNotEqual, "")
				So(l.Comp[0].Stream, ShouldEqual, "foo/+/bar")
				So(l.Comp[1].Stream, ShouldEqual, "foo/+/baz")

				r.Next = l.Next
				l, err = Get(di, r)
				So(err, ShouldBeNil)
				So(l.Base, ShouldEqual, "foo/+")
				So(norm(l), ShouldResemble, []string{"qux$", "bar"})
				So(l.Next, ShouldNotEqual, "")
				So(l.Comp[0].Stream, ShouldEqual, "foo/+/qux")
				So(l.Comp[1].Stream, ShouldEqual, "")

				r.Next = l.Next
				l, err = Get(di, Request{Base: "foo/+", Limit: 2, Next: l.Next})
				So(err, ShouldBeNil)
				So(l.Base, ShouldEqual, "foo/+")
				So(norm(l), ShouldResemble, []string{})
				So(l.Next, ShouldEqual, "")
			})

			Convey(`Can purge some of the streams.`, func() {

			})
		})
	})
}
