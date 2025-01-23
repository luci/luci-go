// Copyright 2025 The LUCI Authors.
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

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestFetchNamedCacheSizeHints(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())

	assert.NoErr(t, datastore.Put(ctx, []*NamedCacheStats{
		{
			Key: NamedCacheStatsKey(ctx, "ignored", "c1"),
			OS: []PerOSEntry{
				{Name: "os1", Size: 6666},
			},
		},
		{
			Key: NamedCacheStatsKey(ctx, "pool", "c1"),
			OS: []PerOSEntry{
				{Name: "os1", Size: 1000},
				{Name: "os2", Size: 2000},
				{Name: "os3", Size: 3000},
			},
		},
		{
			Key: NamedCacheStatsKey(ctx, "pool", "c2"),
			OS: []PerOSEntry{
				{Name: "os1", Size: 4000},
				{Name: "os2", Size: 5000},
				{Name: "os3", Size: 6000},
			},
		},
		{
			Key: NamedCacheStatsKey(ctx, "pool", "c3"),
			OS: []PerOSEntry{
				{Name: "xxx", Size: 7000},
				{Name: "yyy", Size: 9000}, // <-- max, will be used as an estimate
				{Name: "zzz", Size: 8000},
			},
		},
	}))

	t.Run("OK", func(t *testing.T) {
		res, err := FetchNamedCacheSizeHints(ctx, "pool", "os1", []string{
			"c1", "c2", "c3", "missing",
		})
		assert.NoErr(t, err)
		assert.That(t, res, should.Match([]int64{
			1000, 4000, 9000, -1,
		}))
	})

	t.Run("One missing", func(t *testing.T) {
		res, err := FetchNamedCacheSizeHints(ctx, "pool", "os1", []string{
			"missing",
		})
		assert.NoErr(t, err)
		assert.That(t, res, should.Match([]int64{
			-1,
		}))
	})
}
