// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"math"
	"testing"

	dsS "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/datastore/serialize"
	"github.com/luci/luci-go/common/cmpbin"
	. "github.com/luci/luci-go/common/testing/assertions"
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
			q := ds.NewQuery("Foo").Filter("farnsworth >", 20).KeysOnly().Limit(10).Offset(39)

			// normally you can only get cursors from inside of the memory
			// implementation, so this construction is just for testing.
			start := queryCursor(bjoin(
				mkNum(2),
				serialize.ToBytes(dsS.IndexColumn{Property: "farnsworth"}),
				serialize.ToBytes(dsS.IndexColumn{Property: "__key__"}),
				serialize.ToBytes(prop(200)),
				serialize.ToBytes(prop(ds.NewKey("Foo", "id", 0, nil)))))

			So(start.String(), ShouldEqual,
				`gYAAZzFdTeeb3d9zOxsAAF-v221Xy32_AIGHyIkAAUc32-AGabMAAA==`)

			end := queryCursor(bjoin(
				mkNum(2),
				serialize.ToBytes(dsS.IndexColumn{Property: "farnsworth"}),
				serialize.ToBytes(dsS.IndexColumn{Property: "__key__"}),
				serialize.ToBytes(prop(3000)),
				serialize.ToBytes(prop(ds.NewKey("Foo", "zeta", 0, nil)))))

			q = q.Start(start).End(end)
			So(q, ShouldNotBeNil)
			So(q.(*queryImpl).err, ShouldBeNil)
			rq, err := q.(*queryImpl).reduce("", false)
			So(rq, ShouldNotBeNil)
			So(err, ShouldBeNil)
		})

		Convey("ensures orders make sense", func() {
			q := ds.NewQuery("Cool")
			q = q.Filter("cat =", 19).Filter("bob =", 10).Order("bob").Order("bob")

			Convey("removes dups and equality orders", func() {
				q = q.Order("wat")
				qi := q.(*queryImpl)
				So(qi.err, ShouldBeNil)
				rq, err := qi.reduce("", false)
				So(err, ShouldBeNil)
				So(rq.suffixFormat, ShouldResemble, []dsS.IndexColumn{
					{Property: "wat"}, {Property: "__key__"}})
			})

			Convey("if we equality-filter on __key__, that's just silly", func() {
				q = q.Order("wat").Filter("__key__ =", ds.NewKey("Foo", "wat", 0, nil))
				_, err := q.(*queryImpl).reduce("", false)
				So(err, ShouldErrLike, "query equality filter on __key__ is silly")
			})

		})

	})
}

type queryTest struct {
	// name is the name of the test case
	name string

	// q is the input query
	q dsS.Query

	// err is the error to expect after prepping the query (error, string or nil)
	err interface{}

	// equivalentQuery is another query which ShouldResemble q. This is useful to
	// see the effects of redundancy pruning on e.g. filters.
	equivalentQuery dsS.Query
}

type sillyCursor string

func (s sillyCursor) String() string { return string(s) }

func curs(pairs ...interface{}) queryCursor {
	if len(pairs)%2 != 0 {
		panic("curs() takes only even pairs")
	}
	pre := &bytes.Buffer{}
	cmpbin.WriteUint(pre, uint64(len(pairs)/2))
	post := serialize.Invertible(&bytes.Buffer{})
	for i := 0; i < len(pairs); i += 2 {
		k, v := pairs[i].(string), pairs[i+1]

		col := dsS.IndexColumn{Property: k}

		post.SetInvert(false)
		if k[0] == '-' {
			post.SetInvert(false)
			col.Property = k[1:]
			col.Direction = dsS.DESCENDING
		}
		serialize.WriteIndexColumn(pre, col)
		serialize.WriteProperty(post, serialize.WithoutContext, prop(v))
	}
	return queryCursor(bjoin(pre.Bytes(), post.Bytes()))
}

