// Copyright 2015 The LUCI Authors.
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

package datastore

import (
	"math"
	"testing"

	"go.chromium.org/luci/common/sync/parallel"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

const (
	MaxUint     = ^uint(0)
	MaxInt      = int(MaxUint >> 1)
	IntIs32Bits = int64(MaxInt) < math.MaxInt64
)

func TestDatastoreQueries(t *testing.T) {
	Convey("Datastore Query suport", t, func() {
		Convey("can create good queries", func() {
			q := NewQuery("Foo").Gt("farnsworth", 20).KeysOnly(true).Limit(10).Offset(39)

			start := fakeCursor(1337)

			end := fakeCursor(24601)

			q = q.Start(start).End(end)
			So(q, ShouldNotBeNil)
			fq, err := q.Finalize()
			So(fq, ShouldNotBeNil)
			So(err, ShouldBeNil)
		})

		Convey("ensures orders make sense", func() {
			q := NewQuery("Cool")
			q = q.Eq("cat", 19).Eq("bob", 10).Order("bob", "bob")

			Convey("removes dups and equality orders", func() {
				q = q.Order("wat")
				fq, err := q.Finalize()
				So(err, ShouldBeNil)
				So(fq.Orders(), ShouldResemble, []IndexColumn{
					{Property: "wat"}, {Property: "__key__"}})
			})
		})

	})
}

type queryTest struct {
	// name is the name of the test case
	name string

	// q is the input query
	q *Query

	// gql is the expected generated GQL.
	gql string

	// assertion is the error to expect after prepping the query, or nil if the
	// error should be nil.
	assertion func(err error)

	// equivalentQuery is another query which ShouldResemble q. This is useful to
	// see the effects of redundancy pruning on e.g. filters.
	equivalentQuery *Query
}

type sillyCursor string

func (s sillyCursor) String() string { return string(s) }

func nq(kinds ...string) *Query {
	kind := "Foo"
	if len(kinds) > 0 {
		kind = kinds[0]
	}
	return NewQuery(kind)
}

func mkKey(elems ...interface{}) *Key {
	return MkKeyContext("s~aid", "ns").MakeKey(elems...)
}

func errString(v string) func(error) {
	return func(err error) {
		So(err, ShouldErrLike, v)
	}
}

func shouldBeErrInvalidKey(err error) {
	So(IsErrInvalidKey(err), ShouldBeTrue)
}

var queryTests = []queryTest{
	{"only one inequality",
		nq().Order("bob", "wat").Gt("bob", 10).Lt("wat", 29),
		"",
		errString("inequality filters on multiple properties"),
		nil},

	{"bad order",
		nq().Order("+Bob"),
		"",
		errString("invalid order"), nil},

	{"empty order",
		nq().Order(""),
		"",
		errString("empty order"), nil},

	{"negative offset disables Offset",
		nq().Offset(100).Offset(-20),
		"SELECT * FROM `Foo` ORDER BY `__key__`",
		nil, nq()},

	{"projecting a keys-only query",
		nq().Project("hello").KeysOnly(true),
		"",
		errString("cannot project a keysOnly query"), nil},

	{"projecting a keys-only query (reverse)",
		nq().KeysOnly(true).Project("hello"),
		"",
		errString("cannot project a keysOnly query"), nil},

	{"projecting an empty field",
		nq().Project("hello", ""),
		"",
		errString("cannot filter/project on: \"\""), nil},

	{"projecting __key__",
		nq().Project("hello", "__key__"),
		"",
		errString("cannot project on \"__key__\""), nil},

	{"getting all the keys",
		nq("").KeysOnly(true),
		"SELECT __key__ ORDER BY `__key__`",
		nil, nil},

	{"projecting a duplicate",
		nq().Project("hello", "hello"),
		"SELECT `hello` FROM `Foo` ORDER BY `hello`, `__key__`",
		nil, nq().Project("hello")},

	{"projecting a duplicate (style 2)",
		nq().Project("hello").Project("hello"),
		"SELECT `hello` FROM `Foo` ORDER BY `hello`, `__key__`",
		nil, nq().Project("hello")},

	{"project distinct",
		nq().Project("hello").Distinct(true),
		"SELECT DISTINCT `hello` FROM `Foo` ORDER BY `hello`, `__key__`",
		nil, nil},

	{"projecting in an inequality query",
		nq().Gte("foo", 10).Project("bar").Distinct(true),
		"SELECT DISTINCT `bar`, `foo` FROM `Foo` WHERE `foo` >= 10 ORDER BY `foo`, `bar`, `__key__`",
		nil, nil},

	{"bad ancestors",
		nq().Ancestor(mkKey("goop", 0)),
		"",
		shouldBeErrInvalidKey, nil},

	{"nil ancestors",
		nq().Ancestor(nil),
		"SELECT * FROM `Foo` ORDER BY `__key__`",
		nil, nq()},

	{"Bad key filters",
		nq().Gt("__key__", mkKey("goop", 0)),
		"",
		shouldBeErrInvalidKey, nil},

	{"filters for __key__ that aren't keys",
		nq().Gt("__key__", 10),
		"",
		errString("filters on \"__key__\" must have type *Key"), nil},

	{"multiple inequalities",
		nq().Gt("bob", 19).Lt("charlie", 20),
		"",
		errString("inequality filters on multiple properties"), nil},

	{"inequality must be first sort order",
		nq().Gt("bob", 19).Order("-charlie"),
		"",
		errString("first sort order"), nil},

	{"inequality must be first sort order (reverse)",
		nq().Order("-charlie").Gt("bob", 19),
		"",
		errString("first sort order"), nil},

	{"equality filter projected field",
		nq().Project("foo").Eq("foo", 10),
		"",
		errString("cannot project"), nil},

	{"equality filter projected field (reverse)",
		nq().Eq("foo", 10).Project("foo"),
		"",
		errString("cannot project"), nil},

	{"kindless with non-__key__ filters",
		nq("").Lt("face", 25.3),
		"",
		errString("kindless queries can only filter on __key__"), nil},

	{"kindless with non-__key__ orders",
		nq("").Order("face"),
		"",
		errString("invalid order for kindless query"), nil},

	{"kindless with descending-__key__ order",
		nq("").Order("-__key__"),
		"",
		errString("invalid order for kindless query"), nil},

	{"kindless with equality filters",
		nq("").Eq("hello", 1),
		"",
		errString("may not have any equality"), nil},

	{"kindless with ancestor filter",
		nq("").Ancestor(mkKey("Parent", 1)),
		"SELECT * WHERE __key__ HAS ANCESTOR KEY(DATASET(\"s~aid\"), NAMESPACE(\"ns\"), \"Parent\", 1) ORDER BY `__key__`",
		nil, nil},

	{"kindless with ancestor filter and __key__ ineq",
		nq("").Ancestor(mkKey("Parent", 1)).Lt("__key__", mkKey("Parent", 1, "Sub", "hat")),
		"SELECT * WHERE `__key__` < KEY(DATASET(\"s~aid\"), NAMESPACE(\"ns\"), \"Parent\", 1, \"Sub\", \"hat\") AND __key__ HAS ANCESTOR KEY(DATASET(\"s~aid\"), NAMESPACE(\"ns\"), \"Parent\", 1) ORDER BY `__key__`",
		nil, nil},

	{"distinct non-projection",
		nq().Distinct(true).Gt("marla", 1),
		"SELECT * FROM `Foo` WHERE `marla` > 1 ORDER BY `marla`, `__key__`",
		nil, nq().Gt("marla", 1)},

	{"chained errors return the first",
		nq().Eq("__reserved__", 100).Eq("hello", "wurld").Order(""),
		"",
		errString("__reserved__"), nil},

	{"multiple ancestors",
		nq().Ancestor(mkKey("something", "correct")).Ancestor(mkKey("something", "else")),
		("SELECT * FROM `Foo` " +
			"WHERE __key__ HAS ANCESTOR KEY(DATASET(\"s~aid\"), NAMESPACE(\"ns\"), \"something\", \"else\") " +
			"ORDER BY `__key__`"),
		nil, nq().Ancestor(mkKey("something", "else"))},

	{"filter with illegal type",
		nq().Eq("something", complex(1, 2)),
		"",
		errString("bad type complex"), nil},

	{"sort orders used for equality are ignored",
		nq().Order("a", "b", "c").Eq("b", 2, 2),
		"SELECT * FROM `Foo` WHERE `b` = 2 ORDER BY `a`, `c`, `__key__`",
		nil, nq().Order("a", "c").Eq("b", 2)},

	{"sort orders used for equality are ignored (reversed)",
		nq().Eq("b", 2).Order("a", "b", "c"),
		"SELECT * FROM `Foo` WHERE `b` = 2 ORDER BY `a`, `c`, `__key__`",
		nil,
		nq().Order("a", "c").Eq("b", 2)},

	{"duplicate equality filters are ignored",
		nq().Eq("b", 10, -1, 2, 2, 7, 1, 2, 10, -1, 7, 1, 2),
		"SELECT * FROM `Foo` WHERE `b` = -1 AND `b` = 1 AND `b` = 2 AND `b` = 7 AND `b` = 10 ORDER BY `__key__`",
		nil,
		nq().Eq("b", -1, 1, 2, 7, 10)},

	{"duplicate orders are ignored",
		nq().Order("a").Order("a").Order("a"),
		"SELECT * FROM `Foo` ORDER BY `a`, `__key__`",
		nil,
		nq().Order("a")},

	{"Filtering on a reserved property is forbidden",
		nq().Gte("__special__", 10),
		"",
		errString("cannot filter/project on reserved property: \"__special__\""),
		nil},

	{"in-bound key filters with ancestor OK",
		nq().Ancestor(mkKey("Hello", 10)).Lte("__key__", mkKey("Hello", 10, "Something", "hi")),
		("SELECT * FROM `Foo` " +
			"WHERE `__key__` <= KEY(DATASET(\"s~aid\"), NAMESPACE(\"ns\"), \"Hello\", 10, \"Something\", \"hi\") AND " +
			"__key__ HAS ANCESTOR KEY(DATASET(\"s~aid\"), NAMESPACE(\"ns\"), \"Hello\", 10) " +
			"ORDER BY `__key__`"),
		nil,
		nil},

	{"projection elements get filled in",
		nq().Project("Foo", "Bar").Order("-Bar"),
		"SELECT `Bar`, `Foo` FROM `Foo` ORDER BY `Bar` DESC, `Foo`, `__key__`",
		nil, nq().Project("Foo", "Bar").Order("-Bar").Order("Foo")},

	{"query without anything is fine",
		nq(),
		"SELECT * FROM `Foo` ORDER BY `__key__`",
		nil,
		nil},

	{"ineq on __key__ with ancestor must be an ancestor of __ancestor__!",
		nq().Ancestor(mkKey("Hello", 10)).Lt("__key__", mkKey("Hello", 8)),
		"",
		errString("inequality filters on __key__ must be descendants of the __ancestor__"),
		nil},

	{"ineq on __key__ with ancestor must be an ancestor of __ancestor__! (2)",
		nq().Ancestor(mkKey("Hello", 10)).Gt("__key__", mkKey("Hello", 8)),
		"",
		errString("inequality filters on __key__ must be descendants of the __ancestor__"),
		nil},

	{"can build an empty query",
		nq().Lt("hello", 10).Gt("hello", 50),
		"",
		func(err error) { So(err, ShouldEqual, ErrNullQuery) },
		nil},
}

func TestQueries(t *testing.T) {
	t.Parallel()

	Convey("queries have tons of condition checking", t, func() {
		for _, tc := range queryTests {
			Convey(tc.name, func() {
				fq, err := tc.q.Finalize()
				if err == nil {
					err = fq.Valid(MkKeyContext("s~aid", "ns"))
				}
				if tc.assertion != nil {
					tc.assertion(err)
				} else {
					So(err, ShouldBeNil)
				}

				if tc.gql != "" {
					So(fq.GQL(), ShouldEqual, tc.gql)
				}

				if tc.equivalentQuery != nil {
					fq2, err := tc.equivalentQuery.Finalize()
					So(err, ShouldBeNil)

					fq.original = nil
					fq2.original = nil
					So(fq, ShouldResemble, fq2)
				}
			})
		}
	})
}

func TestQueryConcurrencySafety(t *testing.T) {
	t.Parallel()

	Convey("query and derivative query finalization is goroutine-safe", t, func() {
		const rounds = 10

		q := NewQuery("Foo")

		err := parallel.FanOutIn(func(outerC chan<- func() error) {

			for i := 0; i < rounds; i++ {
				outerQ := q.Gt("Field", i)

				// Finalize the original query.
				outerC <- func() error {
					_, err := q.Finalize()
					return err
				}

				// Finalize the derivative query a lot.
				outerC <- func() error {
					return parallel.FanOutIn(func(innerC chan<- func() error) {
						for i := 0; i < rounds; i++ {
							innerC <- func() error {
								_, err := outerQ.Finalize()
								return err
							}
						}
					})
				}
			}
		})
		So(err, ShouldBeNil)
	})
}
