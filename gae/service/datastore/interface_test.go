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

package datastore_test

import (
	"context"
	"iter"
	"sync"
	"sync/atomic"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

const SimpleRecordKind = "SimpleRecord"

type SimpleRecord struct {
	_kind string `gae:"$kind,SimpleRecord"`
	key   string `gae:"$id"`
	value string `gae:"value"`
}

func getAll(ctx context.Context, n int) ([]*SimpleRecord, error) {
	var out []*SimpleRecord
	query := datastore.NewQuery(SimpleRecordKind)
	var err error
	if n <= 0 {
		err = datastore.GetAll(ctx, query, &out)
	} else {
		err = datastore.GetAllWithLimit(ctx, query, &out, n)
	}
	return out, err
}

func TestGetAll_Empty(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = memory.Use(ctx)
	datastore.GetTestable(ctx).Consistent(true)

	records, err := getAll(ctx, 1)
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, len(records), should.Equal(0))
}

func TestGetAll_Singleton(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = memory.Use(ctx)
	datastore.GetTestable(ctx).Consistent(true)

	err := datastore.Put(ctx, &SimpleRecord{key: "a", value: "b"})
	assert.Loosely(t, err, should.BeNil)

	records, err := getAll(ctx, 1)
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, len(records), should.Equal(1))
}

func TestGetAll_Doubleton(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = memory.Use(ctx)
	datastore.GetTestable(ctx).Consistent(true)

	err := datastore.Put(ctx, &SimpleRecord{key: "a", value: "b"})
	assert.Loosely(t, err, should.BeNil)

	err = datastore.Put(ctx, &SimpleRecord{key: "a1", value: "b1"})
	assert.Loosely(t, err, should.BeNil)

	records, err := getAll(ctx, 1)
	assert.Loosely(t, err, should.ErrLike(datastore.ErrLimitExceeded))
	assert.Loosely(t, len(records), should.Equal(1))

	records, err = getAll(ctx, 2)
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, len(records), should.Equal(2))
}

type TestIterRecord struct {
	Kind  string `gae:"$kind,TestIterRecord"`
	ID    string `gae:"$id"`
	Value string `gae:"value"`
}

