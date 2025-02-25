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
	"testing"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testutil"
)

func TestSpan(t *testing.T) {
	ftt.Run(`With Spanner Test Database`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		t.Run(`Reads`, func(t *ftt.Test) {
			reference := time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC)
			shards := []ReclusteringShard{
				NewShard(1).WithShardNumber(1).WithProject("projecta").WithAttemptTimestamp(reference).WithNoProgress().Build(),
				NewShard(2).WithShardNumber(2).WithProject("projecta").WithAttemptTimestamp(reference).WithProgress(250).Build(),
				NewShard(3).WithShardNumber(3).WithProject("projecta").WithAttemptTimestamp(reference).WithProgress(123).Build(),
				NewShard(4).WithShardNumber(4).WithProject("projectb").WithAttemptTimestamp(reference).WithNoProgress().Build(),
				NewShard(5).WithShardNumber(5).WithAttemptTimestamp(reference.Add(-1 * time.Minute)).Build(),
				NewShard(6).WithShardNumber(6).WithAttemptTimestamp(reference.Add(-2 * time.Minute)).Build(),
			}
			err := SetShardsForTesting(ctx, t, shards)
			assert.Loosely(t, err, should.BeNil)

			t.Run(`ReadAll`, func(t *ftt.Test) {
				t.Run(`Not Exists`, func(t *ftt.Test) {
					err := SetShardsForTesting(ctx, t, nil)
					assert.Loosely(t, err, should.BeNil)

					readShards, err := ReadAll(span.Single(ctx))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, readShards, should.Match([]ReclusteringShard{}))
				})
				t.Run(`Exists`, func(t *ftt.Test) {
					readShards, err := ReadAll(span.Single(ctx))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, readShards, should.Match(shards))
				})
			})
			t.Run(`ReadAllProgresses`, func(t *ftt.Test) {
				t.Run(`Not Exists`, func(t *ftt.Test) {
					readProgresses, err := ReadAllProgresses(span.Single(ctx), reference.Add(1*time.Minute))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, readProgresses, should.Match([]ReclusteringProgress{}))
				})
				t.Run(`Exists`, func(t *ftt.Test) {
					readProgresses, err := ReadAllProgresses(span.Single(ctx), reference)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, readProgresses, should.Match([]ReclusteringProgress{
						{
							Project:          "projecta",
							AttemptTimestamp: reference,
							ShardCount:       3,
							ShardsReported:   2,
							Progress:         250 + 123,
						},
						{
							Project:          "projectb",
							AttemptTimestamp: reference,
							ShardCount:       1,
							ShardsReported:   0,
							Progress:         0,
						},
					}))
				})
			})
			t.Run(`ReadProgress`, func(t *ftt.Test) {
				t.Run(`Not Exists`, func(t *ftt.Test) {
					readProgress, err := ReadProgress(span.Single(ctx), "projecta", reference.Add(1*time.Minute))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, readProgress, should.Match(ReclusteringProgress{
						Project:          "projecta",
						AttemptTimestamp: reference.Add(1 * time.Minute),
						ShardCount:       0,
						ShardsReported:   0,
						Progress:         0,
					}))
				})
				t.Run(`Exists`, func(t *ftt.Test) {
					readProgress, err := ReadProgress(span.Single(ctx), "projecta", reference)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, readProgress, should.Match(ReclusteringProgress{
						Project:          "projecta",
						AttemptTimestamp: reference,
						ShardCount:       3,
						ShardsReported:   2,
						Progress:         250 + 123,
					}))
				})
			})
		})
		t.Run(`UpdateProgress`, func(t *ftt.Test) {
			reference := time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC)
			assertProgress := func(progress int64) {
				shards, err := ReadAll(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, shards, should.HaveLength(1))
				assert.Loosely(t, shards[0].Progress.Valid, should.BeTrue)
				assert.Loosely(t, shards[0].Progress.Int64, should.Equal(progress))
			}

			shards := []ReclusteringShard{
				NewShard(1).WithShardNumber(1).WithProject("projecta").WithAttemptTimestamp(reference).WithNoProgress().Build(),
			}
			err := SetShardsForTesting(ctx, t, shards)
			assert.Loosely(t, err, should.BeNil)

			_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				return UpdateProgress(ctx, 1, reference, 500)
			})
			assert.Loosely(t, err, should.BeNil)
			assertProgress(500)

			_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				return UpdateProgress(ctx, 1, reference, 1000)
			})
			assert.Loosely(t, err, should.BeNil)
			assertProgress(1000)
		})
		t.Run(`Create`, func(t *ftt.Test) {
			testCreate := func(s ReclusteringShard) error {
				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					return Create(ctx, s)
				})
				return err
			}
			s := NewShard(100).Build()

			t.Run(`Valid`, func(t *ftt.Test) {
				err := testCreate(s)
				assert.Loosely(t, err, should.BeNil)

				shards, err := ReadAll(span.Single(ctx))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, shards, should.HaveLength(1))

				// Create does not set the progress, so do not expect it to.
				expectedShard := s
				expectedShard.Progress = spanner.NullInt64{}
				assert.Loosely(t, shards[0], should.Match(expectedShard))
			})
			t.Run(`With invalid Shard Number`, func(t *ftt.Test) {
				s.ShardNumber = 0
				err := testCreate(s)
				assert.Loosely(t, err, should.ErrLike("shard number must be a positive integer"))
			})
			t.Run(`With invalid Project`, func(t *ftt.Test) {
				t.Run(`Unspecified`, func(t *ftt.Test) {
					s.Project = ""
					err := testCreate(s)
					assert.Loosely(t, err, should.ErrLike("project: unspecified"))
				})
				t.Run(`Invalid`, func(t *ftt.Test) {
					s.Project = "!"
					err := testCreate(s)
					assert.Loosely(t, err, should.ErrLike(`project: must match ^[a-z0-9\-]{1,40}$`))
				})
			})
			t.Run(`With invalid Attempt Timestamp`, func(t *ftt.Test) {
				s.AttemptTimestamp = time.Time{}
				err := testCreate(s)
				assert.Loosely(t, err, should.ErrLike("attempt timestamp must be valid"))
			})
		})
		t.Run(`DeleteAll`, func(t *ftt.Test) {
			reference := time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC)
			shards := []ReclusteringShard{
				NewShard(1).WithShardNumber(1).WithProject("projecta").WithAttemptTimestamp(reference).WithProgress(1000).Build(),
				NewShard(2).WithShardNumber(1).WithProject("projecta").WithAttemptTimestamp(reference.Add(-1 * time.Minute)).WithNoProgress().Build(),
				NewShard(3).WithShardNumber(2).WithProject("projectb").WithAttemptTimestamp(reference.Add(-2 * time.Minute)).WithProgress(500).Build(),
				NewShard(4).WithShardNumber(1).WithProject("projecta").WithAttemptTimestamp(reference.Add(-2 * time.Minute)).WithProgress(1000).Build(),
			}
			err := SetShardsForTesting(ctx, t, shards)
			assert.Loosely(t, err, should.BeNil)

			_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				DeleteAll(ctx)
				return nil
			})
			assert.Loosely(t, err, should.BeNil)

			readShards, err := ReadAll(span.Single(ctx))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, readShards, should.Match([]ReclusteringShard{}))
		})
	})
}
