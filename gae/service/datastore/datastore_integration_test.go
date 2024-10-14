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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
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

	ftt.Run("RunMulti", t, func(t *ftt.Test) {
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

		assert.Loosely(t, datastore.Put(ctx, foos), should.BeNil)

		t.Run("ok", func(t *ftt.Test) {
			t.Run("default ordering", func(t *ftt.Test) {
				queries := []*datastore.Query{
					datastore.NewQuery("Foo").Gte("__key__", datastore.KeyForObj(ctx, &Foo{ID: 1})),
				}
				var res []*Foo
				err := datastore.RunMulti(ctx, queries, func(foo *Foo) error {
					res = append(res, foo)
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Resemble(foos))
			})

			t.Run("querying key only", func(t *ftt.Test) {
				queries := []*datastore.Query{
					datastore.NewQuery("Foo").Eq("multi_vals", "m2"),
					datastore.NewQuery("Foo").Eq("multi_vals", "m3"),
				}
				var keys []*datastore.Key
				err := datastore.RunMulti(ctx, queries, func(k *datastore.Key) error {
					keys = append(keys, k)
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, keys, should.Resemble([]*datastore.Key{
					datastore.KeyForObj(ctx, foos[0]),
					datastore.KeyForObj(ctx, foos[1]),
					datastore.KeyForObj(ctx, foos[2]),
				}))
			})

			t.Run("overlapped results with non-default orders", func(t *ftt.Test) {
				queries := []*datastore.Query{
					datastore.NewQuery("Foo").Eq("multi_vals", "m2").Order("-single_val", "status"),
					datastore.NewQuery("Foo").Eq("multi_vals", "m3").Order("-single_val", "status"),
				}
				var res []*Foo
				err := datastore.RunMulti(ctx, queries, func(foo *Foo) error {
					res = append(res, foo)
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Resemble([]*Foo{foos[2], foos[1], foos[0]}))
			})

			t.Run("send Stop in cb", func(t *ftt.Test) {
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
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Resemble([]*Foo{foos[0], foos[1]}))
			})

			t.Run("not found", func(t *ftt.Test) {
				queries := []*datastore.Query{
					datastore.NewQuery("Foo").Eq("single_val", "non-existent"),
				}
				var res []*Foo
				err := datastore.RunMulti(ctx, queries, func(foo *Foo) error {
					res = append(res, foo)
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.BeNil)
			})
			t.Run("cb with cursorCB", func(t *ftt.Test) {
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
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, fooses, should.NotBeNil)
				assert.Loosely(t, fooses, should.Resemble([]*Foo{foos[0]}))
				// Apply the cursor to the queries
				queries, err = datastore.ApplyCursors(ctx, queries, cur)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, queries, should.NotBeNil)
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
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, fooses, should.NotBeNil)
				assert.Loosely(t, fooses, should.Resemble([]*Foo{foos[0], foos[1], foos[2]}))
				// Apply the cursor to the queries
				queries, err = datastore.ApplyCursors(ctx, queries, cur)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, queries, should.NotBeNil)
				err = datastore.RunMulti(ctx, queries, func(foo *Foo, c datastore.CursorCB) error {
					fooses = append(fooses, foo)
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, fooses, should.NotBeNil)
				assert.Loosely(t, fooses, should.Resemble([]*Foo{foos[0], foos[1], foos[2], foos[3]}))
			})
			t.Run("cb with cursorCB, repeat entities", func(t *ftt.Test) {
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
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, fooses, should.NotBeNil)
				assert.Loosely(t, fooses, should.Resemble([]*Foo{foos[0], foos[1]}))
				// Apply the cursor to the queries
				queries, err = datastore.ApplyCursors(ctx, queries, cur)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, queries, should.NotBeNil)
				err = datastore.RunMulti(ctx, queries, func(foo *Foo, c datastore.CursorCB) error {
					fooses = append(fooses, foo)
					return nil
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, fooses, should.NotBeNil)
				// RunMulti returns only the unique entities for that run. If there q1 and q2 are in q,
				// which is a slice of query. And if x is a valid response to both. If the callback reads
				// one of the x and then stops and retrieves the cursor. It is possible to repeat the same
				// set of queries with the cursor applied and get x again. This happens because RunMulti
				// doesn't have any knowledge of the previous call and it doesn't see that x has already
				// been returned in a previous run.
				assert.Loosely(t, fooses, should.Resemble([]*Foo{foos[0], foos[1], foos[1], foos[2], foos[3]}))
			})
		})

		t.Run("bad", func(t *ftt.Test) {
			t.Run("Queries with more than one kind", func(t *ftt.Test) {
				queries := []*datastore.Query{
					datastore.NewQuery("Foo"),
					datastore.NewQuery("Foo1"),
				}

				err := datastore.RunMulti(ctx, queries, func(foo *Foo, c datastore.CursorCB) error {
					return nil
				})
				assert.Loosely(t, err, should.ErrLike("should query the same kind"))
			})

			t.Run("Queries with different order", func(t *ftt.Test) {
				queries := []*datastore.Query{
					datastore.NewQuery("Foo").Order("field1"),
					datastore.NewQuery("Foo").Order("-field1"),
				}

				err := datastore.RunMulti(ctx, queries, func(foo *Foo, c datastore.CursorCB) error {
					return nil
				})
				assert.Loosely(t, err, should.ErrLike("should use the same order"))
			})
		})

		t.Run("context cancelation", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.Equal(context.Canceled))
		})

		t.Run("callback error", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.Equal(customErr))
			assert.Loosely(t, keys, should.HaveLength(1))
		})
	})
}
