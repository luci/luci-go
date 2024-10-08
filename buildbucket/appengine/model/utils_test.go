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

package model

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

type testEntity struct {
	ID  int `gae:"$id"`
	Val int
}

func TestGetIgnoreMissing(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)

		for i := 1; i < 5; i++ {
			assert.Loosely(t, datastore.Put(ctx, &testEntity{ID: i, Val: i}), should.BeNil)
		}

		t.Run("No missing", func(t *ftt.Test) {
			exists := testEntity{ID: 1}
			assert.Loosely(t, GetIgnoreMissing(ctx, &exists), should.BeNil)
			assert.Loosely(t, exists.Val, should.Equal(1))
		})

		t.Run("Scalar args", func(t *ftt.Test) {
			exists1 := testEntity{ID: 1}
			exists2 := testEntity{ID: 2}
			missing := testEntity{ID: 1000}

			t.Run("One", func(t *ftt.Test) {
				assert.Loosely(t, GetIgnoreMissing(ctx, &exists1), should.BeNil)
				assert.Loosely(t, exists1.Val, should.Equal(1))

				assert.Loosely(t, GetIgnoreMissing(ctx, &missing), should.BeNil)
				assert.Loosely(t, missing.Val, should.BeZero)
			})

			t.Run("Many", func(t *ftt.Test) {
				assert.Loosely(t, GetIgnoreMissing(ctx, &exists1, &missing, &exists2), should.BeNil)
				assert.Loosely(t, exists1.Val, should.Equal(1))
				assert.Loosely(t, missing.Val, should.BeZero)
				assert.Loosely(t, exists2.Val, should.Equal(2))
			})
		})

		t.Run("Vector args", func(t *ftt.Test) {
			t.Run("One", func(t *ftt.Test) {
				ents := []testEntity{{ID: 1}, {ID: 1000}, {ID: 2}}

				assert.Loosely(t, GetIgnoreMissing(ctx, ents), should.BeNil)
				assert.Loosely(t, ents[0].Val, should.Equal(1))
				assert.Loosely(t, ents[1].Val, should.BeZero)
				assert.Loosely(t, ents[2].Val, should.Equal(2))
			})

			t.Run("Many", func(t *ftt.Test) {
				ents1 := []testEntity{{ID: 1}, {ID: 1000}, {ID: 2}}
				ents2 := []testEntity{{ID: 3}, {ID: 1001}, {ID: 4}}

				assert.Loosely(t, GetIgnoreMissing(ctx, ents1, ents2), should.BeNil)
				assert.Loosely(t, ents1[0].Val, should.Equal(1))
				assert.Loosely(t, ents1[1].Val, should.BeZero)
				assert.Loosely(t, ents1[2].Val, should.Equal(2))
				assert.Loosely(t, ents2[0].Val, should.Equal(3))
				assert.Loosely(t, ents2[1].Val, should.BeZero)
				assert.Loosely(t, ents2[2].Val, should.Equal(4))
			})
		})

		t.Run("Mixed args", func(t *ftt.Test) {
			exists := testEntity{ID: 1}
			missing := testEntity{ID: 1000}
			ents := []testEntity{{ID: 3}, {ID: 1001}, {ID: 4}}

			assert.Loosely(t, GetIgnoreMissing(ctx, &exists, &missing, ents), should.BeNil)
			assert.Loosely(t, exists.Val, should.Equal(1))
			assert.Loosely(t, missing.Val, should.BeZero)
			assert.Loosely(t, ents[0].Val, should.Equal(3))
			assert.Loosely(t, ents[1].Val, should.BeZero)
			assert.Loosely(t, ents[2].Val, should.Equal(4))
		})
	})
}
