// Copyright 2023 The LUCI Authors.
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
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
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

func TestCheckLiveness(t *testing.T) {
	t.Parallel()

	Convey("CheckLiveness", t, func() {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		ctx, _ = metrics.WithCustomMetrics(ctx, &pb.SettingsCfg{})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, sch := tq.TestingContext(ctx, nil)
		baseTime := testclock.TestRecentTimeLocal
		ctx, _ = testclock.UseTime(ctx, baseTime)

		bld := &model.Build{
			Proto: &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status:     pb.Status_SCHEDULED,
				CreateTime: timestamppb.New(baseTime),
				UpdateTime: timestamppb.New(baseTime),
				SchedulingTimeout: &durationpb.Duration{
					Seconds: 60,
				},
				ExecutionTimeout: &durationpb.Duration{
					Seconds: 120,
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

		So(datastore.Put(ctx, bld, inf, bs), ShouldBeNil)

		Convey("build not found", func() {
			err := CheckLiveness(ctx, 999, 10)
			So(err, ShouldErrLike, "failed to get build")
			So(sch.Tasks(), ShouldBeEmpty)
		})

		Convey("build is ended", func() {
			bld.Proto.Status = pb.Status_SUCCESS
			bs.Status = pb.Status_SUCCESS
			So(datastore.Put(ctx, bld), ShouldBeNil)

			err := CheckLiveness(ctx, 1, 10)
			So(err, ShouldBeNil)
			So(sch.Tasks(), ShouldBeEmpty)
		})

		Convey("exceeds scheduling timeout", func() {
			now := baseTime.Add(61 * time.Second)
			ctx, _ = testclock.UseTime(ctx, now)

			err := CheckLiveness(ctx, 1, 10)
			So(err, ShouldBeNil)
			So(datastore.Get(ctx, bld, bs), ShouldBeNil)
			So(bld.Status, ShouldEqual, pb.Status_INFRA_FAILURE)
			So(bs.Status, ShouldEqual, pb.Status_INFRA_FAILURE)
			// FinalizeResultDB, ExportBigQuery, NotifyPubSub, NotifyPubSubGoProxy tasks.
			So(sch.Tasks(), ShouldHaveLength, 4)
		})

		Convey("exceeds heartbeat timeout", func() {
			bld.Proto.Status = pb.Status_STARTED
			bld.Proto.StartTime = timestamppb.New(baseTime)
			bs.Status = pb.Status_STARTED
			steps := &model.BuildSteps{
				ID:    1,
				Build: datastore.KeyForObj(ctx, bld),
			}
			So(steps.FromProto([]*pb.Step{
				{Name: "step1", Status: pb.Status_STARTED},
			}), ShouldBeNil)
			So(datastore.Put(ctx, bld, steps), ShouldBeNil)
			now := baseTime.Add(11 * time.Second)
			ctx, _ = testclock.UseTime(ctx, now)

			err := CheckLiveness(ctx, 1, 10)
			So(err, ShouldBeNil)
			So(datastore.Get(ctx, bld, bs, steps), ShouldBeNil)
			So(bld.Status, ShouldEqual, pb.Status_INFRA_FAILURE)
			So(bs.Status, ShouldEqual, pb.Status_INFRA_FAILURE)
			mSteps, err := steps.ToProto(ctx)
			So(err, ShouldBeNil)
			So(mSteps, ShouldResembleProto, []*pb.Step{
				{
					Name:    "step1",
					Status:  pb.Status_CANCELED,
					EndTime: timestamppb.New(now),
				},
			})
			So(sch.Tasks(), ShouldHaveLength, 4)
		})

		Convey("exceeds execution timeout", func() {
			now := baseTime.Add(121 * time.Second)
			bld.Proto.Status = pb.Status_STARTED
			bld.Proto.StartTime = timestamppb.New(baseTime)
			// not exceed heartbeat timeout
			bld.Proto.UpdateTime = timestamppb.New(now.Add(-9 * time.Second))
			bs.Status = pb.Status_STARTED

			So(datastore.Put(ctx, bld), ShouldBeNil)
			ctx, _ = testclock.UseTime(ctx, now)

			err := CheckLiveness(ctx, 1, 10)
			So(err, ShouldBeNil)
			So(datastore.Get(ctx, bld, bs), ShouldBeNil)
			So(bld.Status, ShouldEqual, pb.Status_INFRA_FAILURE)
			So(bs.Status, ShouldEqual, pb.Status_INFRA_FAILURE)
			So(sch.Tasks(), ShouldHaveLength, 4)
		})

		Convey("not exceed scheduling timeout", func() {
			now := baseTime.Add(59 * time.Second)
			ctx, _ = testclock.UseTime(ctx, now)

			err := CheckLiveness(ctx, 1, 10)
			So(err, ShouldBeNil)
			// continuation task
			So(sch.Tasks(), ShouldHaveLength, 1)
			So(sch.Tasks().Payloads()[0], ShouldResembleProto, &taskdefs.CheckBuildLiveness{
				BuildId:          1,
				HeartbeatTimeout: 10,
			})
		})

		Convey("not exceed heartbeat timeout", func() {
			bld.Proto.Status = pb.Status_STARTED
			bld.Proto.StartTime = timestamppb.New(baseTime)
			bs.Status = pb.Status_STARTED
			So(datastore.Put(ctx, bld), ShouldBeNil)
			now := baseTime.Add(9 * time.Second)
			ctx, _ = testclock.UseTime(ctx, now)

			err := CheckLiveness(ctx, 1, 10)
			So(err, ShouldBeNil)
			// continuation task
			So(sch.Tasks(), ShouldHaveLength, 1)
			So(sch.Tasks().Payloads()[0], ShouldResembleProto, &taskdefs.CheckBuildLiveness{
				BuildId:          1,
				HeartbeatTimeout: 10,
			})
			So(sch.Tasks()[0].ETA, ShouldEqual, now.Add(10*time.Second))
		})

		Convey("not exceed any timout && heartbeat timeout not set", func() {
			bld.Proto.Status = pb.Status_STARTED
			bld.Proto.StartTime = timestamppb.New(baseTime)
			bs.Status = pb.Status_STARTED
			So(datastore.Put(ctx, bld), ShouldBeNil)
			now := baseTime.Add(9 * time.Second)
			ctx, _ = testclock.UseTime(ctx, now)

			err := CheckLiveness(ctx, 1, 0)
			So(err, ShouldBeNil)
			So(sch.Tasks(), ShouldHaveLength, 1)
			So(sch.Tasks().Payloads()[0], ShouldResembleProto, &taskdefs.CheckBuildLiveness{
				BuildId:          1,
				HeartbeatTimeout: 0,
			})
			So(sch.Tasks()[0].ETA, ShouldEqual, baseTime.Add(120*time.Second))
		})
	})
}
