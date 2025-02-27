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
	"time"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/count"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/cachingtest"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(
		datastore.Nullable[string, datastore.Indexed]{},
	))
}

func TestTaskRequestSerialization(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())

	taskID := "65aba3a3e6b99310"
	key, err := TaskIDToRequestKey(ctx, taskID)
	assert.NoErr(t, err)

	// A task request with few of representative fields.
	req := &TaskRequest{
		Key:     key,
		TxnUUID: "123",
		TaskSlices: []TaskSlice{
			{
				Properties: TaskProperties{
					Idempotent: false,
					Dimensions: TaskDimensions{
						"a": {"b", "c"},
					},
				},
				PropertiesHash:  []byte("zzz"),
				ExpirationSecs:  123,
				WaitForCapacity: false,
			},
		},
		ParentTaskID: datastore.NewIndexedNullable("parent-id"),
		Created:      time.Date(2100, 2, 2, 0, 0, 0, 0, time.UTC),
		ResultDB: ResultDBConfig{
			Enable: true,
		},
	}

	blob, err := serializeTaskRequest(req)
	assert.NoErr(t, err)

	back, err := deserializeTaskRequest(key, blob)
	assert.NoErr(t, err)
	assert.That(t, back, should.Match(req))
}

func TestFetchTaskRequest(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	ctx, counter := count.FilterRDS(ctx)
	ctx = cachingtest.WithGlobalCache(ctx, map[string]caching.BlobCache{
		taskRequestCacheNamespace: cachingtest.NewBlobCache(),
	})

	taskID := "65aba3a3e6b99310"
	key, err := TaskIDToRequestKey(ctx, taskID)
	assert.NoErr(t, err)

	req := &TaskRequest{
		Key:     key,
		TxnUUID: "123",
	}

	// Missing initially.
	_, err = FetchTaskRequest(ctx, key)
	assert.That(t, err, should.Equal(datastore.ErrNoSuchEntity))
	assert.That(t, counter.GetMulti.Total(), should.Equal(int64(1)))

	assert.NoErr(t, datastore.Put(ctx, req))

	// Fetched from the datastore and put into cache.
	fetched, err := FetchTaskRequest(ctx, key)
	assert.NoErr(t, err)
	assert.That(t, fetched, should.Match(req))
	assert.That(t, counter.GetMulti.Total(), should.Equal(int64(2)))

	// Fetched from the cache now.
	fetched, err = FetchTaskRequest(ctx, key)
	assert.NoErr(t, err)
	assert.That(t, fetched, should.Match(req))
	assert.That(t, counter.GetMulti.Total(), should.Equal(int64(2))) // didn't change
}
