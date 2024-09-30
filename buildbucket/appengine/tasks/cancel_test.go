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

package tasks

import (
	"context"
	"sort"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCancelBuild(t *testing.T) {
	Convey("Do", t, func() {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		ctx, _ = metrics.WithCustomMetrics(ctx, &pb.SettingsCfg{})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, sch := tq.TestingContext(ctx, nil)
		now := testclock.TestRecentTimeLocal
		ctx, _ = testclock.UseTime(ctx, now)

		Convey("not found", func() {
			bld, err := Cancel(ctx, 1)
			So(err, ShouldErrLike, "not found")
			So(bld, ShouldBeNil)
			So(sch.Tasks(), ShouldBeEmpty)
		})

		Convey("found", func() {
			bld := &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			}
			inf := &model.BuildInfra{
				ID:    1,
				Build: datastore.MakeKey(ctx, "Build", 1),
				Proto: &pb.BuildInfra{
					Resultdb: &pb.BuildInfra_ResultDB{
						Hostname:   "rdbhost",
						Invocation: "inv",
					},
				},
			}
			bs := &model.BuildStatus{
				Build:  datastore.MakeKey(ctx, "Build", 1),
				Status: pb.Status_SCHEDULED,
			}
			bldr := &model.Builder{
				ID:     "builder",
				Parent: model.BucketKey(ctx, "project", "bucket"),
				Config: &pb.BuilderConfig{
					MaxConcurrentBuilds: 2,
				},
			}
			So(datastore.Put(ctx, bld, inf, bs, bldr), ShouldBeNil)
			bld, err := Cancel(ctx, 1)
			So(err, ShouldBeNil)
			So(bld.Proto, ShouldResembleProto, &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				UpdateTime: timestamppb.New(now),
				EndTime:    timestamppb.New(now),
				Status:     pb.Status_CANCELED,
			})
			// export-bigquery
			// finalize-resultdb-go
			// notify-pubsub
			// notify-pubsub-go-proxy
			// pop-pending-builds
			So(sch.Tasks(), ShouldHaveLength, 5)
			bs = &model.BuildStatus{
				Build: datastore.MakeKey(ctx, "Build", 1),
			}
			So(datastore.Get(ctx, bs), ShouldBeNil)
			So(bs.Status, ShouldEqual, pb.Status_CANCELED)
		})

		Convey("ended", func() {
			So(datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_SUCCESS,
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.BuildStatus{
				Build:  datastore.MakeKey(ctx, "Build", 1),
				Status: pb.Status_SUCCESS,
			}), ShouldBeNil)
			bld, err := Cancel(ctx, 1)
			So(err, ShouldBeNil)
			So(bld.Proto, ShouldResembleProto, &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_SUCCESS,
			})
			So(sch.Tasks(), ShouldBeEmpty)
		})

		Convey("swarming task cancellation", func() {
			So(datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.BuildInfra{
				Build: datastore.MakeKey(ctx, "Build", 1),
				Proto: &pb.BuildInfra{
					Swarming: &pb.BuildInfra_Swarming{
						Hostname: "example.com",
						TaskId:   "id",
					},
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.BuildStatus{
				Build:  datastore.MakeKey(ctx, "Build", 1),
				Status: pb.Status_STARTED,
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.Builder{
				ID:     "builder",
				Parent: model.BucketKey(ctx, "project", "bucket"),
				Config: &pb.BuilderConfig{
					MaxConcurrentBuilds: 2,
				},
			}), ShouldBeNil)
			bld, err := Cancel(ctx, 1)
			So(err, ShouldBeNil)
			So(bld.Proto, ShouldResembleProto, &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				UpdateTime: timestamppb.New(now),
				EndTime:    timestamppb.New(now),
				Status:     pb.Status_CANCELED,
			})
			// cancel-swarming-task-go
			// export-bigquery
			// notify-pubsub
			// notify-pubsub-go-proxy
			// pop-pending-builds
			So(sch.Tasks(), ShouldHaveLength, 5)
			bs := &model.BuildStatus{
				Build: datastore.MakeKey(ctx, "Build", 1),
			}
			So(datastore.Get(ctx, bs), ShouldBeNil)
			So(bs.Status, ShouldEqual, pb.Status_CANCELED)
		})

		Convey("backend task cancellation", func() {
			So(datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.BuildInfra{
				Build: datastore.MakeKey(ctx, "Build", 1),
				Proto: &pb.BuildInfra{
					Backend: &pb.BuildInfra_Backend{
						Hostname: "example.com",
						Task: &pb.Task{
							Id: &pb.TaskID{
								Id:     "123",
								Target: "swarming://chromium-swarmin-dev",
							},
							Status: pb.Status_STARTED,
						},
					},
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.BuildStatus{
				Build:  datastore.MakeKey(ctx, "Build", 1),
				Status: pb.Status_STARTED,
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.Builder{
				ID:     "builder",
				Parent: model.BucketKey(ctx, "project", "bucket"),
				Config: &pb.BuilderConfig{
					MaxConcurrentBuilds: 2,
				},
			}), ShouldBeNil)
			bld, err := Cancel(ctx, 1)
			So(err, ShouldBeNil)
			So(bld.Proto, ShouldResembleProto, &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				UpdateTime: timestamppb.New(now),
				EndTime:    timestamppb.New(now),
				Status:     pb.Status_CANCELED,
			})
			// cancel-backend-task
			// export-bigquery
			// notify-pubsub
			// notify-pubsub-go-proxy
			// pop-pending-builds
			So(sch.Tasks(), ShouldHaveLength, 5)
			bs := &model.BuildStatus{
				Build: datastore.MakeKey(ctx, "Build", 1),
			}
			So(datastore.Get(ctx, bs), ShouldBeNil)
			So(bs.Status, ShouldEqual, pb.Status_CANCELED)
		})

		Convey("resultdb finalization", func() {
			So(datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.BuildInfra{
				Build: datastore.MakeKey(ctx, "Build", 1),
				Proto: &pb.BuildInfra{
					Resultdb: &pb.BuildInfra_ResultDB{
						Hostname:   "example.com",
						Invocation: "id",
					},
				},
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.BuildStatus{
				Build:  datastore.MakeKey(ctx, "Build", 1),
				Status: pb.Status_STARTED,
			}), ShouldBeNil)
			So(datastore.Put(ctx, &model.Builder{
				ID:     "builder",
				Parent: model.BucketKey(ctx, "project", "bucket"),
				Config: &pb.BuilderConfig{
					MaxConcurrentBuilds: 2,
				},
			}), ShouldBeNil)
			bld, err := Cancel(ctx, 1)
			So(err, ShouldBeNil)
			So(bld.Proto, ShouldResembleProto, &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				UpdateTime: timestamppb.New(now),
				EndTime:    timestamppb.New(now),
				Status:     pb.Status_CANCELED,
			})
			So(sch.Tasks(), ShouldHaveLength, 5)
			bs := &model.BuildStatus{
				Build: datastore.MakeKey(ctx, "Build", 1),
			}
			So(datastore.Get(ctx, bs), ShouldBeNil)
			So(bs.Status, ShouldEqual, pb.Status_CANCELED)
		})
	})

	Convey("Start", t, func() {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, sch := tq.TestingContext(ctx, nil)
		now := testclock.TestRecentTimeLocal
		ctx, _ = testclock.UseTime(ctx, now)

		Convey("not found", func() {
			_, err := StartCancel(ctx, 1, "")
			So(err, ShouldErrLike, "not found")
			So(sch.Tasks(), ShouldBeEmpty)
		})

		Convey("found", func() {
			So(datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_STARTED,
				},
			}), ShouldBeNil)
			bld, err := StartCancel(ctx, 1, "summary")
			So(err, ShouldBeNil)
			So(bld.Proto, ShouldResembleProto, &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				UpdateTime:           timestamppb.New(now),
				Status:               pb.Status_STARTED,
				CancelTime:           timestamppb.New(now),
				CanceledBy:           "buildbucket",
				CancellationMarkdown: "summary",
			})
			So(sch.Tasks(), ShouldHaveLength, 1)
		})

		Convey("ended", func() {
			So(datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_SUCCESS,
				},
			}), ShouldBeNil)
			bld, err := StartCancel(ctx, 1, "summary")
			So(err, ShouldBeNil)
			So(bld.Proto, ShouldResembleProto, &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_SUCCESS,
			})
			So(sch.Tasks(), ShouldBeEmpty)
		})

		Convey("in cancel process", func() {
			So(datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status:     pb.Status_STARTED,
					CancelTime: timestamppb.New(now.Add(-time.Minute)),
				},
			}), ShouldBeNil)
			bld, err := StartCancel(ctx, 1, "summary")
			So(err, ShouldBeNil)
			So(bld.Proto, ShouldResembleProto, &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status:     pb.Status_STARTED,
				CancelTime: timestamppb.New(now.Add(-time.Minute)),
			})
			So(sch.Tasks(), ShouldBeEmpty)
		})

		Convey("w/ decedents", func() {
			So(datastore.Put(ctx, &model.Build{
				ID: 1,
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			}), ShouldBeNil)
			// Child can outlive parent.
			So(datastore.Put(ctx, &model.Build{
				ID: 2,
				Proto: &pb.Build{
					Id: 2,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					AncestorIds:      []int64{1},
					CanOutliveParent: true,
				},
			}), ShouldBeNil)
			// Child cannot outlive parent.
			So(datastore.Put(ctx, &model.Build{
				ID: 3,
				Proto: &pb.Build{
					Id: 3,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					AncestorIds:      []int64{1},
					CanOutliveParent: false,
				},
			}), ShouldBeNil)
			// Grandchild.
			So(datastore.Put(ctx, &model.Build{
				ID: 4,
				Proto: &pb.Build{
					Id: 3,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					AncestorIds:      []int64{1, 3},
					CanOutliveParent: false,
				},
			}), ShouldBeNil)
			bld, err := StartCancel(ctx, 1, "summary")
			So(err, ShouldBeNil)
			So(bld.Proto, ShouldResembleProto, &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				UpdateTime:           timestamppb.New(now),
				CancelTime:           timestamppb.New(now),
				CancellationMarkdown: "summary",
				CanceledBy:           "buildbucket",
			})
			ids := make([]int, len(sch.Tasks()))
			for i, task := range sch.Tasks() {
				switch v := task.Payload.(type) {
				case *taskdefs.CancelBuildTask:
					ids[i] = int(v.BuildId)
				default:
					panic("unexpected task payload")
				}
			}
			So(sch.Tasks(), ShouldHaveLength, 3)
			sort.Ints(ids)
			So(ids, ShouldResemble, []int{1, 3, 4})
		})
	})
}
