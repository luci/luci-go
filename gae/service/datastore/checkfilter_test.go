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

// adapted from github.com/golang/appengine/datastore

package datastore

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/gae/service/info"
)

type fakeRDS struct{ RawInterface }

func (fakeRDS) Constraints() Constraints { return Constraints{} }

func TestCheckFilter(t *testing.T) {
	t.Parallel()

	ftt.Run("Test checkFilter", t, func(t *ftt.Test) {
		// Note that the way we have this context set up, any calls which aren't
		// stopped at the checkFilter will nil-pointer panic. We use this panic
		// behavior to indicate that the checkfilter has allowed a call to pass
		// through to the implementation in the tests below. In a real application
		// the panics observed in the tests below would actually be sucessful calls
		// to the implementation.
		c := SetRaw(info.Set(context.Background(), fakeInfo{}), fakeRDS{})
		rds := Raw(c) // has checkFilter
		assert.Loosely(t, rds, should.NotBeNil)

		t.Run("RunInTransaction", func(t *ftt.Test) {
			assert.Loosely(t, rds.RunInTransaction(nil, nil), should.ErrLike("is nil"))
			hit := false
			assert.Loosely(t, func() {
				assert.Loosely(t, rds.RunInTransaction(func(context.Context) error {
					hit = true
					return nil
				}, nil), should.BeNil)
			}, should.Panic)
			assert.Loosely(t, hit, should.BeFalse)
		})

		t.Run("Run", func(t *ftt.Test) {
			assert.Loosely(t, rds.Run(nil, nil), should.ErrLike("query is nil"))
			fq, err := NewQuery("sup").Finalize()
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, rds.Run(fq, nil), should.ErrLike("callback is nil"))
			hit := false
			assert.Loosely(t, func() {
				assert.Loosely(t, rds.Run(fq, func(*Key, PropertyMap, CursorCB) error {
					hit = true
					return nil
				}), should.BeNil)
			}, should.Panic)
			assert.Loosely(t, hit, should.BeFalse)
		})

		t.Run("GetMulti", func(t *ftt.Test) {
			assert.Loosely(t, rds.GetMulti(nil, nil, func(int, PropertyMap, error) {}), should.BeNil)
			assert.Loosely(t, rds.GetMulti([]*Key{mkKey("", "", "", "")}, nil, nil), should.ErrLike("is nil"))

			// this is in the wrong aid/ns
			keys := []*Key{MkKeyContext("wut", "wrong").MakeKey("Kind", 1)}
			assert.Loosely(t, rds.GetMulti(keys, nil, func(_ int, pm PropertyMap, err error) {
				assert.Loosely(t, pm, should.BeNil)
				assert.Loosely(t, err, should.ErrLike(ErrInvalidKey))
			}), should.BeNil)

			keys[0] = mkKey("Kind", 1)
			hit := false
			assert.Loosely(t, func() {
				assert.Loosely(t, rds.GetMulti(keys, nil, func(_ int, pm PropertyMap, err error) {
					hit = true
				}), should.BeNil)
			}, should.Panic)
			assert.Loosely(t, hit, should.BeFalse)
		})

		t.Run("PutMulti", func(t *ftt.Test) {
			nullCb := func(int, *Key, error) {}
			keys := []*Key{}
			vals := []PropertyMap{{}}
			assert.Loosely(t, rds.PutMulti(keys, vals, nullCb), should.ErrLike("mismatched keys/vals"))
			assert.Loosely(t, rds.PutMulti(nil, nil, nullCb), should.BeNil)

			keys = append(keys, mkKey("aid", "ns", "Wut", 0, "Kind", 0))
			assert.Loosely(t, rds.PutMulti(keys, vals, nil), should.ErrLike("callback is nil"))

			assert.Loosely(t, rds.PutMulti(keys, vals, func(_ int, k *Key, err error) {
				assert.Loosely(t, k, should.BeNil)
				assert.Loosely(t, err, should.ErrLike(ErrInvalidKey))
			}), should.BeNil)

			keys = []*Key{mkKey("s~aid", "ns", "Kind", 0)}
			vals = []PropertyMap{nil}
			assert.Loosely(t, rds.PutMulti(keys, vals, func(_ int, k *Key, err error) {
				assert.Loosely(t, k, should.BeNil)
				assert.Loosely(t, err, should.ErrLike("nil vals entry"))
			}), should.BeNil)

			vals = []PropertyMap{{}}
			hit := false
			assert.Loosely(t, func() {
				assert.Loosely(t, rds.PutMulti(keys, vals, func(_ int, k *Key, err error) {
					hit = true
				}), should.BeNil)
			}, should.Panic)
			assert.Loosely(t, hit, should.BeFalse)
		})

		t.Run("DeleteMulti", func(t *ftt.Test) {
			assert.Loosely(t, rds.DeleteMulti(nil, func(int, error) {}), should.BeNil)

			assert.Loosely(t, rds.DeleteMulti([]*Key{mkKey("", "", "", "")}, nil), should.ErrLike("is nil"))
			assert.Loosely(t, rds.DeleteMulti([]*Key{mkKey("", "", "", "")}, func(_ int, err error) {
				assert.Loosely(t, err, should.ErrLike(ErrInvalidKey))
			}), should.BeNil)

			hit := false
			assert.Loosely(t, func() {
				assert.Loosely(t, rds.DeleteMulti([]*Key{mkKey("s~aid", "ns", "Kind", 1)}, func(int, error) {
					hit = true
				}), should.BeNil)
			}, should.Panic)
			assert.Loosely(t, hit, should.BeFalse)
		})
	})

	ftt.Run("Test context done", t, func(t *ftt.Test) {
		ctx := info.Set(context.Background(), fakeInfo{})
		fds := fakeDatastore{}
		ctx = SetRawFactory(ctx, fds.factory())
		ctx, cancel := context.WithCancel(ctx)
		rds := Raw(ctx)
		fq, err := NewQuery("any").Finalize()
		keys := []*Key{mkKey("FailAll", 1)}
		vals := []PropertyMap{{}}
		cancel()

		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, rds.DeleteMulti(keys, func(int, error) {}), should.Equal(context.Canceled))
		assert.Loosely(t, rds.Run(fq, func(*Key, PropertyMap, CursorCB) error {
			return nil
		}), should.Equal(context.Canceled))
		assert.Loosely(t, rds.RunInTransaction(func(context.Context) error {
			return nil
		}, nil), should.Equal(context.Canceled))
		assert.Loosely(t, rds.GetMulti(keys, nil, func(_ int, pm PropertyMap, err error) {}), should.Equal(context.Canceled))
		assert.Loosely(t, rds.PutMulti(keys, vals, func(_ int, k *Key, err error) {}), should.Equal(context.Canceled))
	})
}
