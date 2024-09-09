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

func TestPushPendingBuildTask(t *testing.T) {
	t.Parallel()

	Convey("PushPendingBuildTask", t, func() {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, sch := tq.TestingContext(ctx, nil)
		now := testclock.TestRecentTimeLocal
		ctx, _ = testclock.UseTime(ctx, now)

		Convey("builder not found", func() {
			err := PushPendingBuildTask(ctx, 1, &pb.BuilderID{
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
			err := PushPendingBuildTask(ctx, 1, &pb.BuilderID{
				Project: "project",
				Bucket:  "bucket",
				Builder: "builder",
			})
			So(err, ShouldBeNil)
			// create-backend-task-go
			So(sch.Tasks(), ShouldHaveLength, 1)
			bq := &model.BuilderQueue{
				ID: "project/bucket/builder",
			}
			So(datastore.Get(ctx, bq), ShouldBeNil)
			So(bq, ShouldResembleProto, &model.BuilderQueue{
				ID: "project/bucket/builder",
				TriggeredBuilds: []int64{
					1,
				},
			})
		})

		Convey("max_concurrent_builds is 0", func() {
			bldr := &model.Builder{
				ID:     "builder",
				Parent: model.BucketKey(ctx, "project", "bucket"),
				Config: &pb.BuilderConfig{
					MaxConcurrentBuilds: 0,
				},
			}
			So(datastore.Put(ctx, bldr), ShouldBeNil)
			err := PushPendingBuildTask(ctx, 1, &pb.BuilderID{
				Project: "project",
				Bucket:  "bucket",
				Builder: "builder",
			})
			So(err, ShouldBeNil)
			// create-backend-task-go
			So(sch.Tasks(), ShouldHaveLength, 1)
			bq := &model.BuilderQueue{
				ID: "project/bucket/builder",
			}
			So(datastore.Get(ctx, bq), ShouldErrLike, "no such entity")
		})

		Convey("pending_builds is empty", func() {
			Convey("triggered_builds is less than max_concurrent_builds", func() {
				bldr := &model.Builder{
					ID:     "builder",
					Parent: model.BucketKey(ctx, "project", "bucket"),
					Config: &pb.BuilderConfig{
						MaxConcurrentBuilds: 2,
					},
				}
				bq := &model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						1,
					},
				}
				So(datastore.Put(ctx, bldr, bq), ShouldBeNil)
				err := PushPendingBuildTask(ctx, 2, &pb.BuilderID{
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
						1, 2,
					},
				})
			})

			Convey("triggered_builds is greater or equal to max_concurrent_builds", func() {
				bldr := &model.Builder{
					ID:     "builder",
					Parent: model.BucketKey(ctx, "project", "bucket"),
					Config: &pb.BuilderConfig{
						MaxConcurrentBuilds: 1,
					},
				}
				bq := &model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						1, 2,
					},
				}
				So(datastore.Put(ctx, bldr, bq), ShouldBeNil)
				err := PushPendingBuildTask(ctx, 3, &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				})
				So(err, ShouldBeNil)
				// No backend task triggered.
				So(sch.Tasks(), ShouldBeEmpty)
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
						3,
					},
				})
			})
		})

		Convey("pending_builds is non-empty", func() {
			Convey("triggered_builds is less than max_concurrent_builds", func() {
				bldr := &model.Builder{
					ID:     "builder",
					Parent: model.BucketKey(ctx, "project", "bucket"),
					Config: &pb.BuilderConfig{
						MaxConcurrentBuilds: 3,
					},
				}
				bq := &model.BuilderQueue{
					ID: "project/bucket/builder",
					PendingBuilds: []int64{
						1,
					},
					TriggeredBuilds: []int64{
						2, 3,
					},
				}
				So(datastore.Put(ctx, bldr, bq), ShouldBeNil)
				err := PushPendingBuildTask(ctx, 4, &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				})
				So(err, ShouldBeNil)
				// No backend task triggered.
				So(sch.Tasks(), ShouldBeEmpty)
				bq = &model.BuilderQueue{
					ID: "project/bucket/builder",
				}
				So(datastore.Get(ctx, bq), ShouldBeNil)
				So(bq, ShouldResembleProto, &model.BuilderQueue{
					ID: "project/bucket/builder",
					PendingBuilds: []int64{
						1, 4,
					},
					TriggeredBuilds: []int64{
						2, 3,
					},
				})
			})

			Convey("triggered_builds is greater or equal to max_concurrent_builds", func() {
				bldr := &model.Builder{
					ID:     "builder",
					Parent: model.BucketKey(ctx, "project", "bucket"),
					Config: &pb.BuilderConfig{
						MaxConcurrentBuilds: 2,
					},
				}
				bq := &model.BuilderQueue{
					ID: "project/bucket/builder",
					PendingBuilds: []int64{
						3,
					},
					TriggeredBuilds: []int64{
						1, 2,
					},
				}
				So(datastore.Put(ctx, bldr, bq), ShouldBeNil)
				err := PushPendingBuildTask(ctx, 4, &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				})
				So(err, ShouldBeNil)
				// No backend task triggered.
				So(sch.Tasks(), ShouldBeEmpty)
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
		})
	})
}
