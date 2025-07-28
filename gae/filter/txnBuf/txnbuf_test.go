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

package txnBuf

import (
	"bytes"
	"context"
	"math/rand"
	"testing"

	"go.chromium.org/luci/common/data/cmpbin"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/gae/filter/count"
	"go.chromium.org/luci/gae/impl/memory"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
)

type Foo struct {
	ID     int64   `gae:"$id"`
	Parent *ds.Key `gae:"$parent"`

	Value   []int64
	ValueNI []byte `gae:",noindex"`
	Sort    []string
}

func fooShouldHaveValues(c context.Context, t testing.TB, id int64, values []int64, valueNI []byte) {
	t.Helper()

	f := &Foo{ID: id}
	err := ds.Get(c, f)

	if len(values) == 0 && len(valueNI) == 0 {
		assert.That(t, err, should.ErrLike(ds.ErrNoSuchEntity), truth.LineContext())
	} else {
		if len(values) > 0 {
			assert.That(t, f.Value, should.Match(values), truth.LineContext())
		}
		if len(valueNI) > 0 {
			assert.That(t, f.ValueNI, should.Match(valueNI), truth.LineContext())
		}
	}
}

func setFooTo(c context.Context, t testing.TB, id int64, values []int64, valueNI []byte) {
	t.Helper()

	f := &Foo{ID: id, Value: values, ValueNI: valueNI}
	if len(values) == 0 && len(valueNI) == 0 {
		assert.That(t, ds.Delete(c, ds.KeyForObj(c, f)), should.ErrLike(nil), truth.LineContext())
	} else {
		assert.That(t, ds.Put(c, f), should.ErrLike(nil), truth.LineContext())
	}
}

const dataLen = 26

var (
	dataMultiRoot  = make([]*Foo, dataLen)
	dataSingleRoot = make([]*Foo, dataLen)
	hugeField      = make([]byte, DefaultSizeBudget/8)
	hugeData       = make([]*Foo, 11)
	root           = ds.MkKeyContext("something~else", "").MakeKey("Parent", 1)
)

func init() {
	cb := func(i int64) string {
		buf := &bytes.Buffer{}
		cmpbin.WriteInt(buf, i)
		return buf.String()
	}

	rs := rand.NewSource(0)
	nums := make([]string, dataLen)
	for i := range dataMultiRoot {
		id := int64(i + 1)
		nums[i] = cb(id)

		val := make([]int64, (rs.Int63()%dataLen)+1)
		for j := range val {
			r := rs.Int63()
			val[j] = r
		}

		dataMultiRoot[i] = &Foo{ID: id, Value: val}
		dataSingleRoot[i] = &Foo{ID: id, Parent: root, Value: val}
	}

	for i := range hugeField {
		hugeField[i] = byte(i)
	}

	for i := range hugeData {
		hugeData[i] = &Foo{ID: int64(i + 1), ValueNI: hugeField}
	}
}

func mkds(data []*Foo) (under, over *count.DSCounter, c context.Context) {
	c = memory.UseWithAppID(context.Background(), "something~else")

	dataKey := ds.KeyForObj(c, data[0])
	if err := ds.AllocateIDs(c, ds.NewIncompleteKeys(c, 100, dataKey.Kind(), dataKey.Parent())); err != nil {
		panic(err)
	}
	if err := ds.Put(c, data); err != nil {
		panic(err)
	}

	c, under = count.FilterRDS(c)
	c = FilterRDS(c)
	c, over = count.FilterRDS(c)
	return
}

