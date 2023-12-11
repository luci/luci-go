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

package shards

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/server/span"

	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/internal/testutil"
)

const testProject = "myproject"

// ShardBuilder provides methods to build a reclustering shard
// for testing.
type ShardBuilder struct {
	shard ReclusteringShard
}

// NewShard starts building a new Shard, for testing.
func NewShard(uniqifier int) *ShardBuilder {
	s := ReclusteringShard{
		ShardNumber:      int64(uniqifier + 1),
		Project:          testProject,
		AttemptTimestamp: time.Date(2010, time.January, 1, 1, 0, 0, uniqifier, time.UTC),
		Progress:         spanner.NullInt64{Valid: true, Int64: int64(uniqifier*457) % 1000},
	}
	return &ShardBuilder{
		shard: s,
	}
}

// WithShardNumber specifies the shard number to use on the shard.
func (b *ShardBuilder) WithShardNumber(shardNumber int64) *ShardBuilder {
	b.shard.ShardNumber = shardNumber
	return b
}

// WithProject specifies the project to use on the shard.
func (b *ShardBuilder) WithProject(project string) *ShardBuilder {
	b.shard.Project = project
	return b
}

// WithAttemptTimestamp specifies the attempt timestamp to use on the shard.
func (b *ShardBuilder) WithAttemptTimestamp(attemptTimestamp time.Time) *ShardBuilder {
	b.shard.AttemptTimestamp = attemptTimestamp
	return b
}

// WithNoProgress sets that the shard has not reported progress.
func (b *ShardBuilder) WithNoProgress() *ShardBuilder {
	b.shard.Progress = spanner.NullInt64{Valid: false}
	return b
}

// WithProgress sets the progress on the shard. This should be a value between
// 0 and 1000.
func (b *ShardBuilder) WithProgress(progress int) *ShardBuilder {
	b.shard.Progress = spanner.NullInt64{Valid: true, Int64: int64(progress)}
	return b
}

func (b *ShardBuilder) Build() ReclusteringShard {
	return b.shard
}

// SetShardsForTesting replaces the set of stored shards to match the given set.
func SetShardsForTesting(ctx context.Context, rs []ReclusteringShard) error {
	testutil.MustApply(ctx,
		spanner.Delete("ReclusteringShards", spanner.AllKeys()))
	// Insert some ReclusteringShards.
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		for _, r := range rs {
			ms := spanutil.InsertMap("ReclusteringShards", map[string]any{
				"ShardNumber":      r.ShardNumber,
				"Project":          r.Project,
				"AttemptTimestamp": r.AttemptTimestamp,
				"Progress":         r.Progress,
			})
			span.BufferWrite(ctx, ms)
		}
		return nil
	})
	return err
}
