// Copyright 2020 The LUCI Authors.
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
	"bytes"
	"context"
	"strconv"
	"testing"

	"go.chromium.org/luci/gae/impl/memory"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/datastore/types/serialize"

	. "github.com/smartystreets/goconvey/convey"
)

type Foo struct {
	_kind string `gae:"$kind,Foo"`
	ID    int64  `gae:"$id"`

	Val []string `gae:"val"`
}

func TestDatastoreQueryIterator(t *testing.T) {
	t.Parallel()
	Convey("QueryIterator", t, func() {
		Convey("normal", func() {
			qi := QueryIterator{
				order: []ds.IndexColumn{
					{Property: "field1", Descending: true},
					{Property: "field2"},
					{Property: "__key__"},
				},
				itemCh: make(chan *rawQueryResult),
			}

			key := ds.MkKeyContext("s~aid", "ns").MakeKey("testKind", 1)
			// populating results to the pipeline
			go func() {
				defer close(qi.itemCh)
				qi.itemCh <- &rawQueryResult{
					key: key,
					data: ds.PropertyMap{
						"field1": ds.PropertySlice{
							ds.MkProperty("1"),
							ds.MkProperty("11"),
						},
						"field2": ds.MkProperty("aa1"),
					},
				}
			}()
			err := qi.Next()
			So(err, ShouldBeNil)
			So(qi.currentQueryResult, ShouldNotBeNil)

			Convey("CurrentItemKey", func() {

				itemKey := qi.CurrentItemKey()
				expectedKey := ds.MkKeyContext("s~aid", "ns").MakeKey("testKind", 1)
				e := string(serialize.Serialize.ToBytes(expectedKey))
				So(itemKey, ShouldEqual, e)
			})

			Convey("CurrentItemOrder", func() {
				itemOrder, err := qi.CurrentItemOrder()
				So(err, ShouldBeNil)

				invBuf := serialize.Invertible(&bytes.Buffer{})
				invBuf.SetInvert(true)
				err = serialize.Serialize.Property(invBuf, ds.MkProperty(strconv.Itoa(11)))
				invBuf.SetInvert(false)
				err = serialize.Serialize.Property(invBuf, ds.MkProperty("aa1"))
				err = serialize.Serialize.Key(invBuf, key)
				So(err, ShouldBeNil)
				So(itemOrder, ShouldEqual, invBuf.String())
			})

			Convey("CurrentItem", func() {
				key, data := qi.CurrentItem()
				expectedPM := ds.PropertyMap{
					"field1": ds.PropertySlice{
						ds.MkProperty("1"),
						ds.MkProperty("11"),
					},
					"field2": ds.MkProperty("aa1"),
				}
				So(key, ShouldResemble, key)
				So(data, ShouldResemble, expectedPM)
			})

			// end of results
			err = qi.Next()
			So(err, ShouldResemble, ds.Stop)
		})

		Convey("invalid QueryIterator", func() {
			qi := QueryIterator{}
			So(func() { qi.Next() }, ShouldPanicWith,
				"item channel for QueryIterator is not properly initiated")
		})

		Convey("empty query results", func() {
			qi := &QueryIterator{
				order:  []ds.IndexColumn{},
				itemCh: make(chan *rawQueryResult),
			}
			go func() {
				qi.itemCh <- &rawQueryResult{
					key:  nil,
					data: ds.PropertyMap{},
				}
				close(qi.itemCh)
			}()

			err := qi.Next()
			So(err, ShouldBeNil)
			So(qi.CurrentItemKey(), ShouldEqual, "")
			itemOrder, err := qi.CurrentItemOrder()
			So(itemOrder, ShouldEqual, "")
			key, data := qi.CurrentItem()
			So(key, ShouldBeNil)
			So(data, ShouldResemble, ds.PropertyMap{})
		})
	})

	Convey("start QueryIterator", t, func() {
		ctx := memory.Use(context.Background())
		ctx, cancel := context.WithCancel(ctx)
		ds.GetTestable(ctx).AutoIndex(true)
		ds.GetTestable(ctx).Consistent(true)
		So(ds.Put(ctx, &Foo{ID: 1, Val: []string{"aa", "bb"}}), ShouldBeNil)
		So(ds.Put(ctx, &Foo{ID: 2, Val: []string{"aa", "cc"}}), ShouldBeNil)

		Convey("found", func() {
			dq := ds.NewQuery("Foo").Order("val")
			fq, err := dq.Finalize()
			So(err, ShouldBeNil)
			qi := StartQueryIterator(ctx, fq)

			err = qi.Next()
			So(err, ShouldBeNil)
			So(qi.currentQueryResult.key, ShouldResemble, ds.MakeKey(ctx, "Foo", 1))
			So(qi.currentQueryResult.data, ShouldResemble,
				ds.PropertyMap{
					"val": ds.PropertySlice{
						ds.MkProperty("aa"),
						ds.MkProperty("bb"),
					},
				})

			err = qi.Next()
			So(err, ShouldBeNil)
			So(qi.currentQueryResult.key, ShouldResemble, ds.MakeKey(ctx, "Foo", 2))
			So(qi.currentQueryResult.data, ShouldResemble,
				ds.PropertyMap{
					"val": ds.PropertySlice{
						ds.MkProperty("aa"),
						ds.MkProperty("cc"),
					},
				})

			err = qi.Next()
			So(err, ShouldResemble, ds.Stop)
		})

		Convey("cancel", func() {
			dq := ds.NewQuery("Foo").Order("val")
			fq, err := dq.Finalize()
			So(err, ShouldBeNil)
			qi := StartQueryIterator(ctx, fq)

			cancel()
			// When calling `cancel()`, one rawQueryResult may already be put into the itemCh.
			// So it asserts the two possible scenarios: 1) one rawQueryResult with a followed Stop signal.
			// 2) qi.Next() directly returns a Stop signal.
			err = qi.Next()
			if err == nil {
				So(qi.currentQueryResult, ShouldResemble, &rawQueryResult{
					key: ds.MakeKey(ctx, "Foo", 1),
					data: ds.PropertyMap{
						"val": ds.PropertySlice{
							ds.MkProperty("aa"),
							ds.MkProperty("bb"),
						},
					},
				})
				err = qi.Next()
				So(err, ShouldResemble, ds.Stop)
			} else {
				So(err, ShouldResemble, ds.Stop)
			}
		})

		Convey("not found", func() {
			dq := ds.NewQuery("Foo").Order("val")
			dq = dq.Eq("val", "no match")
			fq, err := dq.Finalize()
			So(err, ShouldBeNil)
			qi := StartQueryIterator(ctx, fq)

			err = qi.Next()
			So(err, ShouldResemble, ds.Stop)
		})
	})
}