var queryTests = []queryTest{
	{"only one inequality",
		nq().Order("bob").Order("wat").Filter("bob >", 10).Filter("wat <", 29),
		"inequality filters on multiple properties", nil},

	{"bad filter ops",
		nq().Filter("Bob !", "value"),
		"invalid operator \"!\"", nil},

	{"bad filter",
		nq().Filter("Bob", "value"),
		"invalid filter", nil},

	{"bad order",
		nq().Order("+Bob"),
		"invalid order", nil},

	{"empty order",
		nq().Order(""),
		"empty order", nil},

	{"underflow offset",
		nq().Offset(-20),
		"negative query offset", nil},

	{"bad cursors (empty)",
		nq().Start(queryCursor("")),
		"invalid cursor", nil},

	{"bad cursors (nil)",
		nq().Start(queryCursor("")),
		"invalid cursor", nil},

	{"bad cursors (no key)",
		nq().End(curs("Foo", 100)),
		"invalid cursor", nil},

	// TODO(riannucci): exclude cursors which are out-of-bounds with inequality?
	// I think right now you could have a query for > 10 with a start cursor of 1.
	{"bad cursors (doesn't include ineq)",
		nq().Filter("Bob >", 10).Start(
			curs("Foo", 100, "__key__", key("something", 1)),
		),
		"start cursor is invalid", nil},

	{"bad cursors (doesn't include all orders)",
		nq().Order("Luci").Order("Charliene").Start(
			curs("Luci", 100, "__key__", key("something", 1)),
		),
		"start cursor is invalid", nil},

	{"cursor set multiple times",
		nq().Order("Luci").End(
			curs("Luci", 100, "__key__", key("something", 1)),
		).End(
			curs("Luci", 100, "__key__", key("something", 1)),
		),
		"multiply defined", nil},

	{"cursor bad type",
		nq().Order("Luci").End(sillyCursor("I am a banana")),
		"unknown type", nil},

	{"projecting a keys-only query",
		nq().Project("hello").KeysOnly(),
		"cannot project a keysOnly query", nil},

	{"projecting a keys-only query (reverse)",
		nq().KeysOnly().Project("hello"),
		"cannot project a keysOnly query", nil},

	{"projecting an empty field",
		nq().Project("hello", ""),
		"cannot project on an empty field", nil},

	{"projecting __key__",
		nq().Project("hello", "__key__"),
		"cannot project on __key__", nil},

	{"projecting a duplicate",
		nq().Project("hello", "hello"),
		"cannot project on the same field twice", nil},

	{"projecting a duplicate (style 2)",
		nq().Project("hello").Project("hello"),
		"cannot project on the same field twice", nil},

	{"bad ancestors",
		nq().Ancestor(key("goop", nil)),
		dsS.ErrInvalidKey, nil},

	{"nil ancestors",
		nq().Ancestor(nil),
		"nil query ancestor", nil},

	{"Bad key filters",
		nq().Filter("__key__ >", key("goop", nil)),
		dsS.ErrInvalidKey, nil},

	{"filters for __key__ that aren't keys",
		nq().Filter("__key__ >", 10),
		"is not a key", nil},

	{"multiple inequalities",
		nq().Filter("bob > ", 19).Filter("charlie < ", 20),
		"inequality filters on multiple properties", nil},

	{"inequality must be first sort order",
		nq().Filter("bob > ", 19).Order("-charlie"),
		"first sort order", nil},

	{"inequality must be first sort order (reverse)",
		nq().Order("-charlie").Filter("bob > ", 19),
		"first sort order", nil},

	{"equality filter projected field",
		nq().Project("foo").Filter("foo = ", 10),
		"cannot project", nil},

	{"equality filter projected field (reverse)",
		nq().Filter("foo = ", 10).Project("foo"),
		"cannot project", nil},

	{"kindless with non-__key__ filters",
		nq("").Filter("face <", 25.3),
		"kindless queries can only filter on __key__", nil},

	{"kindless with non-__key__ orders",
		nq("").Order("face"),
		"invalid order for kindless query", nil},

	{"kindless with descending-__key__ order",
		nq("").Order("-__key__"),
		"invalid order for kindless query", nil},

	{"bad namespace",
		nq("something", "sup").Order("__key__"),
		"Namespace mismatched", nil},

	{"distinct non-projection",
		nq().Distinct().Filter("marla >", 1),
		"only makes sense on projection queries", nil},

	{"chained errors return the first",
		nq().Ancestor(nil).Filter("hello", "wurld").Order(""),
		"nil query ancestor", nil},

	{"bad ancestor namespace",
		nq("", "nerd").Ancestor(key("something", "correct")),
		"bad namespace", nil},

	{"multiple ancestors",
		nq().Ancestor(key("something", "correct")).Ancestor(key("something", "else")),
		"more than one ancestor", nil},

	{"filter with illegal type",
		nq().Filter("something =", complex(1, 2)),
		"bad type complex", nil},

	{"sort orders used for equality are ignored",
		nq().Order("a").Order("b").Order("c").Filter("b =", 2),
		nil,
		nq().Order("a").Order("c").Filter("b =", 2)},

	{"sort orders used for equality are ignored (reversed)",
		nq().Filter("b =", 2).Order("a").Order("b").Order("c"),
		nil,
		nq().Order("a").Order("c").Filter("b =", 2)},

	{"duplicate orders are ignored",
		nq().Order("a").Order("a").Order("a"),
		nil,
		nq().Order("a")},

	{"overconstrained inequality (>= v <)",
		nq().Filter("bob >=", 10).Filter("bob <", 10),
		"done", nil},

	{"overconstrained inequality (> v <)",
		nq().Filter("bob >", 10).Filter("bob <", 10),
		"done", nil},

	{"overconstrained inequality (> v <=)",
		nq().Filter("bob >", 10).Filter("bob <=", 10),
		"done", nil},

	{"silly inequality (=> v <=)",
		nq().Filter("bob >=", 10).Filter("bob <=", 10),
		nil,
		nil},

	{"Filtering on a reserved property is forbidden",
		nq().Filter("__special__ >=", 10),
		"filter on reserved property",
		nil},

	{"oob key filters with ancestor (highside)",
		nq().Ancestor(key("Hello", 10)).Filter("__key__ <", key("Hello", 9)),
		"__key__ inequality",
		nil},

	{"oob key filters with ancestor (lowside)",
		nq().Ancestor(key("Hello", 10)).Filter("__key__ >", key("Hello", 11)),
		"__key__ inequality",
		nil},

	{"in-bound key filters with ancestor OK",
		nq().Ancestor(key("Hello", 10)).Filter("__key__ <", key("Something", "hi", key("Hello", 10))),
		nil,
		nil},

	{"projection elements get filled in",
		nq().Project("Foo", "Bar").Order("-Bar"),
		nil,
		nq().Project("Foo", "Bar").Order("-Bar").Order("Foo")},

	{"cursors get smooshed into the inquality range",
		(nq().Filter("Foo >", 3).Filter("Foo <", 10).
			Start(curs("Foo", 2, "__key__", key("Something", 1))).
			End(curs("Foo", 20, "__key__", key("Something", 20)))),
		nil,
		nq().Filter("Foo >", 3).Filter("Foo <", 10)},

	{"cursors could cause the whole query to be useless",
		(nq().Filter("Foo >", 3).Filter("Foo <", 10).
			Start(curs("Foo", 200, "__key__", key("Something", 1))).
			End(curs("Foo", 1, "__key__", key("Something", 20)))),
		errQueryDone,
		nil},

	{"query without anything is fine",
		nq(),
		nil,
		nil},
}

