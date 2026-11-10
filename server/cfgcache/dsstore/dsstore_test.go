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
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	cfgcachepb "go.chromium.org/luci/server/cfgcache/proto"
)

func TestStore(t *testing.T) {
	t.Parallel()

	pid := "svc-pipeline"
	cs := config.Set("services/my-app")
	msg := &durationpb.Duration{Seconds: 42}
	m := &cfgcachepb.Meta{TransformVersion: 1, InputHash: "abc123", TransformedSize: int64(proto.Size(msg))}
	wantMeta := &cfgcachepb.Meta{TransformVersion: 1, TransformedSize: 2, InputHash: "abc123"}

	t.Run("Put, LoadMeta, and Get round-trip and no-op rewrite", func(t *testing.T) {
		t.Parallel()
		ctx := memory.Use(t.Context())
		c := Store[*durationpb.Duration]{}

		assert.NoErr(t, c.Put(ctx, pid, cs, m, msg))
		datastore.GetTestable(ctx).CatchupIndexes()

		// Re-putting identical cfgcache.Meta is a no-op.
		assert.NoErr(t, c.Put(ctx, pid, cs, m, msg))

		hashes, err := c.LoadMeta(ctx, pid)
		assert.NoErr(t, err)
		assert.That(t, hashes, should.Match(map[config.Set]*cfgcachepb.Meta{cs: wantMeta}))

		for loaded, err := range c.Get(ctx, pid, hashes) {
			assert.NoErr(t, err)
			assert.That(t, loaded.ConfigSet, should.Equal(cs))
			assert.That(t, loaded.Meta, should.Match(wantMeta))
			assert.That(t, loaded.Config.GetSeconds(), should.Equal(int64(42)))
		}
	})

	t.Run("LoadMeta isolates by pipelineID", func(t *testing.T) {
		t.Parallel()
		ctx := memory.Use(t.Context())
		c := Store[*durationpb.Duration]{}

		assert.NoErr(t, c.Put(ctx, "pipe-a", "projects/p1", m, msg))
		assert.NoErr(t, c.Put(ctx, "pipe-a", "projects/p2", m, msg))
		assert.NoErr(t, c.Put(ctx, "pipe-a-other", "projects/p1", m, msg))
		datastore.GetTestable(ctx).CatchupIndexes()

		got, err := c.LoadMeta(ctx, "pipe-a")
		assert.NoErr(t, err)
		assert.That(t, got, should.Match(map[config.Set]*cfgcachepb.Meta{
			"projects/p1": wantMeta,
			"projects/p2": wantMeta,
		}))
	})

	t.Run("Delete removes config sets and empty Delete is a no-op", func(t *testing.T) {
		t.Parallel()
		ctx := memory.Use(t.Context())
		c := Store[*durationpb.Duration]{}

		assert.NoErr(t, c.Put(ctx, pid, cs, m, msg))
		assert.NoErr(t, c.Delete(ctx, pid, []config.Set{cs}))
		datastore.GetTestable(ctx).CatchupIndexes()

		hashes, err := c.LoadMeta(ctx, pid)
		assert.NoErr(t, err)
		assert.That(t, hashes, should.Match(map[config.Set]*cfgcachepb.Meta{}))

		assert.NoErr(t, c.Delete(ctx, pid, nil))
	})
}
