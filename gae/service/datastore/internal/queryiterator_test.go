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
	"errors"
	"strconv"
	"testing"

	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/datastore/serialize"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDatastoreQueryIterator(t *testing.T) {
	Convey("QueryIterator", t, func() {
		Convey("normal", func() {
			qi := NewQueryIterator([]ds.IndexColumn{
				{Property: "__key__"},
				{Property: "field1", Descending: true},
				{Property: "field2"},
			})

			// populating results to the pipeline
			go func() {
				for i := 1; i < 2; i++ {
					qi.itemCh <- &rawQueryResult{
						key: ds.MkKeyContext("s~aid", "ns").MakeKey("testKind", i),
						data: ds.PropertyMap{
							"field1": ds.PropertySlice{
								ds.MkProperty(strconv.Itoa(i)),
								ds.MkProperty(strconv.Itoa(i*10 + i)),
							},
							"field2": ds.MkProperty("aa" + strconv.Itoa(i)),
						},
					}
				}
				close(qi.itemCh)
			}()

			err := qi.Next()
			So(err, ShouldBeNil)
			So(qi.currentQueryResult, ShouldNotBeNil)

			// test CurrentItemOrder
			itemOrder, err := qi.CurrentItemOrder()
			So(err, ShouldBeNil)
			invBuf:= serialize.Invertible(&bytes.Buffer{})
			invBuf.SetInvert(true)
			err = serialize.WriteProperty(invBuf, false, ds.MkProperty(strconv.Itoa(11)))
			err = serialize.WriteProperty(invBuf, false, ds.MkProperty(strconv.Itoa(1)))
			invBuf.SetInvert(false)
			err = serialize.WriteProperty(invBuf, false, ds.MkProperty("aa1"))
			So(err, ShouldBeNil)
			So(itemOrder, ShouldEqual, invBuf.String())

			// test CurrentItemKey
			itemKey := qi.CurrentItemKey()
			expectedKey := ds.MkKeyContext("s~aid", "ns").MakeKey("testKind", 1)
			e := string(serialize.ToBytes(expectedKey))
			So(itemKey, ShouldEqual, e)

			// test CurrentItem
			key, data := qi.CurrentItem()
			expectedPM := ds.PropertyMap{
				"field1": ds.PropertySlice{
					ds.MkProperty("1"),
					ds.MkProperty("11"),
				},
				"field2": ds.MkProperty("aa1"),
			}
			So(key, ShouldResemble, expectedKey)
			So(data, ShouldResemble, expectedPM)

			// end of results
			err = qi.Next()
			So(err, ShouldResemble, ds.Stop)
		})

		Convey("invalid QueryIterator", func() {
			qi := QueryIterator{}
			err := qi.Next()
			So(err, ShouldBeError, errors.New("item channel for QueryIterator is not properly initiated"))
		})

		Convey("empty query results", func() {
			qi := NewQueryIterator([]ds.IndexColumn{})
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
}