func TestTransactionBuffers(t *testing.T) {
	t.Parallel()

	ftt.Run("Get/Put/Delete", t, func(t *ftt.Test) {
		under, over, c := mkds(dataMultiRoot)
		ds.GetTestable(c).SetTransactionRetryCount(1)

		assert.Loosely(t, under.PutMulti.Total(), should.BeZero)
		assert.Loosely(t, over.PutMulti.Total(), should.BeZero)

		t.Run("Good", func(t *ftt.Test) {
			t.Run("read-only", func(t *ftt.Test) {
				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					fooShouldHaveValues(c, t, 4, dataMultiRoot[3].Value, nil)
					return nil
				}, nil), should.BeNil)
			})

			t.Run("single-level read/write", func(t *ftt.Test) {
				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					fooShouldHaveValues(c, t, 4, dataMultiRoot[3].Value, nil)

					setFooTo(c, t, 4, []int64{1, 2, 3, 4}, nil)
					setFooTo(c, t, 3, []int64{1, 2, 3, 4}, nil)

					// look! it remembers :)
					fooShouldHaveValues(c, t, 4, []int64{1, 2, 3, 4}, nil)
					return nil
				}, nil), should.BeNil)

				// 2 because we are simulating a transaction failure
				assert.Loosely(t, under.PutMulti.Total(), should.Equal(2))

				fooShouldHaveValues(c, t, 3, []int64{1, 2, 3, 4}, nil)
				fooShouldHaveValues(c, t, 4, []int64{1, 2, 3, 4}, nil)
			})

			t.Run("multi-level read/write", func(t *ftt.Test) {
				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					fooShouldHaveValues(c, t, 3, dataMultiRoot[2].Value, nil)

					setFooTo(c, t, 3, []int64{1, 2, 3, 4}, nil)
					setFooTo(c, t, 7, nil, nil)

					vals := []*Foo{
						{ID: 793},
						{ID: 7},
						{ID: 3},
						{ID: 4},
					}
					assert.Loosely(t, ds.Get(c, vals), should.ErrLike(errors.NewMultiError(
						ds.ErrNoSuchEntity,
						ds.ErrNoSuchEntity,
						nil,
						nil,
					)))

					assert.Loosely(t, vals[0].Value, should.BeNil)
					assert.Loosely(t, vals[1].Value, should.BeNil)
					assert.Loosely(t, vals[2].Value, should.Match([]int64{1, 2, 3, 4}))
					assert.Loosely(t, vals[3].Value, should.Match(dataSingleRoot[3].Value))

					// inner, failing, transaction
					assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
						// we can see stuff written in the outer txn
						fooShouldHaveValues(c, t, 7, nil, nil)
						fooShouldHaveValues(c, t, 3, []int64{1, 2, 3, 4}, nil)

						setFooTo(c, t, 3, []int64{10, 20, 30, 40}, nil)

						// disaster strikes!
						return errors.New("whaaaa")
					}, nil), should.ErrLike("whaaaa"))

					fooShouldHaveValues(c, t, 3, []int64{1, 2, 3, 4}, nil)

					// inner, successful, transaction
					assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
						fooShouldHaveValues(c, t, 3, []int64{1, 2, 3, 4}, nil)
						setFooTo(c, t, 3, []int64{10, 20, 30, 40}, nil)
						return nil
					}, nil), should.BeNil)

					// now we see it
					fooShouldHaveValues(c, t, 3, []int64{10, 20, 30, 40}, nil)
					return nil
				}, nil), should.BeNil)

				// 2 because we are simulating a transaction failure
				assert.Loosely(t, under.PutMulti.Total(), should.Equal(2))
				assert.Loosely(t, under.DeleteMulti.Total(), should.Equal(2))

				assert.Loosely(t, over.PutMulti.Total(), should.Equal(8))

				fooShouldHaveValues(c, t, 7, nil, nil)
				fooShouldHaveValues(c, t, 3, []int64{10, 20, 30, 40}, nil)
			})

			t.Run("can allocate IDs from an inner transaction", func(t *ftt.Test) {
				nums := []int64{4, 8, 15, 16, 23, 42}
				k := (*ds.Key)(nil)
				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
						f := &Foo{Value: nums}
						assert.Loosely(t, ds.Put(c, f), should.BeNil)
						k = ds.KeyForObj(c, f)
						return nil
					}, nil), should.BeNil)

					fooShouldHaveValues(c, t, k.IntID(), nums, nil)

					return nil
				}, nil), should.BeNil)

				fooShouldHaveValues(c, t, k.IntID(), nums, nil)
			})
		})

		t.Run("Bad", func(t *ftt.Test) {
			t.Run("too many roots", func(t *ftt.Test) {
				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					for i := 1; i < 26; i++ {
						f := &Foo{ID: int64(i)}
						assert.Loosely(t, ds.Get(c, f), should.BeNil)
						assert.Loosely(t, f, should.Match(dataMultiRoot[i-1]))
					}

					f := &Foo{ID: 7}
					f.Value = []int64{9}
					assert.Loosely(t, ds.Put(c, f), should.BeNil)

					return nil
				}, nil), should.BeNil)

				f := &Foo{ID: 7}
				assert.Loosely(t, ds.Get(c, f), should.BeNil)
				assert.Loosely(t, f.Value, should.Match([]int64{9}))
			})

			t.Run("buffered errors never reach the datastore", func(t *ftt.Test) {
				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					assert.Loosely(t, ds.Put(c, &Foo{ID: 1, Value: []int64{1, 2, 3, 4}}), should.BeNil)
					return errors.New("boop")
				}, nil), should.ErrLike("boop"))
				assert.Loosely(t, under.PutMulti.Total(), should.BeZero)
				assert.Loosely(t, over.PutMulti.Successes(), should.Equal(1))
			})
		})
	})
}

