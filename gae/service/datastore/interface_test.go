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
	Value string `gae:"value"`
}

func TestAsSlice_Empty(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = memory.Use(ctx)
	datastore.GetTestable(ctx).Consistent(true)

	q := datastore.NewQuery(SimpleRecordKind).Limit(1)
	records, err := datastore.RunQuery[*SimpleRecord](ctx, q).AsSlice()
	assert.NoErr(t, err)
	assert.Loosely(t, len(records), should.Equal(0))
}

func TestAsSlice_Singleton(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = memory.Use(ctx)
	datastore.GetTestable(ctx).Consistent(true)

	err := datastore.Put(ctx, &SimpleRecord{key: "a", Value: "b"})
	assert.NoErr(t, err)

	q := datastore.NewQuery(SimpleRecordKind)
	it := datastore.RunQuery[*SimpleRecord](ctx, q)
	it.SetSizeLimit(40)
	records, err := it.AsSlice()
	assert.NoErr(t, err)
	assert.Loosely(t, len(records), should.Equal(1))
}

func TestAsSlice_Doubleton(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = memory.Use(ctx)
	datastore.GetTestable(ctx).Consistent(true)

	err := datastore.Put(ctx, &SimpleRecord{key: "a", Value: "b"})
	assert.NoErr(t, err)

	err = datastore.Put(ctx, &SimpleRecord{key: "a1", Value: "b1"})
	assert.NoErr(t, err)

	q := datastore.NewQuery(SimpleRecordKind)
	it := datastore.RunQuery[*SimpleRecord](ctx, q)
	it.SetSizeLimit(40)
	records, err := it.AsSlice()
	assert.Loosely(t, err, should.ErrLike(datastore.ErrLimitExceeded))
	assert.Loosely(t, len(records), should.Equal(1))

	it = datastore.RunQuery[*SimpleRecord](ctx, q)
	it.SetSizeLimit(80)
	records, err = it.AsSlice()
	assert.NoErr(t, err)
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
	assert.NoErr(t, err)
	err = datastore.Put(ctx, &TestIterRecord{ID: "b", Value: "val_b"})
	assert.NoErr(t, err)

	t.Run("PointerToStruct", func(t *testing.T) {
		q := datastore.NewQuery("TestIterRecord")
		it := datastore.RunQuery[*TestIterRecord](ctx, q)
		var got []*TestIterRecord
		for r, err := range it.Results {
			assert.NoErr(t, err)
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
			assert.NoErr(t, err)
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
			assert.NoErr(t, err)
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
			assert.NoErr(t, err)
			got = append(got, r)
		}
		assert.Loosely(t, len(got), should.Equal(1))
		assert.Loosely(t, got[0].Value, should.Equal("val_a"))

		cur, err := it.Cursor()
		assert.NoErr(t, err)
		assert.Loosely(t, cur, should.NotBeNil)

		q2 := datastore.NewQuery("TestIterRecord").Start(cur)
		it2 := datastore.RunQuery[*TestIterRecord](ctx, q2)
		var got2 []*TestIterRecord
		for r, err := range it2.Results {
			assert.NoErr(t, err)
			got2 = append(got2, r)
		}
		assert.Loosely(t, len(got2), should.Equal(1))
		assert.Loosely(t, got2[0].Value, should.Equal("val_b"))
	})

	t.Run("CurrentCursor", func(t *testing.T) {
		t.Run("ErrNoCurrentCursor before iteration", func(t *testing.T) {
			q := datastore.NewQuery("TestIterRecord")
			it := datastore.RunQuery[*TestIterRecord](ctx, q)
			_, err := it.CurrentCursor()
			assert.Loosely(t, err, should.Equal(datastore.ErrNoCurrentCursor))
		})

		t.Run("ErrNoCurrentCursor on empty query", func(t *testing.T) {
			q := datastore.NewQuery("NonExistentKind")
			it := datastore.RunQuery[*TestIterRecord](ctx, q)
			for _, err := range it.Results {
				assert.NoErr(t, err)
			}
			_, err := it.CurrentCursor()
			assert.Loosely(t, err, should.Equal(datastore.ErrNoCurrentCursor))
		})

		t.Run("Step by step cursor vs current cursor", func(t *testing.T) {
			q := datastore.NewQuery("TestIterRecord")
			it := datastore.RunQuery[*TestIterRecord](ctx, q)
			count := 0
			for r, err := range it.Results {
				assert.NoErr(t, err)
				count++
				curCur, err := it.CurrentCursor()
				assert.NoErr(t, err)
				assert.Loosely(t, curCur, should.NotBeNil)

				nextCur, err := it.Cursor()
				assert.NoErr(t, err)
				assert.Loosely(t, nextCur, should.NotBeNil)

				// Resume from CurrentCursor: starts from current item (r.Value).
				qFromCurrent := datastore.NewQuery("TestIterRecord").Start(curCur)
				resCurrent, err := datastore.RunQuery[*TestIterRecord](ctx, qFromCurrent).AsSlice()
				assert.NoErr(t, err)
				assert.Loosely(t, resCurrent[0].Value, should.Equal(r.Value))

				// Resume from Cursor: starts from next item (skips r.Value).
				qFromNext := datastore.NewQuery("TestIterRecord").Start(nextCur)
				resNext, err := datastore.RunQuery[*TestIterRecord](ctx, qFromNext).AsSlice()
				assert.NoErr(t, err)
				assert.Loosely(t, len(resNext), should.Equal(2-count))
			}
			assert.Loosely(t, count, should.Equal(2))

			// After iteration finishes, CurrentCursor still points to the last item ("val_b").
			lastCur, err := it.CurrentCursor()
			assert.NoErr(t, err)
			resLast, err := datastore.RunQuery[*TestIterRecord](ctx, datastore.NewQuery("TestIterRecord").Start(lastCur)).AsSlice()
			assert.NoErr(t, err)
			assert.Loosely(t, len(resLast), should.Equal(1))
			assert.Loosely(t, resLast[0].Value, should.Equal("val_b"))
		})
	})

	t.Run("RawQueryIterStub CurrentCursor", func(t *testing.T) {
		stubNil := datastore.RawQueryIterStub(nil)
		_, err := stubNil.CurrentCursor()
		assert.Loosely(t, err, should.Equal(datastore.ErrNoCurrentCursor))

		stubErr := datastore.RawQueryIterStub(datastore.ErrLimitExceeded)
		_, err = stubErr.CurrentCursor()
		assert.Loosely(t, err, should.Equal(datastore.ErrLimitExceeded))
	})

	t.Run("PropertyMap", func(t *testing.T) {
		q := datastore.NewQuery("TestIterRecord")
		it := datastore.RunQuery[datastore.PropertyMap](ctx, q)
		var got []datastore.PropertyMap
		for pm, err := range it.Results {
			assert.NoErr(t, err)
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

	t.Run("SetSizeLimit", func(t *testing.T) {
		q := datastore.NewQuery("TestIterRecord")

		t.Run("Exceeded", func(t *testing.T) {
			it := datastore.RunQuery[*TestIterRecord](ctx, q)
			it.SetSizeLimit(1)
			var got []*TestIterRecord
			var iterErr error
			for r, err := range it.Results {
				if err != nil {
					iterErr = err
					break
				}
				got = append(got, r)
			}
			assert.Loosely(t, iterErr, should.Equal(datastore.ErrLimitExceeded))
			assert.Loosely(t, got, should.BeNil)
		})

		t.Run("Disabled", func(t *testing.T) {
			it := datastore.RunQuery[*TestIterRecord](ctx, q)
			it.SetSizeLimit(-1)
			var got []*TestIterRecord
			for r, err := range it.Results {
				assert.NoErr(t, err)
				got = append(got, r)
			}
			assert.Loosely(t, len(got), should.Equal(2))
		})

		t.Run("Sufficient", func(t *testing.T) {
			it := datastore.RunQuery[*TestIterRecord](ctx, q)
			it.SetSizeLimit(1024 * 1024)
			var got []*TestIterRecord
			for r, err := range it.Results {
				assert.NoErr(t, err)
				got = append(got, r)
			}
			assert.Loosely(t, len(got), should.Equal(2))
		})
	})

	t.Run("AsSlice", func(t *testing.T) {
		t.Run("PointerToStruct", func(t *testing.T) {
			q := datastore.NewQuery("TestIterRecord")
			got, err := datastore.RunQuery[*TestIterRecord](ctx, q).AsSlice()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(got), should.Equal(2))
			assert.Loosely(t, got[0].Value, should.Equal("val_a"))
			assert.Loosely(t, got[1].Value, should.Equal("val_b"))
		})

		t.Run("StructValue", func(t *testing.T) {
			q := datastore.NewQuery("TestIterRecord")
			got, err := datastore.RunQuery[TestIterRecord](ctx, q).AsSlice()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(got), should.Equal(2))
			assert.Loosely(t, got[0].Value, should.Equal("val_a"))
			assert.Loosely(t, got[1].Value, should.Equal("val_b"))
		})

		t.Run("KeysOnly", func(t *testing.T) {
			q := datastore.NewQuery("TestIterRecord")
			got, err := datastore.RunQuery[*datastore.Key](ctx, q).AsSlice()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(got), should.Equal(2))
			assert.Loosely(t, got[0].StringID(), should.Equal("a"))
			assert.Loosely(t, got[1].StringID(), should.Equal("b"))
		})

		t.Run("PropertyMap", func(t *testing.T) {
			q := datastore.NewQuery("TestIterRecord")
			got, err := datastore.RunQuery[datastore.PropertyMap](ctx, q).AsSlice()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(got), should.Equal(2))
			assert.Loosely(t, got[0].Slice("value")[0].Value(), should.Equal("val_a"))
			assert.Loosely(t, got[1].Slice("value")[0].Value(), should.Equal("val_b"))
		})

		t.Run("Empty", func(t *testing.T) {
			q := datastore.NewQuery("NonExistent")
			got, err := datastore.RunQuery[*TestIterRecord](ctx, q).AsSlice()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(got), should.Equal(0))
		})

		t.Run("InvalidQuery", func(t *testing.T) {
			q := datastore.NewQuery("").Lt("invalid", nil)
			got, err := datastore.RunQuery[*TestIterRecord](ctx, q).AsSlice()
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, got, should.BeNil)
		})

		t.Run("SetSizeLimit", func(t *testing.T) {
			q := datastore.NewQuery("TestIterRecord")

			t.Run("Exceeded", func(t *testing.T) {
				it := datastore.RunQuery[*TestIterRecord](ctx, q)
				it.SetSizeLimit(1)
				got, err := it.AsSlice()
				assert.Loosely(t, err, should.Equal(datastore.ErrLimitExceeded))
				assert.Loosely(t, got, should.BeNil)
			})

			t.Run("Disabled", func(t *testing.T) {
				it := datastore.RunQuery[*TestIterRecord](ctx, q)
				it.SetSizeLimit(-1)
				got, err := it.AsSlice()
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(got), should.Equal(2))
			})

			t.Run("Sufficient", func(t *testing.T) {
				it := datastore.RunQuery[*TestIterRecord](ctx, q)
				it.SetSizeLimit(1024 * 1024)
				got, err := it.AsSlice()
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(got), should.Equal(2))
			})
		})
	})
}

