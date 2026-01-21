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

package model

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type testMemberShard struct {
	Blob []byte
}

func (s *testMemberShard) GetBlob() []byte {
	return s.Blob
}

func TestMembersSharding(t *testing.T) {
	t.Parallel()

	ftt.Run("returns nil when sharding isn't necessary", t, func(t *ftt.Test) {
		ctx := context.Background()

		members := []string{"user:a@example.com", "user:b@example.com"}
		blobs := shardMembers(ctx, members, 38)
		assert.Loosely(t, blobs, should.BeNil)
	})

	ftt.Run("unsharding sharded members works", t, func(t *ftt.Test) {
		ctx := context.Background()

		members := []string{"user:a@example.com", "user:b@example.com"}
		blobs := shardMembers(ctx, members, 5)
		assert.Loosely(t, blobs, should.NotBeEmpty)

		shards := make([]*testMemberShard, len(blobs))
		for i, blob := range blobs {
			shards[i] = &testMemberShard{
				Blob: blob,
			}
		}
		actual, err := unshardMembers(ctx, shards)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Match(members))
	})

	ftt.Run("returns nil when not given shards", t, func(t *ftt.Test) {
		ctx := context.Background()

		actual, err := unshardMembers(ctx, []*testMemberShard{})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.BeNil)
	})

	ftt.Run("returns error when shard blobs aren't from sharding", t, func(t *ftt.Test) {
		ctx := context.Background()

		shards := []*testMemberShard{
			{Blob: []byte("this")},
			{Blob: []byte("is")},
			{Blob: []byte("a")},
			{Blob: []byte("test")},
		}
		_, err := unshardMembers(ctx, shards)
		assert.Loosely(t, err, should.NotBeNil)
	})
}