func TestHuge(t *testing.T) {
	t.Parallel()

	ftt.Run("testing datastore enforces thresholds", t, func(t *ftt.Test) {
		_, _, c := mkds(dataMultiRoot)

		t.Run("exceeding inner txn size threshold still allows outer", func(t *ftt.Test) {
			assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
				setFooTo(c, t, 18, nil, hugeField)

				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					assert.Loosely(t, ds.Put(c, hugeData), should.BeNil)
					return nil
				}, nil), should.ErrLike(ErrTransactionTooLarge))

				return nil
			}, nil), should.BeNil)

			fooShouldHaveValues(c, t, 18, nil, hugeField)
		})

		t.Run("exceeding inner txn count threshold still allows outer", func(t *ftt.Test) {
			assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
				setFooTo(c, t, 18, nil, hugeField)

				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					p := ds.MakeKey(c, "mom", 1)

					// This will exceed the budget, since we've already done one write in
					// the parent.
					for i := 1; i <= DefaultWriteCountBudget; i++ {
						assert.Loosely(t, ds.Put(c, &Foo{ID: int64(i), Parent: p}), should.BeNil)
					}
					return nil
				}, nil), should.ErrLike(ErrTransactionTooLarge))

				return nil
			}, nil), should.BeNil)

			fooShouldHaveValues(c, t, 18, nil, hugeField)
		})

		t.Run("exceeding threshold in the parent, then retreating in the child is okay", func(t *ftt.Test) {
			assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
				assert.Loosely(t, ds.Put(c, hugeData), should.BeNil)
				setFooTo(c, t, 18, nil, hugeField)

				// We're over threshold! But the child will delete most of this and
				// bring us back to normal.
				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					keys := make([]*ds.Key, len(hugeData))
					for i, d := range hugeData {
						keys[i] = ds.KeyForObj(c, d)
					}
					return ds.Delete(c, keys)
				}, nil), should.BeNil)

				return nil
			}, nil), should.BeNil)

			fooShouldHaveValues(c, t, 18, nil, hugeField)
		})
	})
}

