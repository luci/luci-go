// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"math"
	"testing"

	dsS "github.com/luci/gae/service/datastore"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

const (
	MaxUint     = ^uint(0)
	MaxInt      = int(MaxUint >> 1)
	IntIs32Bits = int64(MaxInt) < math.MaxInt64
)

func TestDatastoreQueries(t *testing.T) {
	Convey("Datastore Query suport", t, func() {
		c := Use(context.Background())
		ds := dsS.Get(c)
		So(ds, ShouldNotBeNil)

		Convey("can create good queries", func() {
			q := ds.NewQuery("Foo").KeysOnly().Limit(10).Offset(39)
			q = q.Start(queryCursor("kosmik")).End(queryCursor("krabs"))
			So(q, ShouldNotBeNil)
			So(q.(*queryImpl).err, ShouldBeNil)
			done, err := q.(*queryImpl).valid("", false)
			So(done, ShouldBeFalse)
			So(err, ShouldBeNil)
		})

		Convey("ensures orders make sense", func() {
			q := ds.NewQuery("Cool")
			q = q.Filter("cat =", 19).Filter("bob =", 10).Order("bob").Order("bob")

			Convey("removes dups and equality orders", func() {
				q = q.Order("wat")
				qi := q.(*queryImpl)
				done, err := qi.valid("", false)
				So(done, ShouldBeFalse)
				So(err, ShouldBeNil)
				So(qi.order, ShouldResemble, []dsS.IndexColumn{{Property: "wat"}})
			})

			Convey("if we equality-filter on __key__, that's just silly", func() {
				q = q.Order("wat")
				q := q.Filter("__key__ =", ds.NewKey("Foo", "wat", 0, nil))
				_, err := q.(*queryImpl).valid("", false)
				So(err.Error(), ShouldContainSubstring,
					"query equality filter on __key__ is silly")
			})

		})

		Convey("inequalities apply immediately", func() {
			// NOTE: this is (maybe?) a slight divergence from reality, but it's
			// helpful to retain sanity. It's possible that in real-appengine, many
			// inequalities count towards the MaxQueryComponents limit (100), where
			// in this system we will never have more than 2 (an upper and lower
			// bound).
		})

		Convey("can create bad queries", func() {
			q := ds.NewQuery("Foo")

			Convey("only one inequality", func() {
				q = q.Order("bob").Order("wat")
				q = q.Filter("bob >", 10).Filter("wat <", 29)
				qi := q.(*queryImpl)
				_, err := qi.valid("", false)
				So(err.Error(), ShouldContainSubstring,
					"inequality filters on multiple properties")
			})

			Convey("bad filter ops", func() {
				q := q.Filter("Bob !", "value")
				So(q.(*queryImpl).err.Error(), ShouldContainSubstring, "invalid operator \"!\"")
			})
			Convey("bad filter", func() {
				q := q.Filter("Bob", "value")
				So(q.(*queryImpl).err.Error(), ShouldContainSubstring, "invalid filter")
			})
			Convey("bad order", func() {
				q := q.Order("+Bob")
				So(q.(*queryImpl).err.Error(), ShouldContainSubstring, "invalid order")
			})
			Convey("empty", func() {
				q := q.Order("")
				So(q.(*queryImpl).err.Error(), ShouldContainSubstring, "empty order")
			})
			Convey("OOB limit", func() {
				// this is supremely stupid. The SDK uses 'int' which measn we have to
				// use it too, but then THEY BOUNDS CHECK IT FOR 32 BITS... *sigh*
				if !IntIs32Bits {
					q := q.Limit(MaxInt)
					So(q.(*queryImpl).err.Error(), ShouldContainSubstring, "query limit overflow")
				}
			})
			Convey("underflow offset", func() {
				q := q.Offset(-29)
				So(q.(*queryImpl).err.Error(), ShouldContainSubstring, "negative query offset")
			})
			Convey("OOB offset", func() {
				if !IntIs32Bits {
					q := q.Offset(MaxInt)
					So(q.(*queryImpl).err.Error(), ShouldContainSubstring, "query offset overflow")
				}
			})
			Convey("Bad cursors", func() {
				q := q.Start(queryCursor("")).End(queryCursor(""))
				So(q.(*queryImpl).err.Error(), ShouldContainSubstring, "invalid cursor")
			})
			Convey("Bad ancestors", func() {
				q := q.Ancestor(ds.NewKey("Goop", "wat", 10, nil))
				So(q, ShouldNotBeNil)
				_, err := q.(*queryImpl).valid("", false)
				So(err, ShouldEqual, dsS.ErrInvalidKey)
			})
			Convey("nil ancestors", func() {
				_, err := q.Ancestor(nil).(*queryImpl).valid("", false)
				So(err.Error(), ShouldContainSubstring, "nil query ancestor")
			})
			Convey("Bad key filters", func() {
				q := q.Filter("__key__ >", ds.NewKey("Goop", "wat", 10, nil))
				_, err := q.(*queryImpl).valid("", false)
				So(err, ShouldEqual, dsS.ErrInvalidKey)
			})
			Convey("non-ancestor queries in a transaction", func() {
				_, err := q.(*queryImpl).valid("", true)
				So(err.Error(), ShouldContainSubstring, "Only ancestor queries")
			})
			Convey("absurd numbers of filters are prohibited", func() {
				q := q.Ancestor(ds.NewKey("thing", "wat", 0, nil))
				for i := 0; i < 100; i++ {
					q = q.Filter("something =", i)
				}
				_, err := q.(*queryImpl).valid("", false)
				So(err.Error(), ShouldContainSubstring, "query is too large")
			})
			Convey("filters for __key__ that aren't keys", func() {
				q := q.Filter("__key__ > ", 10)
				_, err := q.(*queryImpl).valid("", false)
				So(err.Error(), ShouldContainSubstring, "is not a key")
			})
			Convey("multiple inequalities", func() {
				q := q.Filter("bob > ", 19).Filter("charlie < ", 20)
				_, err := q.(*queryImpl).valid("", false)
				So(err.Error(), ShouldContainSubstring,
					"inequality filters on multiple properties")
			})
			Convey("bad sort orders", func() {
				q := q.Filter("bob > ", 19).Order("-charlie")
				_, err := q.(*queryImpl).valid("", false)
				So(err.Error(), ShouldContainSubstring, "first sort order")
			})
			Convey("kindless with non-__key__ filters", func() {
				q := ds.NewQuery("").Filter("face <", 25.3)
				_, err := q.(*queryImpl).valid("", false)
				So(err.Error(), ShouldContainSubstring,
					"kindless queries can only filter on __key__")
			})
			Convey("kindless with non-__key__ orders", func() {
				q := ds.NewQuery("").Order("face")
				_, err := q.(*queryImpl).valid("", false)
				So(err.Error(), ShouldContainSubstring,
					"invalid order for kindless query")
			})
			Convey("kindless with decending-__key__ orders", func() {
				q := ds.NewQuery("").Order("-__key__")
				_, err := q.(*queryImpl).valid("", false)
				So(err.Error(), ShouldContainSubstring,
					"invalid order for kindless query")
			})
		})

	})
}
