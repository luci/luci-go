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

func TestCheckLiveness(t *testing.T) {
	t.Parallel()

	ftt.Run("CheckLiveness", t, func(t *ftt.Test) {
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
		bldr := &model.Builder{
			ID:     "builder",
			Parent: model.BucketKey(ctx, "project", "bucket"),
			Config: &pb.BuilderConfig{
				MaxConcurrentBuilds: 2,
			},
		}

		assert.Loosely(t, datastore.Put(ctx, bld, inf, bs, bldr), should.BeNil)

		t.Run("build not found", func(t *ftt.Test) {
			err := CheckLiveness(ctx, 999, 10)
			assert.Loosely(t, err, should.ErrLike("failed to get build"))
			assert.Loosely(t, sch.Tasks(), should.BeEmpty)
		})

		t.Run("build is ended", func(t *ftt.Test) {
			bld.Proto.Status = pb.Status_SUCCESS
			bs.Status = pb.Status_SUCCESS
			assert.Loosely(t, datastore.Put(ctx, bld), should.BeNil)

			err := CheckLiveness(ctx, 1, 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, sch.Tasks(), should.BeEmpty)
		})

		t.Run("exceeds scheduling timeout", func(t *ftt.Test) {
			now := baseTime.Add(61 * time.Second)
			ctx, _ = testclock.UseTime(ctx, now)

			err := CheckLiveness(ctx, 1, 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, datastore.Get(ctx, bld, bs), should.BeNil)
			assert.Loosely(t, bld.Status, should.Equal(pb.Status_INFRA_FAILURE))
			assert.Loosely(t, bs.Status, should.Equal(pb.Status_INFRA_FAILURE))
			// FinalizeResultDB, ExportBigQuery, NotifyPubSubGoProxy, PopPendingBuilds tasks.
			assert.Loosely(t, sch.Tasks(), should.HaveLength(4))
		})

		t.Run("exceeds heartbeat timeout", func(t *ftt.Test) {
			bld.Proto.Status = pb.Status_STARTED
			bld.Proto.StartTime = timestamppb.New(baseTime)
			bs.Status = pb.Status_STARTED
			steps := &model.BuildSteps{
				ID:    1,
				Build: datastore.KeyForObj(ctx, bld),
			}
			assert.Loosely(t, steps.FromProto([]*pb.Step{
				{Name: "step1", Status: pb.Status_STARTED},
			}), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, bld, steps), should.BeNil)
			now := baseTime.Add(11 * time.Second)
			ctx, _ = testclock.UseTime(ctx, now)

			err := CheckLiveness(ctx, 1, 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, datastore.Get(ctx, bld, bs, steps), should.BeNil)
			assert.Loosely(t, bld.Status, should.Equal(pb.Status_INFRA_FAILURE))
			assert.Loosely(t, bs.Status, should.Equal(pb.Status_INFRA_FAILURE))
			mSteps, err := steps.ToProto(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, mSteps, should.Match([]*pb.Step{
				{
					Name:    "step1",
					Status:  pb.Status_CANCELED,
					EndTime: timestamppb.New(now),
				},
			}))
			assert.Loosely(t, sch.Tasks(), should.HaveLength(4))
		})

		t.Run("exceeds execution timeout", func(t *ftt.Test) {
			now := baseTime.Add(121 * time.Second)
			bld.Proto.Status = pb.Status_STARTED
			bld.Proto.StartTime = timestamppb.New(baseTime)
			// not exceed heartbeat timeout
			bld.Proto.UpdateTime = timestamppb.New(now.Add(-9 * time.Second))
			bs.Status = pb.Status_STARTED

			assert.Loosely(t, datastore.Put(ctx, bld), should.BeNil)
			ctx, _ = testclock.UseTime(ctx, now)

			err := CheckLiveness(ctx, 1, 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, datastore.Get(ctx, bld, bs), should.BeNil)
			assert.Loosely(t, bld.Status, should.Equal(pb.Status_INFRA_FAILURE))
			assert.Loosely(t, bs.Status, should.Equal(pb.Status_INFRA_FAILURE))
			assert.Loosely(t, sch.Tasks(), should.HaveLength(4))
		})

		t.Run("not exceed scheduling timeout", func(t *ftt.Test) {
			now := baseTime.Add(59 * time.Second)
			ctx, _ = testclock.UseTime(ctx, now)

			err := CheckLiveness(ctx, 1, 10)
			assert.Loosely(t, err, should.BeNil)
			// continuation task
			assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
			assert.Loosely(t, sch.Tasks().Payloads()[0], should.Match(&taskdefs.CheckBuildLiveness{
				BuildId:          1,
				HeartbeatTimeout: 10,
			}))
		})

		t.Run("not exceed heartbeat timeout", func(t *ftt.Test) {
			bld.Proto.Status = pb.Status_STARTED
			bld.Proto.StartTime = timestamppb.New(baseTime)
			bs.Status = pb.Status_STARTED
			assert.Loosely(t, datastore.Put(ctx, bld), should.BeNil)
			now := baseTime.Add(9 * time.Second)
			ctx, _ = testclock.UseTime(ctx, now)

			err := CheckLiveness(ctx, 1, 10)
			assert.Loosely(t, err, should.BeNil)
			// continuation task
			assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
			assert.Loosely(t, sch.Tasks().Payloads()[0], should.Match(&taskdefs.CheckBuildLiveness{
				BuildId:          1,
				HeartbeatTimeout: 10,
			}))
			assert.Loosely(t, sch.Tasks()[0].ETA, should.Match(now.Add(10*time.Second)))
		})

		t.Run("not exceed any timout && heartbeat timeout not set", func(t *ftt.Test) {
			bld.Proto.Status = pb.Status_STARTED
			bld.Proto.StartTime = timestamppb.New(baseTime)
			bs.Status = pb.Status_STARTED
			assert.Loosely(t, datastore.Put(ctx, bld), should.BeNil)
			now := baseTime.Add(9 * time.Second)
			ctx, _ = testclock.UseTime(ctx, now)

			err := CheckLiveness(ctx, 1, 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
			assert.Loosely(t, sch.Tasks().Payloads()[0], should.Match(&taskdefs.CheckBuildLiveness{
				BuildId:          1,
				HeartbeatTimeout: 0,
			}))
			assert.Loosely(t, sch.Tasks()[0].ETA, should.Match(baseTime.Add(120*time.Second)))
		})
	})
}