func TestRunQuery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = memory.Use(ctx)
	datastore.GetTestable(ctx).Consistent(true)

	err := datastore.Put(ctx, &TestIterRecord{ID: "a", Value: "val_a"})
	assert.Loosely(t, err, should.BeNil)
	err = datastore.Put(ctx, &TestIterRecord{ID: "b", Value: "val_b"})
	assert.Loosely(t, err, should.BeNil)

	t.Run("PointerToStruct", func(t *testing.T) {
		q := datastore.NewQuery("TestIterRecord")
		it := datastore.RunQuery[*TestIterRecord](ctx, q)
		var got []*TestIterRecord
		for r, err := range it.Results {
			assert.Loosely(t, err, should.BeNil)
			got = append(got, r)
		}
		assert.Loosely(t, len(got), should.Equal(2))
		assert.Loosely(t, got[0].Value, should.Equal("val_a"))
		assert.Loosely(t, got[1].Value, should.Equal("val_b"))
	})

	t.Run("StructValue", func(t *testing.T) {
		q := datastore.NewQuery("TestIterRecord")
		it := datastore.RunQuery[TestIterRecord](ctx, q)
		var got []TestIterRecord
		for r, err := range it.Results {
			assert.Loosely(t, err, should.BeNil)
			got = append(got, r)
		}
		assert.Loosely(t, len(got), should.Equal(2))
		assert.Loosely(t, got[0].Value, should.Equal("val_a"))
		assert.Loosely(t, got[1].Value, should.Equal("val_b"))
	})

	t.Run("KeysOnly", func(t *testing.T) {
		q := datastore.NewQuery("TestIterRecord")
		it := datastore.RunQuery[*datastore.Key](ctx, q)
		var got []*datastore.Key
		for k, err := range it.Results {
			assert.Loosely(t, err, should.BeNil)
			got = append(got, k)
		}
		assert.Loosely(t, len(got), should.Equal(2))
		assert.Loosely(t, got[0].StringID(), should.Equal("a"))
		assert.Loosely(t, got[1].StringID(), should.Equal("b"))
	})

	t.Run("Cursor", func(t *testing.T) {
		q := datastore.NewQuery("TestIterRecord").Limit(1)
		it := datastore.RunQuery[*TestIterRecord](ctx, q)
		var got []*TestIterRecord
		for r, err := range it.Results {
			assert.Loosely(t, err, should.BeNil)
			got = append(got, r)
		}
		assert.Loosely(t, len(got), should.Equal(1))
		assert.Loosely(t, got[0].Value, should.Equal("val_a"))

		cur, err := it.Cursor()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cur, should.NotBeNil)

		q2 := datastore.NewQuery("TestIterRecord").Start(cur)
		it2 := datastore.RunQuery[*TestIterRecord](ctx, q2)
		var got2 []*TestIterRecord
		for r, err := range it2.Results {
			assert.Loosely(t, err, should.BeNil)
			got2 = append(got2, r)
		}
		assert.Loosely(t, len(got2), should.Equal(1))
		assert.Loosely(t, got2[0].Value, should.Equal("val_b"))
	})

	t.Run("PropertyMap", func(t *testing.T) {
		q := datastore.NewQuery("TestIterRecord")
		it := datastore.RunQuery[datastore.PropertyMap](ctx, q)
		var got []datastore.PropertyMap
		for pm, err := range it.Results {
			assert.Loosely(t, err, should.BeNil)
			got = append(got, pm)
		}
		assert.Loosely(t, len(got), should.Equal(2))
		assert.Loosely(t, got[0]["value"].Slice()[0].Value(), should.Equal("val_a"))
		assert.Loosely(t, got[1]["value"].Slice()[0].Value(), should.Equal("val_b"))
	})

	t.Run("InvalidQuery", func(t *testing.T) {
		q := datastore.NewQuery("").Lt("invalid", nil)
		it := datastore.RunQuery[*TestIterRecord](ctx, q)
		for _, err := range it.Results {
			assert.Loosely(t, err, should.NotBeNil)
		}
	})
}

