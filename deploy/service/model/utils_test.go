// Copyright 2022 The LUCI Authors.
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

package model

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/deploy/api/modelpb"
)

func TestFetchAssets(t *testing.T) {
	t.Parallel()

	ftt.Run("With entities", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		datastore.Put(ctx, []*Asset{
			{ID: "1", Asset: &modelpb.Asset{Id: "1"}},
			{ID: "3", Asset: &modelpb.Asset{Id: "3"}},
			{ID: "5", Asset: &modelpb.Asset{Id: "5"}},
		})

		t.Run("shouldExist = false", func(t *ftt.Test) {
			assets, err := fetchAssets(ctx, []string{"1", "2", "3", "4", "5"}, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, assets, should.HaveLength(5))
			for assetID, ent := range assets {
				assert.Loosely(t, ent.ID, should.Equal(assetID))
				assert.Loosely(t, ent.Asset.Id, should.Equal(assetID))
			}
		})

		t.Run("shouldExist = true", func(t *ftt.Test) {
			_, err := fetchAssets(ctx, []string{"1", "2", "3", "4", "5"}, true)
			assert.Loosely(t, err, should.ErrLike("assets entities unexpectedly missing: 2, 4"))
		})

		t.Run("One missing", func(t *ftt.Test) {
			t.Run("shouldExist = false", func(t *ftt.Test) {
				assets, err := fetchAssets(ctx, []string{"missing"}, false)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, assets, should.HaveLength(1))
				assert.Loosely(t, assets["missing"].ID, should.Equal("missing"))
				assert.Loosely(t, assets["missing"].Asset.Id, should.Equal("missing"))
			})

			t.Run("shouldExist = true", func(t *ftt.Test) {
				_, err := fetchAssets(ctx, []string{"missing"}, true)
				assert.Loosely(t, err, should.ErrLike("assets entities unexpectedly missing: missing"))
			})
		})
	})
}