func TestQuerySupport(t *testing.T) {
	t.Parallel()

	ftt.Run("Queries", t, func(t *ftt.Test) {
		t.Run("Good", func(t *ftt.Test) {
			q := ds.NewQuery("Foo").Ancestor(root)

			t.Run("normal", func(t *ftt.Test) {
				_, _, c := mkds(dataSingleRoot)
				ds.GetTestable(c).AddIndexes(&ds.IndexDefinition{
					Kind:     "Foo",
					Ancestor: true,
					SortBy: []ds.IndexColumn{
						{Property: "Value"},
					},
				})

				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					q = q.Lt("Value", 400000000000000000)

					vals := []*Foo{}
					assert.Loosely(t, ds.GetAll(c, q, &vals), should.BeNil)
					assert.Loosely(t, len(vals), should.Equal(15))

					count, err := ds.Count(c, q)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, count, should.Equal(15))

					f := &Foo{ID: 1, Parent: root}
					assert.Loosely(t, ds.Get(c, f), should.BeNil)
					f.Value = append(f.Value, 100)
					assert.Loosely(t, ds.Put(c, f), should.BeNil)

					// Wowee, zowee, merged queries!
					vals2 := []*Foo{}
					assert.Loosely(t, ds.GetAll(c, q, &vals2), should.BeNil)
					assert.Loosely(t, len(vals2), should.Equal(16))
					assert.Loosely(t, vals2[0], should.Match(f))

					vals2 = []*Foo{}
					assert.Loosely(t, ds.GetAll(c, q.Limit(2).Offset(1), &vals2), should.BeNil)
					assert.Loosely(t, len(vals2), should.Equal(2))
					assert.Loosely(t, vals2, should.Match(vals[:2]))

					return nil
				}, nil), should.BeNil)
			})

			t.Run("keysOnly", func(t *ftt.Test) {
				_, _, c := mkds([]*Foo{
					{ID: 2, Parent: root, Value: []int64{1, 2, 3, 4, 5, 6, 7}},
					{ID: 3, Parent: root, Value: []int64{3, 4, 5, 6, 7, 8, 9}},
					{ID: 4, Parent: root, Value: []int64{3, 5, 7, 9, 11, 100, 1}},
					{ID: 5, Parent: root, Value: []int64{1, 70, 101}},
				})

				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					q = q.Eq("Value", 1).KeysOnly(true)
					vals := []*ds.Key{}
					assert.Loosely(t, ds.GetAll(c, q, &vals), should.BeNil)
					assert.Loosely(t, len(vals), should.Equal(3))
					assert.Loosely(t, vals[2], should.Match(ds.MakeKey(c, "Parent", 1, "Foo", 5)))

					// can remove keys
					assert.Loosely(t, ds.Delete(c, ds.MakeKey(c, "Parent", 1, "Foo", 2)), should.BeNil)
					vals = []*ds.Key{}
					assert.Loosely(t, ds.GetAll(c, q, &vals), should.BeNil)
					assert.Loosely(t, len(vals), should.Equal(2))

					// and add new ones
					assert.Loosely(t, ds.Put(c, &Foo{ID: 1, Parent: root, Value: []int64{1, 7, 100}}), should.BeNil)
					assert.Loosely(t, ds.Put(c, &Foo{ID: 7, Parent: root, Value: []int64{20, 1}}), should.BeNil)
					vals = []*ds.Key{}
					assert.Loosely(t, ds.GetAll(c, q, &vals), should.BeNil)
					assert.Loosely(t, len(vals), should.Equal(4))

					assert.Loosely(t, vals[0].IntID(), should.Equal(1))
					assert.Loosely(t, vals[1].IntID(), should.Equal(4))
					assert.Loosely(t, vals[2].IntID(), should.Equal(5))
					assert.Loosely(t, vals[3].IntID(), should.Equal(7))

					return nil
				}, nil), should.BeNil)
			})

			t.Run("project", func(t *ftt.Test) {
				_, _, c := mkds([]*Foo{
					{ID: 2, Parent: root, Value: []int64{1, 2, 3, 4, 5, 6, 7}},
					{ID: 3, Parent: root, Value: []int64{3, 4, 5, 6, 7, 8, 9}},
					{ID: 4, Parent: root, Value: []int64{3, 5, 7, 9, 11, 100, 1}},
					{ID: 5, Parent: root, Value: []int64{1, 70, 101}},
				})

				ds.GetTestable(c).AddIndexes(&ds.IndexDefinition{
					Kind:     "Foo",
					Ancestor: true,
					SortBy: []ds.IndexColumn{
						{Property: "Value"},
					},
				})

				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					count, err := ds.Count(c, q.Project("Value"))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, count, should.Equal(24))

					q = q.Project("Value").Offset(4).Limit(10)

					vals := []ds.PropertyMap{}
					assert.Loosely(t, ds.GetAll(c, q, &vals), should.BeNil)
					assert.Loosely(t, len(vals), should.Equal(10))

					expect := []struct {
						id  int64
						val int64
					}{
						{2, 3},
						{3, 3},
						{4, 3},
						{2, 4},
						{3, 4},
						{2, 5},
						{3, 5},
						{4, 5},
						{2, 6},
						{3, 6},
					}

					for i, pm := range vals {
						assert.Loosely(t, ds.GetMetaDefault(pm, "key", nil), should.Match(
							ds.MakeKey(c, "Parent", 1, "Foo", expect[i].id)))
						assert.Loosely(t, pm.Slice("Value")[0].Value(), should.Equal(expect[i].val))
					}

					// should remove 4 entries, but there are plenty more to fill
					assert.Loosely(t, ds.Delete(c, ds.MakeKey(c, "Parent", 1, "Foo", 2)), should.BeNil)

					vals = []ds.PropertyMap{}
					assert.Loosely(t, ds.GetAll(c, q, &vals), should.BeNil)
					assert.Loosely(t, len(vals), should.Equal(10))

					expect = []struct {
						id  int64
						val int64
					}{
						// note (3, 3) and (4, 3) are correctly missing because deleting
						// 2 removed two entries which are hidden by the Offset(4).
						{3, 4},
						{3, 5},
						{4, 5},
						{3, 6},
						{3, 7},
						{4, 7},
						{3, 8},
						{3, 9},
						{4, 9},
						{4, 11},
					}

					for i, pm := range vals {
						assert.Loosely(t, ds.GetMetaDefault(pm, "key", nil), should.Match(
							ds.MakeKey(c, "Parent", 1, "Foo", expect[i].id)))
						assert.Loosely(t, pm.Slice("Value")[0].Value(), should.Equal(expect[i].val))
					}

					assert.Loosely(t, ds.Put(c, &Foo{ID: 1, Parent: root, Value: []int64{3, 9}}), should.BeNil)

					vals = []ds.PropertyMap{}
					assert.Loosely(t, ds.GetAll(c, q, &vals), should.BeNil)
					assert.Loosely(t, len(vals), should.Equal(10))

					expect = []struct {
						id  int64
						val int64
					}{
						// 'invisible' {1, 3} entry bumps the {4, 3} into view.
						{4, 3},
						{3, 4},
						{3, 5},
						{4, 5},
						{3, 6},
						{3, 7},
						{4, 7},
						{3, 8},
						{1, 9},
						{3, 9},
						{4, 9},
					}

					for i, pm := range vals {
						assert.Loosely(t, ds.GetMetaDefault(pm, "key", nil), should.Match(
							ds.MakeKey(c, "Parent", 1, "Foo", expect[i].id)))
						assert.Loosely(t, pm.Slice("Value")[0].Value(), should.Equal(expect[i].val))
					}

					return nil
				}, nil), should.BeNil)
			})

			t.Run("project+distinct", func(t *ftt.Test) {
				_, _, c := mkds([]*Foo{
					{ID: 2, Parent: root, Value: []int64{1, 2, 3, 4, 5, 6, 7}},
					{ID: 3, Parent: root, Value: []int64{3, 4, 5, 6, 7, 8, 9}},
					{ID: 4, Parent: root, Value: []int64{3, 5, 7, 9, 11, 100, 1}},
					{ID: 5, Parent: root, Value: []int64{1, 70, 101}},
				})

				ds.GetTestable(c).AddIndexes(&ds.IndexDefinition{
					Kind:     "Foo",
					Ancestor: true,
					SortBy: []ds.IndexColumn{
						{Property: "Value"},
					},
				})

				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					q = q.Project("Value").Distinct(true)

					vals := []ds.PropertyMap{}
					assert.Loosely(t, ds.GetAll(c, q, &vals), should.BeNil)
					assert.Loosely(t, len(vals), should.Equal(13))

					expect := []struct {
						id  int64
						val int64
					}{
						{2, 1},
						{2, 2},
						{2, 3},
						{2, 4},
						{2, 5},
						{2, 6},
						{2, 7},
						{3, 8},
						{3, 9},
						{4, 11},
						{5, 70},
						{4, 100},
						{5, 101},
					}

					for i, pm := range vals {
						assert.Loosely(t, pm.Slice("Value")[0].Value(), should.Equal(expect[i].val))
						assert.Loosely(t, ds.GetMetaDefault(pm, "key", nil), should.Match(
							ds.MakeKey(c, "Parent", 1, "Foo", expect[i].id)))
					}

					return nil
				}, nil), should.BeNil)
			})

			t.Run("overwrite", func(t *ftt.Test) {
				data := []*Foo{
					{ID: 2, Parent: root, Value: []int64{1, 2, 3, 4, 5, 6, 7}},
					{ID: 3, Parent: root, Value: []int64{3, 4, 5, 6, 7, 8, 9}},
					{ID: 4, Parent: root, Value: []int64{3, 5, 7, 9, 11, 100, 1, 2}},
					{ID: 5, Parent: root, Value: []int64{1, 70, 101}},
				}

				_, _, c := mkds(data)

				q = q.Eq("Value", 2, 3)

				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					vals := []*Foo{}
					assert.Loosely(t, ds.GetAll(c, q, &vals), should.BeNil)
					assert.Loosely(t, len(vals), should.Equal(2))

					assert.Loosely(t, vals[0], should.Match(data[0]))
					assert.Loosely(t, vals[1], should.Match(data[2]))

					foo2 := &Foo{ID: 2, Parent: root, Value: []int64{2, 3}}
					assert.Loosely(t, ds.Put(c, foo2), should.BeNil)

					vals = []*Foo{}
					assert.Loosely(t, ds.GetAll(c, q, &vals), should.BeNil)
					assert.Loosely(t, len(vals), should.Equal(2))

					assert.Loosely(t, vals[0], should.Match(foo2))
					assert.Loosely(t, vals[1], should.Match(data[2]))

					foo1 := &Foo{ID: 1, Parent: root, Value: []int64{2, 3}}
					assert.Loosely(t, ds.Put(c, foo1), should.BeNil)

					vals = []*Foo{}
					assert.Loosely(t, ds.GetAll(c, q, &vals), should.BeNil)
					assert.Loosely(t, len(vals), should.Equal(3))

					assert.Loosely(t, vals[0], should.Match(foo1))
					assert.Loosely(t, vals[1], should.Match(foo2))
					assert.Loosely(t, vals[2], should.Match(data[2]))

					return nil
				}, nil), should.BeNil)
			})

			projectData := []*Foo{
				{ID: 2, Parent: root, Value: []int64{1, 2, 3, 4, 5, 6, 7}, Sort: []string{"x", "z"}},
				{ID: 3, Parent: root, Value: []int64{3, 4, 5, 6, 7, 8, 9}, Sort: []string{"b"}},
				{ID: 4, Parent: root, Value: []int64{3, 5, 7, 9, 11, 100, 1, 2}, Sort: []string{"aa", "a"}},
				{ID: 5, Parent: root, Value: []int64{1, 70, 101}, Sort: []string{"c"}},
			}

			t.Run("project+extra orders", func(t *ftt.Test) {
				_, _, c := mkds(projectData)
				ds.GetTestable(c).AddIndexes(&ds.IndexDefinition{
					Kind:     "Foo",
					Ancestor: true,
					SortBy: []ds.IndexColumn{
						{Property: "Sort", Descending: true},
						{Property: "Value", Descending: true},
					},
				})

				q = q.Project("Value").Order("-Sort", "-Value").Distinct(true)
				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					assert.Loosely(t, ds.Put(c, &Foo{
						ID: 1, Parent: root, Value: []int64{0, 1, 1000},
						Sort: []string{"zz"}}), should.BeNil)

					vals := []ds.PropertyMap{}
					assert.Loosely(t, ds.GetAll(c, q, &vals), should.BeNil)

					expect := []struct {
						id  int64
						val int64
					}{
						{1, 1000},
						{1, 1},
						{1, 0},
						{2, 7},
						{2, 6},
						{2, 5},
						{2, 4},
						{2, 3},
						{2, 2},
						{5, 101},
						{5, 70},
						{3, 9},
						{3, 8},
						{4, 100},
						{4, 11},
					}

					for i, pm := range vals {
						assert.Loosely(t, pm.Slice("Value")[0].Value(), should.Equal(expect[i].val))
						assert.Loosely(t, ds.GetMetaDefault(pm, "key", nil), should.Match(
							ds.MakeKey(c, "Parent", 1, "Foo", expect[i].id)))
					}

					return nil
				}, nil), should.BeNil)
			})

			t.Run("buffered entity sorts before ineq, but after first parent entity", func(t *ftt.Test) {
				// If we got this wrong, we'd see Foo,3 come before Foo,2. This might
				// happen because we calculate the comparison string for each entity
				// based on the whole entity, but we forgot to limit the comparison
				// string generation by the inequality criteria.
				data := []*Foo{
					{ID: 2, Parent: root, Value: []int64{2, 3, 5, 6}, Sort: []string{"z"}},
				}

				_, _, c := mkds(data)
				ds.GetTestable(c).AddIndexes(&ds.IndexDefinition{
					Kind:     "Foo",
					Ancestor: true,
					SortBy: []ds.IndexColumn{
						{Property: "Value"},
					},
				})

				q = q.Gt("Value", 2).Limit(2)

				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					foo1 := &Foo{ID: 3, Parent: root, Value: []int64{0, 2, 3, 4}}
					assert.Loosely(t, ds.Put(c, foo1), should.BeNil)

					vals := []*Foo{}
					assert.Loosely(t, ds.GetAll(c, q, &vals), should.BeNil)
					assert.Loosely(t, len(vals), should.Equal(2))

					assert.Loosely(t, vals[0], should.Match(data[0]))
					assert.Loosely(t, vals[1], should.Match(foo1))

					return nil
				}, nil), should.BeNil)
			})

			t.Run("keysOnly+extra orders", func(t *ftt.Test) {
				_, _, c := mkds(projectData)
				ds.GetTestable(c).AddIndexes(&ds.IndexDefinition{
					Kind:     "Foo",
					Ancestor: true,
					SortBy: []ds.IndexColumn{
						{Property: "Sort"},
					},
				})

				q = q.Order("Sort").KeysOnly(true)

				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					assert.Loosely(t, ds.Put(c, &Foo{
						ID: 1, Parent: root, Value: []int64{0, 1, 1000},
						Sort: []string{"x", "zz"}}), should.BeNil)

					assert.Loosely(t, ds.Put(c, &Foo{
						ID: 2, Parent: root, Value: []int64{0, 1, 1000},
						Sort: []string{"zz", "zzz", "zzzz"}}), should.BeNil)

					vals := []*ds.Key{}
					assert.Loosely(t, ds.GetAll(c, q, &vals), should.BeNil)
					assert.Loosely(t, len(vals), should.Equal(5))

					kc := ds.GetKeyContext(c)
					assert.Loosely(t, vals, should.Match([]*ds.Key{
						kc.MakeKey("Parent", 1, "Foo", 4),
						kc.MakeKey("Parent", 1, "Foo", 3),
						kc.MakeKey("Parent", 1, "Foo", 5),
						kc.MakeKey("Parent", 1, "Foo", 1),
						kc.MakeKey("Parent", 1, "Foo", 2),
					}))

					return nil
				}, nil), should.BeNil)
			})

			t.Run("query accross nested transactions", func(t *ftt.Test) {
				_, _, c := mkds(projectData)
				q = q.Eq("Value", 2, 3)

				foo1 := &Foo{ID: 1, Parent: root, Value: []int64{2, 3}}
				foo7 := &Foo{ID: 7, Parent: root, Value: []int64{2, 3}}

				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					assert.Loosely(t, ds.Put(c, foo1), should.BeNil)

					vals := []*Foo{}
					assert.Loosely(t, ds.GetAll(c, q, &vals), should.BeNil)
					assert.Loosely(t, vals, should.Match([]*Foo{foo1, projectData[0], projectData[2]}))

					assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
						vals := []*Foo{}
						assert.Loosely(t, ds.GetAll(c, q, &vals), should.BeNil)
						assert.Loosely(t, vals, should.Match([]*Foo{foo1, projectData[0], projectData[2]}))

						assert.Loosely(t, ds.Delete(c, ds.MakeKey(c, "Parent", 1, "Foo", 4)), should.BeNil)
						assert.Loosely(t, ds.Put(c, foo7), should.BeNil)

						vals = []*Foo{}
						assert.Loosely(t, ds.GetAll(c, q, &vals), should.BeNil)
						assert.Loosely(t, vals, should.Match([]*Foo{foo1, projectData[0], foo7}))

						return nil
					}, nil), should.BeNil)

					vals = []*Foo{}
					assert.Loosely(t, ds.GetAll(c, q, &vals), should.BeNil)
					assert.Loosely(t, vals, should.Match([]*Foo{foo1, projectData[0], foo7}))

					return nil
				}, nil), should.BeNil)

				vals := []*Foo{}
				assert.Loosely(t, ds.GetAll(c, q, &vals), should.BeNil)
				assert.Loosely(t, vals, should.Match([]*Foo{foo1, projectData[0], foo7}))
			})

			t.Run("start transaction from inside query", func(t *ftt.Test) {
				_, _, c := mkds(projectData)
				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					q := ds.NewQuery("Foo").Ancestor(root)
					return ds.Run(c, q, func(pm ds.PropertyMap) {
						assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
							pm["Value"] = append(pm.Slice("Value"), ds.MkProperty("wat"))
							return ds.Put(c, pm)
						}, nil), should.BeNil)
					})
				}, nil), should.BeNil)

				assert.Loosely(t, ds.Run(c, ds.NewQuery("Foo"), func(pm ds.PropertyMap) {
					val := pm.Slice("Value")
					assert.Loosely(t, val[len(val)-1].Value(), should.Match("wat"))
				}), should.BeNil)
			})
		})
	})
}

func TestRegressions(t *testing.T) {
	ftt.Run("Regression tests", t, func(t *ftt.Test) {
		t.Run("can remove namespace from txnBuf filter", func(t *ftt.Test) {
			c := info.MustNamespace(memory.Use(context.Background()), "foobar")
			assert.Loosely(t, info.GetNamespace(c), should.Equal("foobar"))
			ds.RunInTransaction(FilterRDS(c), func(c context.Context) error {
				assert.Loosely(t, info.GetNamespace(c), should.Equal("foobar"))
				c = ds.WithoutTransaction(info.MustNamespace(c, ""))
				assert.Loosely(t, info.GetNamespace(c), should.BeEmpty)
				return nil
			}, nil)
		})
	})
}