func TestRunMultiQuery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = memory.Use(ctx)
	datastore.GetTestable(ctx).Consistent(true)

	err := datastore.Put(ctx, &TestIterRecord{ID: "a", Value: "val_a"})
	assert.Loosely(t, err, should.BeNil)
	err = datastore.Put(ctx, &TestIterRecord{ID: "b", Value: "val_b"})
	assert.Loosely(t, err, should.BeNil)

	t.Run("PointerToStruct", func(t *testing.T) {
		queries := []*datastore.Query{
			datastore.NewQuery("TestIterRecord").Eq("value", "val_a"),
			datastore.NewQuery("TestIterRecord").Eq("value", "val_b"),
		}
		it := datastore.RunMultiQuery[*TestIterRecord](ctx, queries)
		var got []*TestIterRecord
		for r, err := range it.Results {
			assert.Loosely(t, err, should.BeNil)
			got = append(got, r)
		}
		assert.Loosely(t, len(got), should.Equal(2))
		assert.Loosely(t, got[0].Value, should.Equal("val_a"))
		assert.Loosely(t, got[1].Value, should.Equal("val_b"))
	})

	t.Run("StructValue", func(t *testing.T) {
		queries := []*datastore.Query{
			datastore.NewQuery("TestIterRecord").Eq("value", "val_a"),
			datastore.NewQuery("TestIterRecord").Eq("value", "val_b"),
		}
		it := datastore.RunMultiQuery[TestIterRecord](ctx, queries)
		var got []TestIterRecord
		for r, err := range it.Results {
			assert.Loosely(t, err, should.BeNil)
			got = append(got, r)
		}
		assert.Loosely(t, len(got), should.Equal(2))
		assert.Loosely(t, got[0].Value, should.Equal("val_a"))
		assert.Loosely(t, got[1].Value, should.Equal("val_b"))
	})

	t.Run("KeysOnly", func(t *testing.T) {
		queries := []*datastore.Query{
			datastore.NewQuery("TestIterRecord").Eq("value", "val_a"),
			datastore.NewQuery("TestIterRecord").Eq("value", "val_b"),
		}
		it := datastore.RunMultiQuery[*datastore.Key](ctx, queries)
		var got []*datastore.Key
		for k, err := range it.Results {
			assert.Loosely(t, err, should.BeNil)
			got = append(got, k)
		}
		assert.Loosely(t, len(got), should.Equal(2))
		assert.Loosely(t, got[0].StringID(), should.Equal("a"))
		assert.Loosely(t, got[1].StringID(), should.Equal("b"))
	})

	t.Run("PropertyMap", func(t *testing.T) {
		queries := []*datastore.Query{
			datastore.NewQuery("TestIterRecord").Eq("value", "val_a"),
			datastore.NewQuery("TestIterRecord").Eq("value", "val_b"),
		}
		it := datastore.RunMultiQuery[datastore.PropertyMap](ctx, queries)
		var got []datastore.PropertyMap
		for pm, err := range it.Results {
			assert.Loosely(t, err, should.BeNil)
			got = append(got, pm)
		}
		assert.Loosely(t, len(got), should.Equal(2))
		assert.Loosely(t, got[0]["value"].Slice()[0].Value(), should.Equal("val_a"))
		assert.Loosely(t, got[1]["value"].Slice()[0].Value(), should.Equal("val_b"))
	})
}

