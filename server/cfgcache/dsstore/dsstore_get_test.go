// Copyright 2026 The LUCI Authors.
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

package dsstore

import (
	"context"
	"testing"

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	cfgcachecommon "go.chromium.org/luci/server/cfgcache/common"
	cfgcachepb "go.chromium.org/luci/server/cfgcache/proto"
)

// getPerKeyErrFilter intercepts GetMulti to simulate per-entity errors.
type getPerKeyErrFilter struct {
	datastore.RawInterface
	perKeyErr error
}

func (f *getPerKeyErrFilter) GetMulti(keys []*datastore.Key, meta datastore.MultiMetaGetter, cb datastore.GetMultiCB) error {
	return f.RawInterface.GetMulti(keys, meta, func(idx int, val datastore.PropertyMap, err error) {
		cb(idx, nil, f.perKeyErr)
	})
}

func TestDatastoreStoreGet(t *testing.T) {
	t.Parallel()

	pid := "pipe"
	cs1 := config.Set("projects/p1")
	cs2 := config.Set("projects/p2")
	csMissing := config.Set("projects/missing")
	m := &cfgcachepb.Meta{TransformVersion: 1, TransformedSize: 2, InputHash: "h1"}

	t.Run("skips missing config sets without error", func(t *testing.T) {
		t.Parallel()
		ctx := memory.Use(t.Context())
		c := Store[*durationpb.Duration]{}

		assert.NoErr(t, c.Put(ctx, pid, cs1, m, &durationpb.Duration{Seconds: 10}))
		assert.NoErr(t, c.Put(ctx, pid, cs2, m, &durationpb.Duration{Seconds: 20}))

		gotSets := map[config.Set]bool{}
		for sc, err := range c.Get(ctx, pid, map[config.Set]*cfgcachepb.Meta{cs1: m, cs2: m, csMissing: m}) {
			assert.NoErr(t, err)
			gotSets[sc.ConfigSet] = true
		}
		assert.That(t, gotSets, should.Match(map[config.Set]bool{cs1: true, cs2: true}))
	})

	t.Run("supports early break from iterator", func(t *testing.T) {
		t.Parallel()
		ctx := memory.Use(t.Context())
		c := Store[*durationpb.Duration]{}

		assert.NoErr(t, c.Put(ctx, pid, cs1, m, &durationpb.Duration{Seconds: 10}))
		assert.NoErr(t, c.Put(ctx, pid, cs2, m, &durationpb.Duration{Seconds: 20}))

		count := 0
		for _, err := range c.Get(ctx, pid, map[config.Set]*cfgcachepb.Meta{cs1: m, cs2: m}) {
			assert.NoErr(t, err)
			count++
			break
		}
		assert.That(t, count, should.Equal(1))
	})

	t.Run("yields per-entity MultiError with StoredConfig", func(t *testing.T) {
		t.Parallel()
		ctx := memory.Use(t.Context())
		c := Store[*durationpb.Duration]{}

		assert.NoErr(t, c.Put(ctx, pid, cs1, m, &durationpb.Duration{Seconds: 10}))

		errFilterCtx := datastore.AddRawFilters(ctx, func(ic context.Context, rds datastore.RawInterface) datastore.RawInterface {
			return &getPerKeyErrFilter{RawInterface: rds, perKeyErr: errors.New("per-key boom")}
		})
		var errSC *cfgcachecommon.StoredConfig[*durationpb.Duration]
		var itemErr error
		for sc, err := range c.Get(errFilterCtx, pid, map[config.Set]*cfgcachepb.Meta{cs1: m}) {
			if err != nil {
				errSC = sc
				itemErr = err
			}
		}
		assert.ErrIsLike(t, itemErr, "per-key boom")
		assert.Loosely(t, errSC, should.NotBeNil)
		assert.That(t, errSC.ConfigSet, should.Equal(cs1))
	})

	t.Run("yields general GetMulti error", func(t *testing.T) {
		t.Parallel()
		ctx, fb := featureBreaker.FilterRDS(memory.Use(t.Context()), nil)
		c := Store[*durationpb.Duration]{}

		fb.BreakFeatures(errors.New("get multi boom"), "GetMulti")
		var generalErr error
		for sc, err := range c.Get(ctx, pid, map[config.Set]*cfgcachepb.Meta{cs1: m}) {
			assert.That(t, sc, should.Equal[*cfgcachecommon.StoredConfig[*durationpb.Duration]](nil))
			generalErr = err
		}
		assert.ErrIsLike(t, generalErr, "get multi boom")
	})
}
