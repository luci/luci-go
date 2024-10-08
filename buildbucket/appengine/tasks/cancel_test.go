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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestCancelBuild(t *testing.T) {
	ftt.Run("Do", t, func(t *ftt.Test) {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		ctx, _ = metrics.WithCustomMetrics(ctx, &pb.SettingsCfg{})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, sch := tq.TestingContext(ctx, nil)
		now := testclock.TestRecentTimeLocal
		ctx, _ = testclock.UseTime(ctx, now)

		t.Run("not found", func(t *ftt.Test) {
			bld, err := Cancel(ctx, 1)
			assert.Loosely(t, err, should.ErrLike("not found"))
			assert.Loosely(t, bld, should.BeNil)
			assert.Loosely(t, sch.Tasks(), should.BeEmpty)
		})

		t.Run("found", func(t *ftt.Test) {
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
			assert.Loosely(t, datastore.Put(ctx, bld, inf, bs, bldr), should.BeNil)
			bld, err := Cancel(ctx, 1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, bld.Proto, should.Resemble(&pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				UpdateTime: timestamppb.New(now),
				EndTime:    timestamppb.New(now),
				Status:     pb.Status_CANCELED,
			}))
			// export-bigquery
			// finalize-resultdb-go
			// notify-pubsub
			// notify-pubsub-go-proxy
			// pop-pending-builds
			assert.Loosely(t, sch.Tasks(), should.HaveLength(5))
			bs = &model.BuildStatus{
				Build: datastore.MakeKey(ctx, "Build", 1),
			}
			assert.Loosely(t, datastore.Get(ctx, bs), should.BeNil)
			assert.Loosely(t, bs.Status, should.Equal(pb.Status_CANCELED))
		})

		t.Run("ended", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_SUCCESS,
				},
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.BuildStatus{
				Build:  datastore.MakeKey(ctx, "Build", 1),
				Status: pb.Status_SUCCESS,
			}), should.BeNil)
			bld, err := Cancel(ctx, 1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, bld.Proto, should.Resemble(&pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_SUCCESS,
			}))
			assert.Loosely(t, sch.Tasks(), should.BeEmpty)
		})

		t.Run("swarming task cancellation", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.BuildInfra{
				Build: datastore.MakeKey(ctx, "Build", 1),
				Proto: &pb.BuildInfra{
					Swarming: &pb.BuildInfra_Swarming{
						Hostname: "example.com",
						TaskId:   "id",
					},
				},
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.BuildStatus{
				Build:  datastore.MakeKey(ctx, "Build", 1),
				Status: pb.Status_STARTED,
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.Builder{
				ID:     "builder",
				Parent: model.BucketKey(ctx, "project", "bucket"),
				Config: &pb.BuilderConfig{
					MaxConcurrentBuilds: 2,
				},
			}), should.BeNil)
			bld, err := Cancel(ctx, 1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, bld.Proto, should.Resemble(&pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				UpdateTime: timestamppb.New(now),
				EndTime:    timestamppb.New(now),
				Status:     pb.Status_CANCELED,
			}))
			// cancel-swarming-task-go
			// export-bigquery
			// notify-pubsub
			// notify-pubsub-go-proxy
			// pop-pending-builds
			assert.Loosely(t, sch.Tasks(), should.HaveLength(5))
			bs := &model.BuildStatus{
				Build: datastore.MakeKey(ctx, "Build", 1),
			}
			assert.Loosely(t, datastore.Get(ctx, bs), should.BeNil)
			assert.Loosely(t, bs.Status, should.Equal(pb.Status_CANCELED))
		})

		t.Run("backend task cancellation", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.BuildInfra{
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
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.BuildStatus{
				Build:  datastore.MakeKey(ctx, "Build", 1),
				Status: pb.Status_STARTED,
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.Builder{
				ID:     "builder",
				Parent: model.BucketKey(ctx, "project", "bucket"),
				Config: &pb.BuilderConfig{
					MaxConcurrentBuilds: 2,
				},
			}), should.BeNil)
			bld, err := Cancel(ctx, 1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, bld.Proto, should.Resemble(&pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				UpdateTime: timestamppb.New(now),
				EndTime:    timestamppb.New(now),
				Status:     pb.Status_CANCELED,
			}))
			// cancel-backend-task
			// export-bigquery
			// notify-pubsub
			// notify-pubsub-go-proxy
			// pop-pending-builds
			assert.Loosely(t, sch.Tasks(), should.HaveLength(5))
			bs := &model.BuildStatus{
				Build: datastore.MakeKey(ctx, "Build", 1),
			}
			assert.Loosely(t, datastore.Get(ctx, bs), should.BeNil)
			assert.Loosely(t, bs.Status, should.Equal(pb.Status_CANCELED))
		})

		t.Run("resultdb finalization", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.BuildInfra{
				Build: datastore.MakeKey(ctx, "Build", 1),
				Proto: &pb.BuildInfra{
					Resultdb: &pb.BuildInfra_ResultDB{
						Hostname:   "example.com",
						Invocation: "id",
					},
				},
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.BuildStatus{
				Build:  datastore.MakeKey(ctx, "Build", 1),
				Status: pb.Status_STARTED,
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.Builder{
				ID:     "builder",
				Parent: model.BucketKey(ctx, "project", "bucket"),
				Config: &pb.BuilderConfig{
					MaxConcurrentBuilds: 2,
				},
			}), should.BeNil)
			bld, err := Cancel(ctx, 1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, bld.Proto, should.Resemble(&pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				UpdateTime: timestamppb.New(now),
				EndTime:    timestamppb.New(now),
				Status:     pb.Status_CANCELED,
			}))
			assert.Loosely(t, sch.Tasks(), should.HaveLength(5))
			bs := &model.BuildStatus{
				Build: datastore.MakeKey(ctx, "Build", 1),
			}
			assert.Loosely(t, datastore.Get(ctx, bs), should.BeNil)
			assert.Loosely(t, bs.Status, should.Equal(pb.Status_CANCELED))
		})
	})

	ftt.Run("Start", t, func(t *ftt.Test) {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, sch := tq.TestingContext(ctx, nil)
		now := testclock.TestRecentTimeLocal
		ctx, _ = testclock.UseTime(ctx, now)

		t.Run("not found", func(t *ftt.Test) {
			_, err := StartCancel(ctx, 1, "")
			assert.Loosely(t, err, should.ErrLike("not found"))
			assert.Loosely(t, sch.Tasks(), should.BeEmpty)
		})

		t.Run("found", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_STARTED,
				},
			}), should.BeNil)
			bld, err := StartCancel(ctx, 1, "summary")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, bld.Proto, should.Resemble(&pb.Build{
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
			}))
			assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
		})

		t.Run("ended", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_SUCCESS,
				},
			}), should.BeNil)
			bld, err := StartCancel(ctx, 1, "summary")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, bld.Proto, should.Resemble(&pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_SUCCESS,
			}))
			assert.Loosely(t, sch.Tasks(), should.BeEmpty)
		})

		t.Run("in cancel process", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
			}), should.BeNil)
			bld, err := StartCancel(ctx, 1, "summary")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, bld.Proto, should.Resemble(&pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status:     pb.Status_STARTED,
				CancelTime: timestamppb.New(now.Add(-time.Minute)),
			}))
			assert.Loosely(t, sch.Tasks(), should.BeEmpty)
		})

		t.Run("w/ decedents", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
				ID: 1,
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
			}), should.BeNil)
			// Child can outlive parent.
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
			}), should.BeNil)
			// Child cannot outlive parent.
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
			}), should.BeNil)
			// Grandchild.
			assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
			}), should.BeNil)
			bld, err := StartCancel(ctx, 1, "summary")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, bld.Proto, should.Resemble(&pb.Build{
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
			}))
			ids := make([]int, len(sch.Tasks()))
			for i, task := range sch.Tasks() {
				switch v := task.Payload.(type) {
				case *taskdefs.CancelBuildTask:
					ids[i] = int(v.BuildId)
				default:
					panic("unexpected task payload")
				}
			}
			assert.Loosely(t, sch.Tasks(), should.HaveLength(3))
			sort.Ints(ids)
			assert.Loosely(t, ids, should.Resemble([]int{1, 3, 4}))
		})
	})
}
