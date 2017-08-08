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
	"fmt"
	"math/rand"
	"testing"

	"go.chromium.org/gae/filter/count"
	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"

	"go.chromium.org/luci/common/data/cmpbin"
	"go.chromium.org/luci/common/errors"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type Foo struct {
	ID     int64   `gae:"$id"`
	Parent *ds.Key `gae:"$parent"`

	Value   []int64
	ValueNI []byte `gae:",noindex"`
	Sort    []string
}

func toIntSlice(stuff []interface{}) []int64 {
	vals, ok := stuff[0].([]int64)
	if !ok {
		vals = make([]int64, len(stuff))
		for i := range vals {
			vals[i] = int64(stuff[i].(int))
		}
	}
	return vals
}

func toInt64(thing interface{}) int64 {
	switch x := thing.(type) {
	case int:
		return int64(x)
	case int64:
		return x
	default:
		panic(fmt.Errorf("wat r it? %v", x))
	}
}

func fooShouldHave(c context.Context) func(interface{}, ...interface{}) string {
	return func(id interface{}, values ...interface{}) string {
		f := &Foo{ID: toInt64(id)}
		err := ds.Get(c, f)
		if len(values) == 0 {
			return ShouldEqual(err, ds.ErrNoSuchEntity)
		}

		ret := ShouldBeNil(err)
		if ret == "" {
			if data, ok := values[0].([]byte); ok {
				ret = ShouldResemble(f.ValueNI, data)
			} else {
				ret = ShouldResemble(f.Value, toIntSlice(values))
			}
		}
		return ret
	}
}

func fooSetTo(c context.Context) func(interface{}, ...interface{}) string {
	return func(id interface{}, values ...interface{}) string {
		f := &Foo{ID: toInt64(id)}
		if len(values) == 0 {
			return ShouldBeNil(ds.Delete(c, ds.KeyForObj(c, f)))
		}
		if data, ok := values[0].([]byte); ok {
			f.ValueNI = data
		} else {
			f.Value = toIntSlice(values)
		}
		return ShouldBeNil(ds.Put(c, f))
	}
}

