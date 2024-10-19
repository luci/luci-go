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
	"context"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/gae/impl/memory"
	ds "go.chromium.org/luci/gae/service/datastore"
)

func TestReadOnly(t *testing.T) {
	t.Parallel()

	ftt.Run("Test datastore filter", t, func(t *ftt.Test) {
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
		assert.Loosely(t, c, should.NotBeNil)

		t.Run("Get works.", func(t *ftt.Test) {
			v := Tester{ID: 1}
			assert.Loosely(t, ds.Get(c, &v), should.BeNil)
			assert.Loosely(t, v.Value, should.Equal("exists 1"))
		})

		t.Run("Count works.", func(t *ftt.Test) {
			q := ds.NewQuery("Tester")
			cnt, err := ds.Count(c, q)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cnt, should.Equal(1))
		})

		t.Run("Put fails with read-only error", func(t *ftt.Test) {
			err := ds.Put(c, &Tester{ID: 1}, &MutableTester{ID: 1, Value: "new"})
			assert.Loosely(t, err, should.ErrLike(errors.MultiError{
				ErrReadOnly,
				nil,
			}))
			// The second put actually worked.
			v := MutableTester{ID: 1}
			assert.Loosely(t, ds.Get(c, &v), should.BeNil)
			assert.Loosely(t, v.Value, should.Equal("new"))
		})

		t.Run("Delete fails with read-only error", func(t *ftt.Test) {
			err := ds.Delete(c, &Tester{ID: 1}, &MutableTester{ID: 1})
			assert.Loosely(t, err, should.ErrLike(errors.MultiError{
				ErrReadOnly,
				nil,
			}))
		})

		t.Run("AllocateIDs fails with read-only error", func(t *ftt.Test) {
			t1 := Tester{}
			t2 := MutableTester{ID: -1}
			err := ds.AllocateIDs(c, &t1, &t2)
			assert.Loosely(t, err, should.ErrLike(errors.MultiError{
				ErrReadOnly,
				nil,
			}))
			assert.Loosely(t, t2.ID, should.BeZero) // allocated
		})

		t.Run("In a transaction", func(t *ftt.Test) {
			t.Run("Get works.", func(t *ftt.Test) {
				v := Tester{ID: 1}

				err := ds.RunInTransaction(c, func(c context.Context) error {
					return ds.Get(c, &v)
				}, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, v.Value, should.Equal("exists 1"))
			})

			t.Run("Count works.", func(t *ftt.Test) {
				// (Need ancestor filter for transaction query)
				q := ds.NewQuery("Tester").Ancestor(ds.KeyForObj(c, &Tester{ID: 1}))

				var cnt int64
				err := ds.RunInTransaction(c, func(c context.Context) (err error) {
					cnt, err = ds.Count(c, q)
					return
				}, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cnt, should.Equal(1))
			})

			t.Run("Put fails with read-only error", func(t *ftt.Test) {
				err := ds.RunInTransaction(c, func(c context.Context) error {
					return ds.Put(c, &Tester{ID: 1})
				}, nil)
				assert.Loosely(t, err, should.Equal(ErrReadOnly))
			})

			t.Run("Delete fails with read-only error", func(t *ftt.Test) {
				err := ds.RunInTransaction(c, func(c context.Context) error {
					return ds.Delete(c, &Tester{ID: 1})
				}, nil)
				assert.Loosely(t, err, should.Equal(ErrReadOnly))
			})

			t.Run("AllocateIDs fails with read-only error", func(t *ftt.Test) {
				err := ds.RunInTransaction(c, func(c context.Context) error {
					return ds.AllocateIDs(c, make([]Tester, 10))
				}, nil)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, errors.SingleError(err), should.Equal(ErrReadOnly))
			})
		})
	})
}
