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

package featureBreaker

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

func TestBrokenFeatures(t *testing.T) {
	t.Parallel()

	e := errors.New("default err")

	ftt.Run("BrokenFeatures", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())

		t.Run("Can break ds", func(t *ftt.Test) {
			t.Run("without a default", func(t *ftt.Test) {
				c, bf := FilterRDS(c, nil)
				vals := []ds.PropertyMap{{
					"$key": ds.MkPropertyNI(ds.NewKey(c, "Wut", "", 1, nil)),
				}}

				t.Run("by specifying an error", func(t *ftt.Test) {
					bf.BreakFeatures(e, "GetMulti", "PutMulti")
					assert.Loosely(t, ds.Get(c, vals), should.Equal(e))

					t.Run("and you can unbreak them as well", func(t *ftt.Test) {
						bf.UnbreakFeatures("GetMulti")

						assert.Loosely(t, errors.SingleError(ds.Get(c, vals)), should.Equal(ds.ErrNoSuchEntity))

						t.Run("no broken features at all is a shortcut", func(t *ftt.Test) {
							bf.UnbreakFeatures("PutMulti")
							assert.Loosely(t, errors.SingleError(ds.Get(c, vals)), should.Equal(ds.ErrNoSuchEntity))
						})
					})
				})

				t.Run("Not specifying an error gets you a generic error", func(t *ftt.Test) {
					bf.BreakFeatures(nil, "GetMulti")
					assert.Loosely(t, ds.Get(c, vals).Error(), should.ContainSubstring(`feature "GetMulti" is broken`))
				})

				t.Run("Callback work and receives correct context", func(t *ftt.Test) {
					errToReturn := errors.New("err from callback")
					key := "some key"

					bf.BreakFeaturesWithCallback(func(c context.Context, feature string) error {
						assert.Loosely(t, c.Value(&key), should.Equal("some value"))
						return errToReturn
					}, "GetMulti")

					ctx := context.WithValue(c, &key, "some value")
					assert.Loosely(t, ds.Get(ctx, vals), should.Equal(errToReturn))

					errToReturn = nil
					assert.Loosely(t, errors.SingleError(ds.Get(ctx, vals)), should.Equal(ds.ErrNoSuchEntity))
				})

				t.Run("Transaction hooks work", func(t *ftt.Test) {
					// A sequence of errors emulating a bunch of failing RPCs that cause
					// the transaction body to be retried once.
					errs := []struct {
						name string
						err  error
					}{
						{"BeginTransaction", nil},
						{"CommitTransaction", ds.ErrConcurrentTransaction},
						{"BeginTransaction", ds.ErrConcurrentTransaction},
						{"BeginTransaction", nil},
						{"CommitTransaction", nil},
					}

					bf.BreakFeaturesWithCallback(func(c context.Context, feature string) error {
						assert.Loosely(t, len(errs), should.BeGreaterThan(0))
						assert.Loosely(t, errs[0].name, should.Equal(feature))
						err := errs[0].err
						errs = errs[1:]
						return err
					}, "BeginTransaction", "CommitTransaction")

					calls := 0
					assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
						calls++
						return nil
					}, nil), should.BeNil)

					assert.Loosely(t, calls, should.Equal(2))
					assert.Loosely(t, errs, should.BeEmpty)
				})
			})

			t.Run("with a default", func(t *ftt.Test) {
				c, bf := FilterRDS(c, e)
				vals := []ds.PropertyMap{{
					"$key": ds.MkPropertyNI(ds.NewKey(c, "Wut", "", 1, nil)),
				}}
				bf.BreakFeatures(nil, "GetMulti")
				assert.Loosely(t, ds.Get(c, vals), should.Equal(e))
			})
		})
	})
}