var (
	dataMultiRoot  = make([]*Foo, 20)
	dataSingleRoot = make([]*Foo, 20)
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
	nums := make([]string, 20)
	for i := range dataMultiRoot {
		id := int64(i + 1)
		nums[i] = cb(id)

		val := make([]int64, rs.Int63()%20)
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

	Convey("Get/Put/Delete", t, func() {
		under, over, c := mkds(dataMultiRoot)
		ds.GetTestable(c).SetTransactionRetryCount(1)

		So(under.PutMulti.Total(), ShouldEqual, 0)
		So(over.PutMulti.Total(), ShouldEqual, 0)

		Convey("Good", func() {
			Convey("read-only", func() {
				So(ds.RunInTransaction(c, func(c context.Context) error {
					So(4, fooShouldHave(c), dataMultiRoot[3].Value)
					return nil
				}, nil), ShouldBeNil)
			})

			Convey("single-level read/write", func() {
				So(ds.RunInTransaction(c, func(c context.Context) error {
					So(4, fooShouldHave(c), dataMultiRoot[3].Value)

					So(4, fooSetTo(c), 1, 2, 3, 4)

					So(3, fooSetTo(c), 1, 2, 3, 4)

					// look! it remembers :)
					So(4, fooShouldHave(c), 1, 2, 3, 4)
					return nil
				}, &ds.TransactionOptions{XG: true}), ShouldBeNil)

				// 2 because we are simulating a transaction failure
				So(under.PutMulti.Total(), ShouldEqual, 2)

				So(3, fooShouldHave(c), 1, 2, 3, 4)
				So(4, fooShouldHave(c), 1, 2, 3, 4)
			})

			Convey("multi-level read/write", func() {
				So(ds.RunInTransaction(c, func(c context.Context) error {
					So(3, fooShouldHave(c), dataMultiRoot[2].Value)

					So(3, fooSetTo(c), 1, 2, 3, 4)
					So(7, fooSetTo(c))

					vals := []*Foo{
						{ID: 793},
						{ID: 7},
						{ID: 3},
						{ID: 4},
					}
					So(ds.Get(c, vals), ShouldResemble, errors.NewMultiError(
						ds.ErrNoSuchEntity,
						ds.ErrNoSuchEntity,
						nil,
						nil,
					))

					So(vals[0].Value, ShouldBeNil)
					So(vals[1].Value, ShouldBeNil)
					So(vals[2].Value, ShouldResemble, []int64{1, 2, 3, 4})
					So(vals[3].Value, ShouldResemble, dataSingleRoot[3].Value)

					// inner, failing, transaction
					So(ds.RunInTransaction(c, func(c context.Context) error {
						// we can see stuff written in the outer txn
						So(7, fooShouldHave(c))
						So(3, fooShouldHave(c), 1, 2, 3, 4)

						So(3, fooSetTo(c), 10, 20, 30, 40)

						// disaster strikes!
						return errors.New("whaaaa")
					}, nil), ShouldErrLike, "whaaaa")

					So(3, fooShouldHave(c), 1, 2, 3, 4)

					// inner, successful, transaction
					So(ds.RunInTransaction(c, func(c context.Context) error {
						So(3, fooShouldHave(c), 1, 2, 3, 4)
						So(3, fooSetTo(c), 10, 20, 30, 40)
						return nil
					}, nil), ShouldBeNil)

					// now we see it
					So(3, fooShouldHave(c), 10, 20, 30, 40)
					return nil
				}, &ds.TransactionOptions{XG: true}), ShouldBeNil)

				// 2 because we are simulating a transaction failure
				So(under.PutMulti.Total(), ShouldEqual, 2)
				So(under.DeleteMulti.Total(), ShouldEqual, 2)

				So(over.PutMulti.Total(), ShouldEqual, 8)

				So(7, fooShouldHave(c))
				So(3, fooShouldHave(c), 10, 20, 30, 40)
			})

			Convey("can allocate IDs from an inner transaction", func() {
				nums := []int64{4, 8, 15, 16, 23, 42}
				k := (*ds.Key)(nil)
				So(ds.RunInTransaction(c, func(c context.Context) error {
					So(ds.RunInTransaction(c, func(c context.Context) error {
						f := &Foo{Value: nums}
						So(ds.Put(c, f), ShouldBeNil)
						k = ds.KeyForObj(c, f)
						return nil
					}, nil), ShouldBeNil)

					So(k.IntID(), fooShouldHave(c), nums)

					return nil
				}, nil), ShouldBeNil)

				So(k.IntID(), fooShouldHave(c), nums)
			})

		})

		Convey("Bad", func() {

			Convey("too many roots", func() {
				So(ds.RunInTransaction(c, func(c context.Context) error {
					f := &Foo{ID: 7}
					So(ds.Get(c, f), ShouldBeNil)
					So(f, ShouldResemble, dataMultiRoot[6])

					So(ds.RunInTransaction(c, func(c context.Context) error {
						return ds.Get(c, &Foo{ID: 5})
					}, nil), ShouldErrLike, "too many entity groups")

					f.Value = []int64{9}
					So(ds.Put(c, f), ShouldBeNil)

					return nil
				}, nil), ShouldBeNil)

				f := &Foo{ID: 7}
				So(ds.Get(c, f), ShouldBeNil)
				So(f.Value, ShouldResemble, []int64{9})
			})

			Convey("buffered errors never reach the datastore", func() {
				So(ds.RunInTransaction(c, func(c context.Context) error {
					So(ds.Put(c, &Foo{ID: 1, Value: []int64{1, 2, 3, 4}}), ShouldBeNil)
					return errors.New("boop")
				}, nil), ShouldErrLike, "boop")
				So(under.PutMulti.Total(), ShouldEqual, 0)
				So(over.PutMulti.Successes(), ShouldEqual, 1)
			})

		})

	})
}

func TestHuge(t *testing.T) {
	t.Parallel()

	Convey("testing datastore enforces thresholds", t, func() {
		_, _, c := mkds(dataMultiRoot)

		Convey("exceeding inner txn size threshold still allows outer", func() {
			So(ds.RunInTransaction(c, func(c context.Context) error {
				So(18, fooSetTo(c), hugeField)

				So(ds.RunInTransaction(c, func(c context.Context) error {
					So(ds.Put(c, hugeData), ShouldBeNil)
					return nil
				}, nil), ShouldErrLike, ErrTransactionTooLarge)

				return nil
			}, &ds.TransactionOptions{XG: true}), ShouldBeNil)

			So(18, fooShouldHave(c), hugeField)
		})

		Convey("exceeding inner txn count threshold still allows outer", func() {
			So(ds.RunInTransaction(c, func(c context.Context) error {
				So(18, fooSetTo(c), hugeField)

				So(ds.RunInTransaction(c, func(c context.Context) error {
					p := ds.MakeKey(c, "mom", 1)

					// This will exceed the budget, since we've already done one write in
					// the parent.
					for i := 1; i <= DefaultWriteCountBudget; i++ {
						So(ds.Put(c, &Foo{ID: int64(i), Parent: p}), ShouldBeNil)
					}
					return nil
				}, nil), ShouldErrLike, ErrTransactionTooLarge)

				return nil
			}, &ds.TransactionOptions{XG: true}), ShouldBeNil)

			So(18, fooShouldHave(c), hugeField)
		})

		Convey("exceeding threshold in the parent, then retreating in the child is okay", func() {
			So(ds.RunInTransaction(c, func(c context.Context) error {
				So(ds.Put(c, hugeData), ShouldBeNil)
				So(18, fooSetTo(c), hugeField)

				// We're over threshold! But the child will delete most of this and
				// bring us back to normal.
				So(ds.RunInTransaction(c, func(c context.Context) error {
					keys := make([]*ds.Key, len(hugeData))
					for i, d := range hugeData {
						keys[i] = ds.KeyForObj(c, d)
					}
					return ds.Delete(c, keys)
				}, nil), ShouldBeNil)

				return nil
			}, &ds.TransactionOptions{XG: true}), ShouldBeNil)

			So(18, fooShouldHave(c), hugeField)
		})
	})
}

func TestQuerySupport(t *testing.T) {
	t.Parallel()

	Convey("Queries", t, func() {
		Convey("Good", func() {
			q := ds.NewQuery("Foo").Ancestor(root)

			Convey("normal", func() {
				_, _, c := mkds(dataSingleRoot)
				ds.GetTestable(c).AddIndexes(&ds.IndexDefinition{
					Kind:     "Foo",
					Ancestor: true,
					SortBy: []ds.IndexColumn{
						{Property: "Value"},
					},
				})

				So(ds.RunInTransaction(c, func(c context.Context) error {
					q = q.Lt("Value", 400000000000000000)

					vals := []*Foo{}
					So(ds.GetAll(c, q, &vals), ShouldBeNil)
					So(len(vals), ShouldEqual, 8)

					count, err := ds.Count(c, q)
					So(err, ShouldBeNil)
					So(count, ShouldEqual, 8)

					f := &Foo{ID: 1, Parent: root}
					So(ds.Get(c, f), ShouldBeNil)
					f.Value = append(f.Value, 100)
					So(ds.Put(c, f), ShouldBeNil)

					// Wowee, zowee, merged queries!
					vals2 := []*Foo{}
					So(ds.GetAll(c, q, &vals2), ShouldBeNil)
					So(len(vals2), ShouldEqual, 9)
					So(vals2[0], ShouldResemble, f)

					vals2 = []*Foo{}
					So(ds.GetAll(c, q.Limit(2).Offset(1), &vals2), ShouldBeNil)
					So(len(vals2), ShouldEqual, 2)
					So(vals2, ShouldResemble, vals[:2])

					return nil
				}, nil), ShouldBeNil)
			})

			Convey("keysOnly", func() {
				_, _, c := mkds([]*Foo{
					{ID: 2, Parent: root, Value: []int64{1, 2, 3, 4, 5, 6, 7}},
					{ID: 3, Parent: root, Value: []int64{3, 4, 5, 6, 7, 8, 9}},
					{ID: 4, Parent: root, Value: []int64{3, 5, 7, 9, 11, 100, 1}},
					{ID: 5, Parent: root, Value: []int64{1, 70, 101}},
				})

				So(ds.RunInTransaction(c, func(c context.Context) error {
					q = q.Eq("Value", 1).KeysOnly(true)
					vals := []*ds.Key{}
					So(ds.GetAll(c, q, &vals), ShouldBeNil)
					So(len(vals), ShouldEqual, 3)
					So(vals[2], ShouldResemble, ds.MakeKey(c, "Parent", 1, "Foo", 5))

					// can remove keys
					So(ds.Delete(c, ds.MakeKey(c, "Parent", 1, "Foo", 2)), ShouldBeNil)
					vals = []*ds.Key{}
					So(ds.GetAll(c, q, &vals), ShouldBeNil)
					So(len(vals), ShouldEqual, 2)

					// and add new ones
					So(ds.Put(c, &Foo{ID: 1, Parent: root, Value: []int64{1, 7, 100}}), ShouldBeNil)
					So(ds.Put(c, &Foo{ID: 7, Parent: root, Value: []int64{20, 1}}), ShouldBeNil)
					vals = []*ds.Key{}
					So(ds.GetAll(c, q, &vals), ShouldBeNil)
					So(len(vals), ShouldEqual, 4)

					So(vals[0].IntID(), ShouldEqual, 1)
					So(vals[1].IntID(), ShouldEqual, 4)
					So(vals[2].IntID(), ShouldEqual, 5)
					So(vals[3].IntID(), ShouldEqual, 7)

					return nil
				}, nil), ShouldBeNil)
			})

			Convey("project", func() {
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

				So(ds.RunInTransaction(c, func(c context.Context) error {
					count, err := ds.Count(c, q.Project("Value"))
					So(err, ShouldBeNil)
					So(count, ShouldEqual, 24)

					q = q.Project("Value").Offset(4).Limit(10)

					vals := []ds.PropertyMap{}
					So(ds.GetAll(c, q, &vals), ShouldBeNil)
					So(len(vals), ShouldEqual, 10)

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
						So(ds.GetMetaDefault(pm, "key", nil), ShouldResemble,
							ds.MakeKey(c, "Parent", 1, "Foo", expect[i].id))
						So(pm.Slice("Value")[0].Value(), ShouldEqual, expect[i].val)
					}

					// should remove 4 entries, but there are plenty more to fill
					So(ds.Delete(c, ds.MakeKey(c, "Parent", 1, "Foo", 2)), ShouldBeNil)

					vals = []ds.PropertyMap{}
					So(ds.GetAll(c, q, &vals), ShouldBeNil)
					So(len(vals), ShouldEqual, 10)

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
						So(ds.GetMetaDefault(pm, "key", nil), ShouldResemble,
							ds.MakeKey(c, "Parent", 1, "Foo", expect[i].id))
						So(pm.Slice("Value")[0].Value(), ShouldEqual, expect[i].val)
					}

					So(ds.Put(c, &Foo{ID: 1, Parent: root, Value: []int64{3, 9}}), ShouldBeNil)

					vals = []ds.PropertyMap{}
					So(ds.GetAll(c, q, &vals), ShouldBeNil)
					So(len(vals), ShouldEqual, 10)

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
						So(ds.GetMetaDefault(pm, "key", nil), ShouldResemble,
							ds.MakeKey(c, "Parent", 1, "Foo", expect[i].id))
						So(pm.Slice("Value")[0].Value(), ShouldEqual, expect[i].val)
					}

					return nil
				}, nil), ShouldBeNil)

			})

			Convey("project+distinct", func() {
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

				So(ds.RunInTransaction(c, func(c context.Context) error {
					q = q.Project("Value").Distinct(true)

					vals := []ds.PropertyMap{}
					So(ds.GetAll(c, q, &vals), ShouldBeNil)
					So(len(vals), ShouldEqual, 13)

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
						So(pm.Slice("Value")[0].Value(), ShouldEqual, expect[i].val)
						So(ds.GetMetaDefault(pm, "key", nil), ShouldResemble,
							ds.MakeKey(c, "Parent", 1, "Foo", expect[i].id))
					}

					return nil
				}, nil), ShouldBeNil)
			})

			Convey("overwrite", func() {
				data := []*Foo{
					{ID: 2, Parent: root, Value: []int64{1, 2, 3, 4, 5, 6, 7}},
					{ID: 3, Parent: root, Value: []int64{3, 4, 5, 6, 7, 8, 9}},
					{ID: 4, Parent: root, Value: []int64{3, 5, 7, 9, 11, 100, 1, 2}},
					{ID: 5, Parent: root, Value: []int64{1, 70, 101}},
				}

				_, _, c := mkds(data)

				q = q.Eq("Value", 2, 3)

				So(ds.RunInTransaction(c, func(c context.Context) error {
					vals := []*Foo{}
					So(ds.GetAll(c, q, &vals), ShouldBeNil)
					So(len(vals), ShouldEqual, 2)

					So(vals[0], ShouldResemble, data[0])
					So(vals[1], ShouldResemble, data[2])

					foo2 := &Foo{ID: 2, Parent: root, Value: []int64{2, 3}}
					So(ds.Put(c, foo2), ShouldBeNil)

					vals = []*Foo{}
					So(ds.GetAll(c, q, &vals), ShouldBeNil)
					So(len(vals), ShouldEqual, 2)

					So(vals[0], ShouldResemble, foo2)
					So(vals[1], ShouldResemble, data[2])

					foo1 := &Foo{ID: 1, Parent: root, Value: []int64{2, 3}}
					So(ds.Put(c, foo1), ShouldBeNil)

					vals = []*Foo{}
					So(ds.GetAll(c, q, &vals), ShouldBeNil)
					So(len(vals), ShouldEqual, 3)

					So(vals[0], ShouldResemble, foo1)
					So(vals[1], ShouldResemble, foo2)
					So(vals[2], ShouldResemble, data[2])

					return nil
				}, nil), ShouldBeNil)
			})

			projectData := []*Foo{
				{ID: 2, Parent: root, Value: []int64{1, 2, 3, 4, 5, 6, 7}, Sort: []string{"x", "z"}},
				{ID: 3, Parent: root, Value: []int64{3, 4, 5, 6, 7, 8, 9}, Sort: []string{"b"}},
				{ID: 4, Parent: root, Value: []int64{3, 5, 7, 9, 11, 100, 1, 2}, Sort: []string{"aa", "a"}},
				{ID: 5, Parent: root, Value: []int64{1, 70, 101}, Sort: []string{"c"}},
			}

			Convey("project+extra orders", func() {

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
				So(ds.RunInTransaction(c, func(c context.Context) error {
					So(ds.Put(c, &Foo{
						ID: 1, Parent: root, Value: []int64{0, 1, 1000},
						Sort: []string{"zz"}}), ShouldBeNil)

					vals := []ds.PropertyMap{}
					So(ds.GetAll(c, q, &vals), ShouldBeNil)

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
						So(pm.Slice("Value")[0].Value(), ShouldEqual, expect[i].val)
						So(ds.GetMetaDefault(pm, "key", nil), ShouldResemble,
							ds.MakeKey(c, "Parent", 1, "Foo", expect[i].id))
					}

					return nil
				}, nil), ShouldBeNil)
			})

			Convey("buffered entity sorts before ineq, but after first parent entity", func() {
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

				So(ds.RunInTransaction(c, func(c context.Context) error {
					foo1 := &Foo{ID: 3, Parent: root, Value: []int64{0, 2, 3, 4}}
					So(ds.Put(c, foo1), ShouldBeNil)

					vals := []*Foo{}
					So(ds.GetAll(c, q, &vals), ShouldBeNil)
					So(len(vals), ShouldEqual, 2)

					So(vals[0], ShouldResemble, data[0])
					So(vals[1], ShouldResemble, foo1)

					return nil
				}, nil), ShouldBeNil)
			})

			Convey("keysOnly+extra orders", func() {
				_, _, c := mkds(projectData)
				ds.GetTestable(c).AddIndexes(&ds.IndexDefinition{
					Kind:     "Foo",
					Ancestor: true,
					SortBy: []ds.IndexColumn{
						{Property: "Sort"},
					},
				})

				q = q.Order("Sort").KeysOnly(true)

				So(ds.RunInTransaction(c, func(c context.Context) error {
					So(ds.Put(c, &Foo{
						ID: 1, Parent: root, Value: []int64{0, 1, 1000},
						Sort: []string{"x", "zz"}}), ShouldBeNil)

					So(ds.Put(c, &Foo{
						ID: 2, Parent: root, Value: []int64{0, 1, 1000},
						Sort: []string{"zz", "zzz", "zzzz"}}), ShouldBeNil)

					vals := []*ds.Key{}
					So(ds.GetAll(c, q, &vals), ShouldBeNil)
					So(len(vals), ShouldEqual, 5)

					kc := ds.GetKeyContext(c)
					So(vals, ShouldResemble, []*ds.Key{
						kc.MakeKey("Parent", 1, "Foo", 4),
						kc.MakeKey("Parent", 1, "Foo", 3),
						kc.MakeKey("Parent", 1, "Foo", 5),
						kc.MakeKey("Parent", 1, "Foo", 1),
						kc.MakeKey("Parent", 1, "Foo", 2),
					})

					return nil
				}, nil), ShouldBeNil)
			})

			Convey("query accross nested transactions", func() {
				_, _, c := mkds(projectData)
				q = q.Eq("Value", 2, 3)

				foo1 := &Foo{ID: 1, Parent: root, Value: []int64{2, 3}}
				foo7 := &Foo{ID: 7, Parent: root, Value: []int64{2, 3}}

				So(ds.RunInTransaction(c, func(c context.Context) error {
					So(ds.Put(c, foo1), ShouldBeNil)

					vals := []*Foo{}
					So(ds.GetAll(c, q, &vals), ShouldBeNil)
					So(vals, ShouldResemble, []*Foo{foo1, projectData[0], projectData[2]})

					So(ds.RunInTransaction(c, func(c context.Context) error {
						vals := []*Foo{}
						So(ds.GetAll(c, q, &vals), ShouldBeNil)
						So(vals, ShouldResemble, []*Foo{foo1, projectData[0], projectData[2]})

						So(ds.Delete(c, ds.MakeKey(c, "Parent", 1, "Foo", 4)), ShouldBeNil)
						So(ds.Put(c, foo7), ShouldBeNil)

						vals = []*Foo{}
						So(ds.GetAll(c, q, &vals), ShouldBeNil)
						So(vals, ShouldResemble, []*Foo{foo1, projectData[0], foo7})

						return nil
					}, nil), ShouldBeNil)

					vals = []*Foo{}
					So(ds.GetAll(c, q, &vals), ShouldBeNil)
					So(vals, ShouldResemble, []*Foo{foo1, projectData[0], foo7})

					return nil
				}, nil), ShouldBeNil)

				vals := []*Foo{}
				So(ds.GetAll(c, q, &vals), ShouldBeNil)
				So(vals, ShouldResemble, []*Foo{foo1, projectData[0], foo7})

			})

			Convey("start transaction from inside query", func() {
				_, _, c := mkds(projectData)
				So(ds.RunInTransaction(c, func(c context.Context) error {
					q := ds.NewQuery("Foo").Ancestor(root)
					return ds.Run(c, q, func(pm ds.PropertyMap) {
						So(ds.RunInTransaction(c, func(c context.Context) error {
							pm["Value"] = append(pm.Slice("Value"), ds.MkProperty("wat"))
							return ds.Put(c, pm)
						}, nil), ShouldBeNil)
					})
				}, &ds.TransactionOptions{XG: true}), ShouldBeNil)

				So(ds.Run(c, ds.NewQuery("Foo"), func(pm ds.PropertyMap) {
					val := pm.Slice("Value")
					So(val[len(val)-1].Value(), ShouldResemble, "wat")
				}), ShouldBeNil)
			})

		})

	})

}

func TestRegressions(t *testing.T) {
	Convey("Regression tests", t, func() {
		Convey("can remove namespace from txnBuf filter", func() {
			c := info.MustNamespace(memory.Use(context.Background()), "foobar")
			So(info.GetNamespace(c), ShouldEqual, "foobar")
			ds.RunInTransaction(FilterRDS(c), func(c context.Context) error {
				So(info.GetNamespace(c), ShouldEqual, "foobar")
				c = ds.WithoutTransaction(info.MustNamespace(c, ""))
				So(info.GetNamespace(c), ShouldEqual, "")
				return nil
			}, nil)
		})
	})
}
