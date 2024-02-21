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

	"go.chromium.org/luci/common/errors"
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
			Convey("cb with cursorCB", func() {
				queries := []*datastore.Query{
					datastore.NewQuery("Foo").Eq("single_val", "s1"),
					datastore.NewQuery("Foo").Eq("single_val", "s2"),
				}
				var cur datastore.Cursor
				var err error
				var fooses []*Foo
				err = datastore.RunMulti(ctx, queries, func(foo *Foo, c datastore.CursorCB) error {
					fooses = append(fooses, foo)
					if len(fooses) == 1 {
						cur, err = c()
						if err != nil {
							return err
						}
						return datastore.Stop
					}
					return nil
				})
				So(err, ShouldBeNil)
				So(fooses, ShouldNotBeNil)
				So(fooses, ShouldResemble, []*Foo{foos[0]})
				// Apply the cursor to the queries
				queries, err = datastore.ApplyCursors(ctx, queries, cur)
				So(err, ShouldBeNil)
				So(queries, ShouldNotBeNil)
				err = datastore.RunMulti(ctx, queries, func(foo *Foo, c datastore.CursorCB) error {
					fooses = append(fooses, foo)
					if len(fooses) == 3 {
						cur, err = c()
						if err != nil {
							return err
						}
						return datastore.Stop
					}
					return nil
				})
				So(err, ShouldBeNil)
				So(fooses, ShouldNotBeNil)
				So(fooses, ShouldResemble, []*Foo{foos[0], foos[1], foos[2]})
				// Apply the cursor to the queries
				queries, err = datastore.ApplyCursors(ctx, queries, cur)
				So(err, ShouldBeNil)
				So(queries, ShouldNotBeNil)
				err = datastore.RunMulti(ctx, queries, func(foo *Foo, c datastore.CursorCB) error {
					fooses = append(fooses, foo)
					return nil
				})
				So(err, ShouldBeNil)
				So(fooses, ShouldNotBeNil)
				So(fooses, ShouldResemble, []*Foo{foos[0], foos[1], foos[2], foos[3]})
			})
			Convey("cb with cursorCB, repeat entities", func() {
				queries := []*datastore.Query{
					datastore.NewQuery("Foo").Eq("multi_vals", "m2"),
					datastore.NewQuery("Foo").Eq("multi_vals", "m3"),
					datastore.NewQuery("Foo").Eq("multi_vals", "m4"),
				}
				var cur datastore.Cursor
				var err error
				var fooses []*Foo
				err = datastore.RunMulti(ctx, queries, func(foo *Foo, c datastore.CursorCB) error {
					fooses = append(fooses, foo)
					if len(fooses) == 2 {
						cur, err = c()
						if err != nil {
							return err
						}
						return datastore.Stop
					}
					return nil
				})
				So(err, ShouldBeNil)
				So(fooses, ShouldNotBeNil)
				So(fooses, ShouldResemble, []*Foo{foos[0], foos[1]})
				// Apply the cursor to the queries
				queries, err = datastore.ApplyCursors(ctx, queries, cur)
				So(err, ShouldBeNil)
				So(queries, ShouldNotBeNil)
				err = datastore.RunMulti(ctx, queries, func(foo *Foo, c datastore.CursorCB) error {
					fooses = append(fooses, foo)
					return nil
				})
				So(err, ShouldBeNil)
				So(fooses, ShouldNotBeNil)
				// RunMulti returns only the unique entities for that run. If there q1 and q2 are in q,
				// which is a slice of query. And if x is a valid response to both. If the callback reads
				// one of the x and then stops and retrieves the cursor. It is possible to repeat the same
				// set of queries with the cursor applied and get x again. This happens because RunMulti
				// doesn't have any knowledge of the previous call and it doesn't see that x has already
				// been returned in a previous run.
				So(fooses, ShouldResemble, []*Foo{foos[0], foos[1], foos[1], foos[2], foos[3]})
			})
		})

		Convey("bad", func() {
			Convey("Queries with more than one kind", func() {
				queries := []*datastore.Query{
					datastore.NewQuery("Foo"),
					datastore.NewQuery("Foo1"),
				}

				err := datastore.RunMulti(ctx, queries, func(foo *Foo, c datastore.CursorCB) error {
					return nil
				})
				So(err, ShouldErrLike, "should query the same kind")
			})

			Convey("Queries with different order", func() {
				queries := []*datastore.Query{
					datastore.NewQuery("Foo").Order("field1"),
					datastore.NewQuery("Foo").Order("-field1"),
				}

				err := datastore.RunMulti(ctx, queries, func(foo *Foo, c datastore.CursorCB) error {
					return nil
				})
				So(err, ShouldErrLike, "should use the same order")
			})
		})

		Convey("context cancelation", func() {
			ctx, cancel := context.WithCancel(ctx)

			queries := []*datastore.Query{
				datastore.NewQuery("Foo").Eq("multi_vals", "m2"),
				datastore.NewQuery("Foo").Eq("multi_vals", "m3"),
			}

			err := datastore.RunMulti(ctx, queries, func(k *datastore.Key) error {
				cancel()
				<-ctx.Done() // make sure it "propagates" everywhere
				return nil
			})
			So(err, ShouldEqual, context.Canceled)
		})

		Convey("callback error", func() {
			customErr := errors.New("boo")

			queries := []*datastore.Query{
				datastore.NewQuery("Foo").Eq("multi_vals", "m2"),
				datastore.NewQuery("Foo").Eq("multi_vals", "m3"),
			}

			var keys []*datastore.Key
			err := datastore.RunMulti(ctx, queries, func(k *datastore.Key) error {
				keys = append(keys, k)
				return customErr
			})
			So(err, ShouldEqual, customErr)
			So(keys, ShouldHaveLength, 1)
		})
	})
}