func TestRunQuery_MultipleUses(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = memory.Use(ctx)
	datastore.GetTestable(ctx).Consistent(true)

	err := datastore.Put(ctx, &TestIterRecord{ID: "a", Value: "val_a"})
	assert.Loosely(t, err, should.BeNil)
	err = datastore.Put(ctx, &TestIterRecord{ID: "b", Value: "val_b"})
	assert.Loosely(t, err, should.BeNil)

	t.Run("RunQuery Full Consumption", func(t *testing.T) {
		q := datastore.NewQuery("TestIterRecord")
		it := datastore.RunQuery[*TestIterRecord](ctx, q)
		count := 0
		for _, err := range it.Results {
			assert.Loosely(t, err, should.BeNil)
			count++
		}
		assert.Loosely(t, count, should.Equal(2))

		assert.Loosely(t, func() {
			for range it.Results {
			}
		}, should.PanicLike("cannot use QueryIter more than once."))
	})

	t.Run("RunQuery Early Break", func(t *testing.T) {
		q := datastore.NewQuery("TestIterRecord")
		it := datastore.RunQuery[*TestIterRecord](ctx, q)
		for range it.Results {
			break
		}

		assert.Loosely(t, func() {
			for range it.Results {
			}
		}, should.PanicLike("cannot use QueryIter more than once."))
	})

	t.Run("RunQuery Pull Iterator", func(t *testing.T) {
		q := datastore.NewQuery("TestIterRecord")
		it := datastore.RunQuery[*TestIterRecord](ctx, q)
		next, stop := iter.Pull2(it.Results)
		defer stop()
		_, _, ok := next()
		assert.Loosely(t, ok, should.BeTrue)

		next2, stop2 := iter.Pull2(it.Results)
		defer stop2()
		assert.Loosely(t, func() {
			next2()
		}, should.PanicLike("cannot use QueryIter more than once."))
	})

	t.Run("RunQuery Stub On Error", func(t *testing.T) {
		q := datastore.NewQuery("").Lt("invalid", nil)
		it := datastore.RunQuery[*TestIterRecord](ctx, q)
		for _, err := range it.Results {
			assert.Loosely(t, err, should.NotBeNil)
		}

		assert.Loosely(t, func() {
			for range it.Results {
			}
		}, should.PanicLike("cannot use QueryIter more than once."))
	})

	t.Run("RunMultiQuery Single Query", func(t *testing.T) {
		queries := []*datastore.Query{
			datastore.NewQuery("TestIterRecord").Eq("value", "val_a"),
		}
		it := datastore.RunMultiQuery[*TestIterRecord](ctx, queries)
		count := 0
		for _, err := range it.Results {
			assert.Loosely(t, err, should.BeNil)
			count++
		}
		assert.Loosely(t, count, should.Equal(1))

		assert.Loosely(t, func() {
			for range it.Results {
			}
		}, should.PanicLike("cannot use QueryIter more than once."))
	})

	t.Run("RunMultiQuery Multiple Queries", func(t *testing.T) {
		queries := []*datastore.Query{
			datastore.NewQuery("TestIterRecord").Eq("value", "val_a"),
			datastore.NewQuery("TestIterRecord").Eq("value", "val_b"),
		}
		it := datastore.RunMultiQuery[*TestIterRecord](ctx, queries)
		count := 0
		for _, err := range it.Results {
			assert.Loosely(t, err, should.BeNil)
			count++
		}
		assert.Loosely(t, count, should.Equal(2))

		assert.Loosely(t, func() {
			for range it.Results {
			}
		}, should.PanicLike("cannot use QueryIter more than once."))
	})

	t.Run("RunMultiQuery Multiple Queries Early Break", func(t *testing.T) {
		queries := []*datastore.Query{
			datastore.NewQuery("TestIterRecord").Eq("value", "val_a"),
			datastore.NewQuery("TestIterRecord").Eq("value", "val_b"),
		}
		it := datastore.RunMultiQuery[*TestIterRecord](ctx, queries)
		for range it.Results {
			break
		}

		assert.Loosely(t, func() {
			for range it.Results {
			}
		}, should.PanicLike("cannot use QueryIter more than once."))
	})

	t.Run("RunMultiQuery Empty Queries", func(t *testing.T) {
		it := datastore.RunMultiQuery[*TestIterRecord](ctx, nil)
		for range it.Results {
		}

		assert.Loosely(t, func() {
			for range it.Results {
			}
		}, should.PanicLike("cannot use QueryIter more than once."))
	})

	t.Run("RunMultiQuery Invalid Queries", func(t *testing.T) {
		queries := []*datastore.Query{
			datastore.NewQuery("TestIterRecord").Eq("value", "val_a"),
			datastore.NewQuery("OtherKind").Eq("value", "val_b"),
		}
		it := datastore.RunMultiQuery[*TestIterRecord](ctx, queries)
		for _, err := range it.Results {
			assert.Loosely(t, err, should.NotBeNil)
		}

		assert.Loosely(t, func() {
			for range it.Results {
			}
		}, should.PanicLike("cannot use QueryIter more than once."))
	})

	t.Run("QueryIterFromRaw", func(t *testing.T) {
		raw := datastore.RawQueryIterStub(nil)
		it := datastore.QueryIterFromRaw[*TestIterRecord](raw)
		for range it.Results {
		}

		assert.Loosely(t, func() {
			for range it.Results {
			}
		}, should.PanicLike("cannot use QueryIter more than once."))
	})

	t.Run("RunQuery Empty Result", func(t *testing.T) {
		q := datastore.NewQuery("NonExistentKind")
		it := datastore.RunQuery[*TestIterRecord](ctx, q)
		count := 0
		for _, err := range it.Results {
			assert.Loosely(t, err, should.BeNil)
			count++
		}
		assert.Loosely(t, count, should.Equal(0))

		assert.Loosely(t, func() {
			for range it.Results {
			}
		}, should.PanicLike("cannot use QueryIter more than once."))
	})

	t.Run("RunMultiQuery Single Query Early Break", func(t *testing.T) {
		queries := []*datastore.Query{
			datastore.NewQuery("TestIterRecord").Eq("value", "val_a"),
		}
		it := datastore.RunMultiQuery[*TestIterRecord](ctx, queries)
		for range it.Results {
			break
		}

		assert.Loosely(t, func() {
			for range it.Results {
			}
		}, should.PanicLike("cannot use QueryIter more than once."))
	})

	t.Run("RunMultiQuery Single Query Pull Iterator", func(t *testing.T) {
		queries := []*datastore.Query{
			datastore.NewQuery("TestIterRecord").Eq("value", "val_a"),
		}
		it := datastore.RunMultiQuery[*TestIterRecord](ctx, queries)
		next, stop := iter.Pull2(it.Results)
		defer stop()
		_, _, ok := next()
		assert.Loosely(t, ok, should.BeTrue)

		next2, stop2 := iter.Pull2(it.Results)
		defer stop2()
		assert.Loosely(t, func() {
			next2()
		}, should.PanicLike("cannot use QueryIter more than once."))
	})

	t.Run("RunMultiQuery Multiple Queries Pull Iterator", func(t *testing.T) {
		queries := []*datastore.Query{
			datastore.NewQuery("TestIterRecord").Eq("value", "val_a"),
			datastore.NewQuery("TestIterRecord").Eq("value", "val_b"),
		}
		it := datastore.RunMultiQuery[*TestIterRecord](ctx, queries)
		next, stop := iter.Pull2(it.Results)
		defer stop()
		_, _, ok := next()
		assert.Loosely(t, ok, should.BeTrue)

		next2, stop2 := iter.Pull2(it.Results)
		defer stop2()
		assert.Loosely(t, func() {
			next2()
		}, should.PanicLike("cannot use QueryIter more than once."))
	})

	t.Run("Concurrent Usage", func(t *testing.T) {
		q := datastore.NewQuery("TestIterRecord")
		it := datastore.RunQuery[*TestIterRecord](ctx, q)
		var wg sync.WaitGroup
		var panicCount atomic.Int32
		for range 5 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						if r == "cannot use QueryIter more than once." {
							panicCount.Add(1)
						}
					}
				}()
				for range it.Results {
				}
			}()
		}
		wg.Wait()
		assert.Loosely(t, panicCount.Load(), should.Equal(4))
	})
}