func TestRunMultiQuery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = memory.Use(ctx)
	datastore.GetTestable(ctx).Consistent(true)

	err := datastore.Put(ctx, &TestIterRecord{ID: "a", Value: "val_a"})
	assert.NoErr(t, err)
	err = datastore.Put(ctx, &TestIterRecord{ID: "b", Value: "val_b"})
	assert.NoErr(t, err)

	t.Run("PointerToStruct", func(t *testing.T) {
		queries := []*datastore.Query{
			datastore.NewQuery("TestIterRecord").Eq("value", "val_a"),
			datastore.NewQuery("TestIterRecord").Eq("value", "val_b"),
		}
		it := datastore.RunMultiQuery[*TestIterRecord](ctx, queries)
		var got []*TestIterRecord
		for r, err := range it.Results {
			assert.NoErr(t, err)
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
			assert.NoErr(t, err)
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
			assert.NoErr(t, err)
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
			assert.NoErr(t, err)
			got = append(got, pm)
		}
		assert.Loosely(t, len(got), should.Equal(2))
		assert.Loosely(t, got[0]["value"].Slice()[0].Value(), should.Equal("val_a"))
		assert.Loosely(t, got[1]["value"].Slice()[0].Value(), should.Equal("val_b"))
	})

	t.Run("AsSlice", func(t *testing.T) {
		queries := []*datastore.Query{
			datastore.NewQuery("TestIterRecord").Eq("value", "val_a"),
			datastore.NewQuery("TestIterRecord").Eq("value", "val_b"),
		}
		got, err := datastore.RunMultiQuery[*TestIterRecord](ctx, queries).AsSlice()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(got), should.Equal(2))
		assert.Loosely(t, got[0].Value, should.Equal("val_a"))
		assert.Loosely(t, got[1].Value, should.Equal("val_b"))
	})
}

