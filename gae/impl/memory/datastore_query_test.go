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

package memory

import (
	"bytes"
	"testing"

	"go.chromium.org/luci/common/data/cmpbin"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	dstore "go.chromium.org/luci/gae/service/datastore"
)

type sillyCursor string

func (s sillyCursor) String() string { return string(s) }

func curs(pairs ...any) queryCursor {
	if len(pairs)%2 != 0 {
		panic("curs() takes only even pairs")
	}
	pre := &bytes.Buffer{}
	if _, err := cmpbin.WriteUint(pre, uint64(len(pairs)/2)); err != nil {
		panic(err)
	}
	post := cmpbin.Invertible(&bytes.Buffer{})
	for i := 0; i < len(pairs); i += 2 {
		k, v := pairs[i].(string), pairs[i+1]

		col, err := dstore.ParseIndexColumn(k)
		if err != nil {
			panic(err)
		}

		post.SetInvert(col.Descending)
		if err := dstore.Serialize.IndexColumn(pre, col); err != nil {
			panic(err)
		}
		if err := dstore.Serialize.Property(post, prop(v)); err != nil {
			panic(err)
		}
	}
	return queryCursor(cmpbin.ConcatBytes(pre.Bytes(), post.Bytes()))
}

type queryTest struct {
	// name is the name of the test case
	name string

	// q is the input query
	q *dstore.Query

	// err is the error to expect after prepping the query (error, string or nil)
	err any

	// equivalentQuery is another query which ShouldResemble q. This is useful to
	// see the effects of redundancy pruning on e.g. filters.
	equivalentQuery *reducedQuery
}

var queryTests = []queryTest{
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
		nq().Gt("Bob", 10).Start(
			curs("Foo", 100, "__key__", key("something", 1)),
		),
		"start cursor is invalid", nil},

	{"bad cursors (doesn't include all orders)",
		nq().Order("Luci").Order("Charliene").Start(
			curs("Luci", 100, "__key__", key("something", 1)),
		),
		"start cursor is invalid", nil},

	{"cursor bad type",
		nq().Order("Luci").End(sillyCursor("I am a banana")),
		"bad cursor type", nil},

	{"overconstrained inequality (>= v <)",
		nq().Gte("bob", 10).Lt("bob", 10),
		dstore.ErrNullQuery, nil},

	{"overconstrained inequality (> v <)",
		nq().Gt("bob", 10).Lt("bob", 10),
		dstore.ErrNullQuery, nil},

	{"overconstrained inequality (> v <=)",
		nq().Gt("bob", 10).Lte("bob", 10),
		dstore.ErrNullQuery, nil},

	{"silly inequality (=> v <=)",
		nq().Gte("bob", 10).Lte("bob", 10),
		nil, nil},

	{"cursors get smooshed into the inquality range",
		(nq().Gt("Foo", 3).Lt("Foo", 10).
			Start(curs("Foo", 2, "__key__", key("Something", 1))).
			End(curs("Foo", 20, "__key__", key("Something", 20)))),
		nil,
		&reducedQuery{
			dstore.MkKeyContext("dev~app", "ns"),
			"Foo", map[string]stringset.Set{}, []dstore.IndexColumn{
				{Property: "Foo"},
				{Property: "__key__"},
			},
			increment(dstore.Serialize.ToBytes(dstore.MkProperty(3))),
			dstore.Serialize.ToBytes(dstore.MkProperty(10)),
			2,
		}},

	{"cursors could cause the whole query to be useless",
		(nq().Gt("Foo", 3).Lt("Foo", 10).
			Start(curs("Foo", 200, "__key__", key("Something", 1))).
			End(curs("Foo", 1, "__key__", key("Something", 20)))),
		dstore.ErrNullQuery,
		nil},
}

func TestQueries(t *testing.T) {
	t.Parallel()

	ftt.Run("queries have tons of condition checking", t, func(t *ftt.Test) {
		kc := dstore.MkKeyContext("dev~app", "ns")

		t.Run("non-ancestor queries in a transaction", func(t *ftt.Test) {
			fq, err := nq().Finalize()
			assert.Loosely(t, err, should.ErrLike(nil))
			_, err = reduce(fq, kc, true)
			assert.Loosely(t, err, should.ErrLike("must include an Ancestor filter"))
		})

		t.Run("non-ancestor queries work in firestore mode in a transaction", func(t *ftt.Test) {
			fq, err := nq().FirestoreMode(true).EventualConsistency(true).Finalize()
			assert.Loosely(t, err, should.ErrLike(nil))
			_, err = reduce(fq, kc, true)
			assert.Loosely(t, err, should.ErrLike(nil))
		})

		t.Run("absurd numbers of filters are prohibited", func(t *ftt.Test) {
			q := nq().Ancestor(key("thing", "wat"))
			for i := 0; i < 100; i++ {
				q = q.Eq("something", i)
			}
			fq, err := q.Finalize()
			assert.Loosely(t, err, should.ErrLike(nil))
			_, err = reduce(fq, kc, false)
			assert.Loosely(t, err, should.ErrLike("query is too large"))
		})

		t.Run("bulk check", func(t *ftt.Test) {
			for _, tc := range queryTests {
				t.Run(tc.name, func(t *ftt.Test) {
					rq := (*reducedQuery)(nil)
					fq, err := tc.q.Finalize()
					if err == nil {
						err = fq.Valid(kc)
						if err == nil {
							rq, err = reduce(fq, kc, false)
						}
					}
					assert.Loosely(t, err, should.ErrLike(tc.err))

					if tc.equivalentQuery != nil {
						assert.Loosely(t, rq, should.Resemble(tc.equivalentQuery))
					}
				})
			}
		})
	})
}
