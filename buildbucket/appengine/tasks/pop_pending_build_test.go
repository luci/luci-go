// Copyright 2024 The LUCI Authors.
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

package tasks

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestPopPendingBuildTask(t *testing.T) {
	t.Parallel()

	Convey("PopPendingBuildTask", t, func() {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, sch := tq.TestingContext(ctx, nil)
		now := testclock.TestRecentTimeLocal
		ctx, _ = testclock.UseTime(ctx, now)

		Convey("builder not found", func() {
			err := PopPendingBuildTask(ctx, 1, &pb.BuilderID{
				Project: "project",
				Bucket:  "bucket",
				Builder: "builder",
			})
			So(err, ShouldErrLike, "no such entity")
			So(sch.Tasks(), ShouldBeEmpty)
		})

		Convey("builderQueue does not exist", func() {
			bldr := &model.Builder{
				ID:     "builder",
				Parent: model.BucketKey(ctx, "project", "bucket"),
				Config: &pb.BuilderConfig{
					MaxConcurrentBuilds: 2,
				},
			}
			So(datastore.Put(ctx, bldr), ShouldBeNil)
			err := PopPendingBuildTask(ctx, 1, &pb.BuilderID{
				Project: "project",
				Bucket:  "bucket",
				Builder: "builder",
			})
			So(err, ShouldBeNil)
			// create-backend-task-go
			So(sch.Tasks(), ShouldHaveLength, 0)
			bq := &model.BuilderQueue{
				ID: "project/bucket/builder",
			}
			So(datastore.Get(ctx, bq), ShouldErrLike, "no such entity")
		})

		// A nil buildID means that PopPendingBuildTask
		// was triggered from the builder config ingestion.
		Convey("buildID is nil", func() {
			Convey("max_concurrent_builds was increased", func() {
				bldr := &model.Builder{
					ID:     "builder",
					Parent: model.BucketKey(ctx, "project", "bucket"),
					Config: &pb.BuilderConfig{
						MaxConcurrentBuilds: 3,
					},
				}
				So(datastore.Put(ctx, bldr), ShouldBeNil)
				bq := &model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						1,
					},
					PendingBuilds: []int64{
						2, 3, 4,
					},
				}
				So(datastore.Put(ctx, bldr, bq), ShouldBeNil)
				err := PopPendingBuildTask(ctx, 0, &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				})
				So(err, ShouldBeNil)
				// create-backend-task-go
				So(sch.Tasks(), ShouldHaveLength, 2)
				bq = &model.BuilderQueue{
					ID: "project/bucket/builder",
				}
				So(datastore.Get(ctx, bq), ShouldBeNil)
				So(bq, ShouldResembleProto, &model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						1, 2, 3,
					},
					PendingBuilds: []int64{
						4,
					},
				})
			})

			Convey("max_concurrent_builds was reset", func() {
				bldr := &model.Builder{
					ID:     "builder",
					Parent: model.BucketKey(ctx, "project", "bucket"),
					Config: &pb.BuilderConfig{
						MaxConcurrentBuilds: 0,
					},
				}
				So(datastore.Put(ctx, bldr), ShouldBeNil)
				bq := &model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						1, 2,
					},
					PendingBuilds: []int64{
						3, 4,
					},
				}
				So(datastore.Put(ctx, bldr, bq), ShouldBeNil)
				err := PopPendingBuildTask(ctx, 0, &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				})
				So(err, ShouldBeNil)
				// create-backend-task-go
				So(sch.Tasks(), ShouldHaveLength, 2)
				bq = &model.BuilderQueue{
					ID: "project/bucket/builder",
				}
				So(datastore.Get(ctx, bq), ShouldErrLike, "no such entity")
			})
		})

		Convey("build is not nil", func() {
			Convey("max_concurrent_builds was decreased", func() {
				bldr := &model.Builder{
					ID:     "builder",
					Parent: model.BucketKey(ctx, "project", "bucket"),
					Config: &pb.BuilderConfig{
						MaxConcurrentBuilds: 2,
					},
				}
				So(datastore.Put(ctx, bldr), ShouldBeNil)
				bq := &model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						1, 2, 3,
					},
					PendingBuilds: []int64{
						4,
					},
				}
				So(datastore.Put(ctx, bldr, bq), ShouldBeNil)
				err := PopPendingBuildTask(ctx, 1, &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				})
				So(err, ShouldBeNil)
				// create-backend-task-go
				So(sch.Tasks(), ShouldHaveLength, 0)
				bq = &model.BuilderQueue{
					ID: "project/bucket/builder",
				}
				So(datastore.Get(ctx, bq), ShouldBeNil)
				So(bq, ShouldResembleProto, &model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						3, 2,
					},
					PendingBuilds: []int64{
						4,
					},
				})
			})

			Convey("build was not tracked", func() {
				bldr := &model.Builder{
					ID:     "builder",
					Parent: model.BucketKey(ctx, "project", "bucket"),
					Config: &pb.BuilderConfig{
						MaxConcurrentBuilds: 2,
					},
				}
				So(datastore.Put(ctx, bldr), ShouldBeNil)
				bq := &model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						1, 2,
					},
					PendingBuilds: []int64{
						3, 4,
					},
				}
				So(datastore.Put(ctx, bldr, bq), ShouldBeNil)
				err := PopPendingBuildTask(ctx, 10, &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				})
				So(err, ShouldBeNil)
				// create-backend-task-go
				So(sch.Tasks(), ShouldHaveLength, 0)
				bq = &model.BuilderQueue{
					ID: "project/bucket/builder",
				}
				So(datastore.Get(ctx, bq), ShouldBeNil)
				So(bq, ShouldResembleProto, &model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						1, 2,
					},
					PendingBuilds: []int64{
						3, 4,
					},
				})
			})

			Convey("build was tracked - happy path", func() {
				bldr := &model.Builder{
					ID:     "builder",
					Parent: model.BucketKey(ctx, "project", "bucket"),
					Config: &pb.BuilderConfig{
						MaxConcurrentBuilds: 2,
					},
				}
				So(datastore.Put(ctx, bldr), ShouldBeNil)
				bq := &model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						1, 2,
					},
					PendingBuilds: []int64{
						3, 4,
					},
				}
				So(datastore.Put(ctx, bldr, bq), ShouldBeNil)
				err := PopPendingBuildTask(ctx, 1, &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				})
				So(err, ShouldBeNil)
				// create-backend-task-go
				So(sch.Tasks(), ShouldHaveLength, 1)
				bq = &model.BuilderQueue{
					ID: "project/bucket/builder",
				}
				So(datastore.Get(ctx, bq), ShouldBeNil)
				So(bq, ShouldResembleProto, &model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						2, 3,
					},
					PendingBuilds: []int64{
						4,
					},
				})
			})
		})

	})

}