func init() {
	// this is supremely stupid. The SDK uses 'int' which measn we have to
	// use it too, but then THEY BOUNDS CHECK IT FOR 32 BITS... *sigh*
	if !IntIs32Bits {
		queryTests = append(queryTests, []queryTest{
			{"OOB limit (32 bit)",
				nq().Limit(MaxInt),
				"query limit overflow", nil},

			{"OOB offset (32 bit)",
				nq().Offset(MaxInt),
				"query offset overflow", nil},
		}...)
	}
}

func TestQueries(t *testing.T) {
	t.Parallel()

	Convey("queries have tons of condition checking", t, func() {
		for _, tc := range queryTests {
			Convey(tc.name, func() {
				rq, err := tc.q.(*queryImpl).reduce("ns", false)
				So(err, ShouldErrLike, tc.err)

				if tc.equivalentQuery != nil {
					rq2, err := tc.equivalentQuery.(*queryImpl).reduce("ns", false)
					So(err, ShouldBeNil)
					So(rq, ShouldResemble, rq2)
				}
			})
		}

		Convey("non-ancestor queries in a transaction", func() {
			_, err := nq().(*queryImpl).reduce("ns", true)
			So(err, ShouldErrLike, "Only ancestor queries")
		})

		Convey("absurd numbers of filters are prohibited", func() {
			q := nq().Ancestor(key("thing", "wat"))
			for i := 0; i < 100; i++ {
				q = q.Filter("something =", i)
			}
			//So(q.(*queryImpl).numComponents(), ShouldEqual, 101)
			_, err := q.(*queryImpl).reduce("ns", false)
			So(err, ShouldErrLike, "query is too large")
		})
	})
}
