// Copyright 2017 The LUCI Authors.
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

package readonly

import (
	"testing"

	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/errors"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReadOnly(t *testing.T) {
	t.Parallel()

	Convey("Test datastore filter", t, func() {
		c := memory.Use(context.Background())

		type Tester struct {
			ID    int `gae:"$id"`
			Value string
		}
		type MutableTester struct {
			ID    int `gae:"$id"`
			Value string
		}

		// Add values to the datastore before applying the filter.
		ds.Put(c, &Tester{ID: 1, Value: "exists 1"})
		ds.Put(c, &MutableTester{ID: 1, Value: "exists 2"})
		ds.GetTestable(c).CatchupIndexes()

		// Apply the read-only filter.
		c = FilterRDS(c, func(k *ds.Key) (ro bool) {
			ro = k.Kind() != "MutableTester"
			return
		})
		So(c, ShouldNotBeNil)

		Convey("Get works.", func() {
			v := Tester{ID: 1}
			So(ds.Get(c, &v), ShouldBeNil)
			So(v.Value, ShouldEqual, "exists 1")
		})

		Convey("Count works.", func() {
			q := ds.NewQuery("Tester")
			cnt, err := ds.Count(c, q)
			So(err, ShouldBeNil)
			So(cnt, ShouldEqual, 1)
		})

		Convey("Put fails with read-only error", func() {
			err := ds.Put(c, &Tester{ID: 1}, &MutableTester{ID: 1, Value: "new"})
			So(err, ShouldResemble, errors.MultiError{
				ErrReadOnly,
				nil,
			})
			// The second put actually worked.
			v := MutableTester{ID: 1}
			So(ds.Get(c, &v), ShouldBeNil)
			So(v.Value, ShouldEqual, "new")
		})

		Convey("Delete fails with read-only error", func() {
			err := ds.Delete(c, &Tester{ID: 1}, &MutableTester{ID: 1})
			So(err, ShouldResemble, errors.MultiError{
				ErrReadOnly,
				nil,
			})
		})

		Convey("AllocateIDs fails with read-only error", func() {
			t1 := Tester{}
			t2 := MutableTester{ID: -1}
			err := ds.AllocateIDs(c, &t1, &t2)
			So(err, ShouldResemble, errors.MultiError{
				ErrReadOnly,
				nil,
			})
			So(t2.ID, ShouldEqual, 0) // allocated
		})

		Convey("In a transaction", func() {
			Convey("Get works.", func() {
				v := Tester{ID: 1}

				err := ds.RunInTransaction(c, func(c context.Context) error {
					return ds.Get(c, &v)
				}, nil)
				So(err, ShouldBeNil)
				So(v.Value, ShouldEqual, "exists 1")
			})

			Convey("Count works.", func() {
				// (Need ancestor filter for transaction query)
				q := ds.NewQuery("Tester").Ancestor(ds.KeyForObj(c, &Tester{ID: 1}))

				var cnt int64
				err := ds.RunInTransaction(c, func(c context.Context) (err error) {
					cnt, err = ds.Count(c, q)
					return
				}, nil)
				So(err, ShouldBeNil)
				So(cnt, ShouldEqual, 1)
			})

			Convey("Put fails with read-only error", func() {
				err := ds.RunInTransaction(c, func(c context.Context) error {
					return ds.Put(c, &Tester{ID: 1})
				}, nil)
				So(err, ShouldEqual, ErrReadOnly)
			})

			Convey("Delete fails with read-only error", func() {
				err := ds.RunInTransaction(c, func(c context.Context) error {
					return ds.Delete(c, &Tester{ID: 1})
				}, nil)
				So(err, ShouldEqual, ErrReadOnly)
			})

			Convey("AllocateIDs fails with read-only error", func() {
				err := ds.RunInTransaction(c, func(c context.Context) error {
					return ds.AllocateIDs(c, make([]Tester, 10))
				}, nil)
				So(err, ShouldNotBeNil)
				So(errors.SingleError(err), ShouldEqual, ErrReadOnly)
			})
		})
	})
}
