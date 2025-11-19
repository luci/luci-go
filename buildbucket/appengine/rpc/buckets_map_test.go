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

package rpc

import (
	"maps"
	"slices"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestBucketsMap(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(t.Context())

	bucketEntity := func(proj, buck, shadow string) *model.Bucket {
		return &model.Bucket{
			Parent: model.ProjectKey(ctx, proj),
			ID:     buck,
			Proto:  &pb.Bucket{Shadow: shadow},
		}
	}

	scheduleReq := func(proj, buck string) *pb.ScheduleBuildRequest {
		return &pb.ScheduleBuildRequest{
			Builder: &pb.BuilderID{
				Project: proj,
				Bucket:  buck,
				Builder: "ignored-builder",
			},
		}
	}

	assert.NoErr(t, datastore.Put(ctx,
		bucketEntity("proj1", "bucket1", ""),
		bucketEntity("proj1", "bucket2", "shadow1"),
		bucketEntity("proj2", "bucket1", ""),
		bucketEntity("proj2", "bucket2", "shadow2"),
	))

	t.Run("prefetchBuckets", func(t *testing.T) {
		m, err := prefetchBuckets(ctx, []*pb.ScheduleBuildRequest{
			scheduleReq("proj1", "bucket1"),
			scheduleReq("proj1", "bucket1"), // dup
			scheduleReq("proj1", "bucket2"),
			scheduleReq("proj2", "bucket2"),
			scheduleReq("proj1", "unknown"),   // missing bucket
			scheduleReq("unknown", "bucket1"), // missing project
			scheduleReq("", "zzz"),            // broken request
		})
		assert.NoErr(t, err)
		assert.That(t, slices.Sorted(maps.Keys(m.buckets)), should.Match([]string{
			"proj1/bucket1",
			"proj1/bucket2",
			"proj1/unknown",
			"proj2/bucket2",
			"unknown/bucket1",
		}))
		assert.Loosely(t, m.Bucket("proj1/bucket1"), should.NotBeNil)
		assert.Loosely(t, m.Bucket("proj1/unknown"), should.BeNil)
		assert.That(t, func() { m.Bucket("something/else") }, should.PanicLikeString("wasn't prefetched"))

		assert.That(t, m.ShadowBucket("proj1/bucket1"), should.Equal(""))
		assert.That(t, m.ShadowBucket("proj1/bucket2"), should.Equal("shadow1"))
		assert.That(t, m.ShadowBucket("proj1/unknown"), should.Equal(""))
		assert.That(t, func() { m.ShadowBucket("something/else") }, should.PanicLikeString("wasn't prefetched"))
	})

	t.Run("Prefetch", func(t *testing.T) {
		m, err := prefetchBuckets(ctx, nil)
		assert.NoErr(t, err)
		assert.Loosely(t, m.buckets, should.HaveLength(0))

		t.Run("existing", func(t *testing.T) {
			assert.NoErr(t, m.Prefetch(ctx, "proj1/bucket2"))
			assert.That(t, m.ShadowBucket("proj1/bucket2"), should.Equal("shadow1"))

			// Noop.
			assert.NoErr(t, m.Prefetch(ctx, "proj1/bucket2"))
		})

		t.Run("missing", func(t *testing.T) {
			assert.NoErr(t, m.Prefetch(ctx, "unknown/unknown"))
			assert.That(t, m.ShadowBucket("unknown/unknown"), should.Equal(""))

			// Noop.
			assert.NoErr(t, m.Prefetch(ctx, "unknown/unknown"))
		})
	})
}