func TestRunQuery_Variadic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = memory.Use(ctx)
	datastore.GetTestable(ctx).Consistent(true)

	err := datastore.Put(ctx, &TestIterRecord{ID: "a", Value: "val_a"})
	assert.NoErr(t, err)
	err = datastore.Put(ctx, &TestIterRecord{ID: "b", Value: "val_b"})
	assert.NoErr(t, err)
	err = datastore.Put(ctx, &TestIterRecord{ID: "c", Value: "val_c"})
	assert.NoErr(t, err)

	q1 := datastore.NewQuery("TestIterRecord").Eq("value", "val_a")
	q2 := datastore.NewQuery("TestIterRecord").Eq("value", "val_b")
	q3 := datastore.NewQuery("TestIterRecord").Eq("value", "val_c")

	t.Run("Zero queries", func(t *testing.T) {
		it := datastore.RunQuery[*TestIterRecord](ctx)
		slice, err := it.AsSlice()
		assert.NoErr(t, err)
		assert.Loosely(t, len(slice), should.Equal(0))

		cur, err := it.Cursor()
		assert.NoErr(t, err)
		assert.Loosely(t, len(cur), should.Equal(0))

		_, err = it.CurrentCursor()
		assert.Loosely(t, err, should.Equal(datastore.ErrNoCurrentCursor))
	})

	t.Run("Single query variadic", func(t *testing.T) {
		slice, err := datastore.RunQuery[*TestIterRecord](ctx, q1).AsSlice()
		assert.NoErr(t, err)
		assert.Loosely(t, len(slice), should.Equal(1))
		assert.Loosely(t, slice[0].Value, should.Equal("val_a"))
	})

	t.Run("Multiple queries AsSlice", func(t *testing.T) {
		slice, err := datastore.RunQuery[*TestIterRecord](ctx, q1, q2, q3).AsSlice()
		assert.NoErr(t, err)
		assert.Loosely(t, len(slice), should.Equal(3))
		assert.Loosely(t, slice[0].Value, should.Equal("val_a"))
		assert.Loosely(t, slice[1].Value, should.Equal("val_b"))
		assert.Loosely(t, slice[2].Value, should.Equal("val_c"))
	})

	t.Run("Multiple queries AddFilter", func(t *testing.T) {
		it := datastore.RunQuery[*TestIterRecord](ctx, q1, q2, q3)
		it.AddFilter(func(r *TestIterRecord) (bool, error) {
			return r.Value != "val_b", nil
		})
		slice, err := it.AsSlice()
		assert.NoErr(t, err)
		assert.Loosely(t, len(slice), should.Equal(2))
		assert.Loosely(t, slice[0].Value, should.Equal("val_a"))
		assert.Loosely(t, slice[1].Value, should.Equal("val_c"))
	})

	t.Run("Multiple queries SetSizeLimit", func(t *testing.T) {
		it := datastore.RunQuery[*TestIterRecord](ctx, q1, q2, q3)
		it.SetSizeLimit(1)
		_, err := it.AsSlice()
		assert.Loosely(t, err, should.Equal(datastore.ErrLimitExceeded))
	})

	t.Run("Multiple queries Cursor before iteration", func(t *testing.T) {
		it := datastore.RunQuery[*TestIterRecord](ctx, q1, q2)
		cur, err := it.Cursor()
		assert.NoErr(t, err)
		assert.Loosely(t, len(cur), should.Equal(2))

		_, err = it.CurrentCursor()
		assert.Loosely(t, err, should.Equal(datastore.ErrNoCurrentCursor))
	})

	t.Run("Multiple queries Step by step Cursor vs CurrentCursor", func(t *testing.T) {
		it := datastore.RunQuery[*TestIterRecord](ctx, q1, q2, q3)
		count := 0
		for r, err := range it.Results {
			assert.NoErr(t, err)
			count++

			curCur, err := it.CurrentCursor()
			assert.NoErr(t, err)
			assert.Loosely(t, len(curCur), should.Equal(3))

			nextCur, err := it.Cursor()
			assert.NoErr(t, err)
			assert.Loosely(t, len(nextCur), should.Equal(3))

			// Resume from CurrentCursor: starts with the current item
			resumedQCur, err := datastore.ApplyCursors(ctx, []*datastore.Query{q1, q2, q3}, curCur)
			assert.NoErr(t, err)
			resCur, err := datastore.RunQuery[*TestIterRecord](ctx, resumedQCur...).AsSlice()
			assert.NoErr(t, err)
			assert.Loosely(t, resCur[0].Value, should.Equal(r.Value))

			// Resume from Cursor: skips the current item
			resumedQNext, err := datastore.ApplyCursors(ctx, []*datastore.Query{q1, q2, q3}, nextCur)
			assert.NoErr(t, err)
			resNext, err := datastore.RunQuery[*TestIterRecord](ctx, resumedQNext...).AsSlice()
			assert.NoErr(t, err)
			assert.Loosely(t, len(resNext), should.Equal(3-count))
		}
		assert.Loosely(t, count, should.Equal(3))
	})

	t.Run("Multiple queries Mismatched Kind", func(t *testing.T) {
		badQ := datastore.NewQuery("OtherKind")
		it := datastore.RunQuery[*TestIterRecord](ctx, q1, badQ)
		for _, err := range it.Results {
			assert.Loosely(t, err, should.ErrLike("should query the same kind"))
		}
	})

	t.Run("Multiple queries Mismatched Order", func(t *testing.T) {
		badQ := datastore.NewQuery("TestIterRecord").Order("-value")
		it := datastore.RunQuery[*TestIterRecord](ctx, q1, badQ)
		for _, err := range it.Results {
			assert.Loosely(t, err, should.ErrLike("should use the same order"))
		}
	})
}

func TestRunQuery_MultipleUses(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = memory.Use(ctx)
	datastore.GetTestable(ctx).Consistent(true)

	err := datastore.Put(ctx, &TestIterRecord{ID: "a", Value: "val_a"})
	assert.NoErr(t, err)
	err = datastore.Put(ctx, &TestIterRecord{ID: "b", Value: "val_b"})
	assert.NoErr(t, err)

	t.Run("RunQuery Full Consumption", func(t *testing.T) {
		q := datastore.NewQuery("TestIterRecord")
		it := datastore.RunQuery[*TestIterRecord](ctx, q)
		count := 0
		for _, err := range it.Results {
			assert.NoErr(t, err)
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
			assert.NoErr(t, err)
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
			assert.NoErr(t, err)
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
			assert.NoErr(t, err)
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

	t.Run("SetSizeLimit after Results", func(t *testing.T) {
		q := datastore.NewQuery("TestIterRecord")
		it := datastore.RunQuery[*TestIterRecord](ctx, q)
		for range it.Results {
			break
		}
		assert.Loosely(t, func() {
			it.SetSizeLimit(100)
		}, should.PanicLike("QueryIter[V].SetSizeLimit: cannot call after using Results"))
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
