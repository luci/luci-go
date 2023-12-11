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

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSpan(t *testing.T) {
	Convey(`With Spanner Test Database`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		Convey(`Reads`, func() {
			reference := time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC)
			shards := []ReclusteringShard{
				NewShard(1).WithShardNumber(1).WithProject("projecta").WithAttemptTimestamp(reference).WithNoProgress().Build(),
				NewShard(2).WithShardNumber(2).WithProject("projecta").WithAttemptTimestamp(reference).WithProgress(250).Build(),
				NewShard(3).WithShardNumber(3).WithProject("projecta").WithAttemptTimestamp(reference).WithProgress(123).Build(),
				NewShard(4).WithShardNumber(4).WithProject("projectb").WithAttemptTimestamp(reference).WithNoProgress().Build(),
				NewShard(5).WithShardNumber(5).WithAttemptTimestamp(reference.Add(-1 * time.Minute)).Build(),
				NewShard(6).WithShardNumber(6).WithAttemptTimestamp(reference.Add(-2 * time.Minute)).Build(),
			}
			err := SetShardsForTesting(ctx, shards)
			So(err, ShouldBeNil)

			Convey(`ReadAll`, func() {
				Convey(`Not Exists`, func() {
					err := SetShardsForTesting(ctx, nil)
					So(err, ShouldBeNil)

					readShards, err := ReadAll(span.Single(ctx))
					So(err, ShouldBeNil)
					So(readShards, ShouldResemble, []ReclusteringShard{})
				})
				Convey(`Exists`, func() {
					readShards, err := ReadAll(span.Single(ctx))
					So(err, ShouldBeNil)
					So(readShards, ShouldResemble, shards)
				})
			})
			Convey(`ReadAllProgresses`, func() {
				Convey(`Not Exists`, func() {
					readProgresses, err := ReadAllProgresses(span.Single(ctx), reference.Add(1*time.Minute))
					So(err, ShouldBeNil)
					So(readProgresses, ShouldResemble, []ReclusteringProgress{})
				})
				Convey(`Exists`, func() {
					readProgresses, err := ReadAllProgresses(span.Single(ctx), reference)
					So(err, ShouldBeNil)
					So(readProgresses, ShouldResemble, []ReclusteringProgress{
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
					})
				})
			})
			Convey(`ReadProgress`, func() {
				Convey(`Not Exists`, func() {
					readProgress, err := ReadProgress(span.Single(ctx), "projecta", reference.Add(1*time.Minute))
					So(err, ShouldBeNil)
					So(readProgress, ShouldResemble, ReclusteringProgress{
						Project:          "projecta",
						AttemptTimestamp: reference.Add(1 * time.Minute),
						ShardCount:       0,
						ShardsReported:   0,
						Progress:         0,
					})
				})
				Convey(`Exists`, func() {
					readProgress, err := ReadProgress(span.Single(ctx), "projecta", reference)
					So(err, ShouldBeNil)
					So(readProgress, ShouldResemble, ReclusteringProgress{
						Project:          "projecta",
						AttemptTimestamp: reference,
						ShardCount:       3,
						ShardsReported:   2,
						Progress:         250 + 123,
					})
				})
			})
		})
		Convey(`UpdateProgress`, func() {
			reference := time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC)
			assertProgress := func(progress int64) {
				shards, err := ReadAll(span.Single(ctx))
				So(err, ShouldBeNil)
				So(shards, ShouldHaveLength, 1)
				So(shards[0].Progress.Valid, ShouldBeTrue)
				So(shards[0].Progress.Int64, ShouldEqual, progress)
			}

			shards := []ReclusteringShard{
				NewShard(1).WithShardNumber(1).WithProject("projecta").WithAttemptTimestamp(reference).WithNoProgress().Build(),
			}
			err := SetShardsForTesting(ctx, shards)
			So(err, ShouldBeNil)

			_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				return UpdateProgress(ctx, 1, reference, 500)
			})
			So(err, ShouldBeNil)
			assertProgress(500)

			_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				return UpdateProgress(ctx, 1, reference, 1000)
			})
			So(err, ShouldBeNil)
			assertProgress(1000)
		})
		Convey(`Create`, func() {
			testCreate := func(s ReclusteringShard) error {
				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					return Create(ctx, s)
				})
				return err
			}
			s := NewShard(100).Build()

			Convey(`Valid`, func() {
				err := testCreate(s)
				So(err, ShouldBeNil)

				shards, err := ReadAll(span.Single(ctx))
				So(err, ShouldBeNil)
				So(shards, ShouldHaveLength, 1)

				// Create does not set the progress, so do not expect it to.
				expectedShard := s
				expectedShard.Progress = spanner.NullInt64{}
				So(shards[0], ShouldResemble, expectedShard)
			})
			Convey(`With invalid Shard Number`, func() {
				s.ShardNumber = 0
				err := testCreate(s)
				So(err, ShouldErrLike, "shard number must be a positive integer")
			})
			Convey(`With invalid Project`, func() {
				Convey(`Unspecified`, func() {
					s.Project = ""
					err := testCreate(s)
					So(err, ShouldErrLike, "project: unspecified")
				})
				Convey(`Invalid`, func() {
					s.Project = "!"
					err := testCreate(s)
					So(err, ShouldErrLike, `project: must match ^[a-z0-9\-]{1,40}$`)
				})
			})
			Convey(`With invalid Attempt Timestamp`, func() {
				s.AttemptTimestamp = time.Time{}
				err := testCreate(s)
				So(err, ShouldErrLike, "attempt timestamp must be valid")
			})
		})
		Convey(`DeleteAll`, func() {
			reference := time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC)
			shards := []ReclusteringShard{
				NewShard(1).WithShardNumber(1).WithProject("projecta").WithAttemptTimestamp(reference).WithProgress(1000).Build(),
				NewShard(2).WithShardNumber(1).WithProject("projecta").WithAttemptTimestamp(reference.Add(-1 * time.Minute)).WithNoProgress().Build(),
				NewShard(3).WithShardNumber(2).WithProject("projectb").WithAttemptTimestamp(reference.Add(-2 * time.Minute)).WithProgress(500).Build(),
				NewShard(4).WithShardNumber(1).WithProject("projecta").WithAttemptTimestamp(reference.Add(-2 * time.Minute)).WithProgress(1000).Build(),
			}
			err := SetShardsForTesting(ctx, shards)
			So(err, ShouldBeNil)

			_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				DeleteAll(ctx)
				return nil
			})
			So(err, ShouldBeNil)

			readShards, err := ReadAll(span.Single(ctx))
			So(err, ShouldBeNil)
			So(readShards, ShouldResemble, []ReclusteringShard{})
		})
	})
}