func TestQueryIter_InvalidTypes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = memory.Use(ctx)

	t.Run("Non-PLS primitive", func(t *testing.T) {
		assert.Loosely(t, func() {
			datastore.QueryIterFromRaw[int](datastore.RawQueryIterStub(nil))
		}, should.PanicLike("is not a PLS or pointer-to-struct"))

		assert.Loosely(t, func() {
			datastore.RunQuery[int](ctx, datastore.NewQuery("TestIterRecord"))
		}, should.PanicLike("is not a PLS or pointer-to-struct"))

		assert.Loosely(t, func() {
			datastore.RunMultiQuery[int](ctx, []*datastore.Query{datastore.NewQuery("TestIterRecord")})
		}, should.PanicLike("is not a PLS or pointer-to-struct"))
	})

	type InvalidInterface interface {
		SomeMethod()
	}

	t.Run("Interface type", func(t *testing.T) {
		assert.Loosely(t, func() {
			datastore.QueryIterFromRaw[InvalidInterface](datastore.RawQueryIterStub(nil))
		}, should.PanicLike("is not a concrete type"))

		assert.Loosely(t, func() {
			datastore.RunQuery[InvalidInterface](ctx, datastore.NewQuery("TestIterRecord"))
		}, should.PanicLike("is not a concrete type"))

		assert.Loosely(t, func() {
			datastore.RunMultiQuery[InvalidInterface](ctx, []*datastore.Query{datastore.NewQuery("TestIterRecord")})
		}, should.PanicLike("is not a concrete type"))
	})
}
