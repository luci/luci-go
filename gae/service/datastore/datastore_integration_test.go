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

package datastore_test

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/logging/memlogger"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type Foo struct {
	_kind string `gae:"$kind,Foo"`
	ID    int64  `gae:"$id"`

	MultiVals []string `gae:"multi_vals"`
	SingleVal string   `gae:"single_val"`
	Status    bool     `gae:"status"`
}

func TestRunMulti(t *testing.T) {
	t.Parallel()

	Convey("RunMulti", t, func() {
		ctx := memory.Use(context.Background())
		ctx = memlogger.Use(ctx)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		foos := []*Foo{
			{
				ID:        1,
				MultiVals: []string{"m1", "m2"},
				SingleVal: "s1",
				Status:    true,
			},
			{
				ID:        2,
				MultiVals: []string{"m2", "m3"},
				SingleVal: "s1",
				Status:    false,
			},
			{
				ID:        3,
				MultiVals: []string{"m3", "m4"},
				SingleVal: "s2",
				Status:    true,
			},
			{
				ID:        4,
				MultiVals: []string{"m4", "m5"},
				SingleVal: "s2",
				Status:    false,
			},
		}

		So(datastore.Put(ctx, foos), ShouldBeNil)

		Convey("ok", func() {
			Convey("default ordering", func() {
				queries := []*datastore.Query{
					datastore.NewQuery("Foo").Gte("__key__", datastore.KeyForObj(ctx, &Foo{ID: 1})),
				}
				var res []*Foo
				err := datastore.RunMulti(ctx, queries, func(foo *Foo) error {
					res = append(res, foo)
					return nil
				})
				So(err, ShouldBeNil)
				So(res, ShouldResemble, foos)
			})

			Convey("querying key only", func() {
				queries := []*datastore.Query{
					datastore.NewQuery("Foo").Eq("multi_vals", "m2"),
					datastore.NewQuery("Foo").Eq("multi_vals", "m3"),
				}
				var keys []*datastore.Key
				err := datastore.RunMulti(ctx, queries, func(k *datastore.Key) error {
					keys = append(keys, k)
					return nil
				})
				So(err, ShouldBeNil)
				So(keys, ShouldResemble, []*datastore.Key{
					datastore.KeyForObj(ctx, foos[0]),
					datastore.KeyForObj(ctx, foos[1]),
					datastore.KeyForObj(ctx, foos[2]),
				})
			})

			Convey("overlapped results with non-default orders", func() {
				queries := []*datastore.Query{
					datastore.NewQuery("Foo").Eq("multi_vals", "m2").Order("-single_val", "status"),
					datastore.NewQuery("Foo").Eq("multi_vals", "m3").Order("-single_val", "status"),
				}
				var res []*Foo
				err := datastore.RunMulti(ctx, queries, func(foo *Foo) error {
					res = append(res, foo)
					return nil
				})
				So(err, ShouldBeNil)
				So(res, ShouldResemble, []*Foo{foos[2], foos[1], foos[0]})
			})

			Convey("send Stop in cb", func() {
				queries := []*datastore.Query{
					datastore.NewQuery("Foo").Eq("multi_vals", "m2"),
					datastore.NewQuery("Foo").Eq("multi_vals", "m3"),
				}
				var res []*Foo
				err := datastore.RunMulti(ctx, queries, func(foo *Foo) error {
					if len(res) == 2 {
						return datastore.Stop
					}
					res = append(res, foo)
					return nil
				})
				So(err, ShouldBeNil)
				So(res, ShouldResemble, []*Foo{foos[0], foos[1]})
			})

			Convey("not found", func() {
				queries := []*datastore.Query{
					datastore.NewQuery("Foo").Eq("single_val", "non-existent"),
				}
				var res []*Foo
				err := datastore.RunMulti(ctx, queries, func(foo *Foo) error {
					res = append(res, foo)
					return nil
				})
				So(err, ShouldBeNil)
				So(res, ShouldBeNil)
			})
		})

		Convey("bad", func() {
			Convey("cb with cursorCB", func() {
				queries := []*datastore.Query{
					datastore.NewQuery("Foo"),
				}

				err := datastore.RunMulti(ctx, queries, func(foo *Foo, c datastore.CursorCB) error {
					return nil
				})
				So(err, ShouldErrLike, "datastore: RunMulti doesn't support CursorCB.")
			})
		})
	})
}
