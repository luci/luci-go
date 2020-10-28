// Copyright 2020 The LUCI Authors.
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

package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

func fv(vs ...interface{}) []interface{} {
	ret := []interface{}{"project/bucket", "project/bucket/builder"}
	return append(ret, vs...)
}

func pbTS(t time.Time) *timestamppb.Timestamp {
	pbts, _ := ptypes.TimestampProto(t)
	return pbts
}

func TestBuildMetrics(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ctx, _ := tsmon.WithDummyInMemory(context.Background())
		store := tsmon.Store(ctx)

		b := &model.Build{
			ID: 1,
			Proto: pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			},
			BucketID:   "project/bucket",
			BuilderID:  "project/bucket/builder",
			Project:    "project",
			Canary:     true,
			CreateTime: testclock.TestRecentTimeUTC,
		}

		Convey("BuildCreated", func() {
			b.Tags = []string{"os:linux"}
			BuildCreated(ctx, b)
			So(store.Get(ctx, BuildCount.Created, time.Time{}, fv("")), ShouldEqual, 1)

			b.Tags = []string{"user_agent:gerrit"}
			BuildCreated(ctx, b)
			So(store.Get(ctx, BuildCount.Created, time.Time{}, fv("gerrit")), ShouldEqual, 1)
		})

		Convey("BuildStarted", func() {
			Convey("build/Started", func() {
				b.Canary = false
				BuildStarted(ctx, b)
				So(store.Get(ctx, BuildCount.Started, time.Time{}, fv(false)), ShouldEqual, 1)

				b.Canary = true
				BuildStarted(ctx, b)
				So(store.Get(ctx, BuildCount.Started, time.Time{}, fv(true)), ShouldEqual, 1)
			})

			Convey("build/scheduling_durations", func() {
				fields := fv("", "", "", true)
				b.Proto.StartTime = pbTS(b.CreateTime.Add(33 * time.Second))
				BuildStarted(ctx, b)
				val := store.Get(ctx, BuildDuration.Scheduling, time.Time{}, fields)
				So(val.(*distribution.Distribution).Sum(), ShouldEqual, 33)
			})
		})

		Convey("BuildCompleted", func() {
			Convey("builds/Completed", func() {
				b.Status = pb.Status_FAILURE
				BuildCompleted(ctx, b)
				fields := fv(model.Failure.String(), model.BuildFailure.String(), "", true)
				So(store.Get(ctx, BuildCount.Completed, time.Time{}, fields), ShouldEqual, 1)

				b.Status = pb.Status_CANCELED
				BuildCompleted(ctx, b)
				fields = fv(model.Canceled.String(), "", model.ExplicitlyCanceled.String(), true)
				So(store.Get(ctx, BuildCount.Completed, time.Time{}, fields), ShouldEqual, 1)

				b.Status = pb.Status_INFRA_FAILURE
				BuildCompleted(ctx, b)
				fields = fv(model.Failure.String(), model.InfraFailure.String(), "", true)
				So(store.Get(ctx, BuildCount.Completed, time.Time{}, fields), ShouldEqual, 1)

				// timeout
				b.Status = pb.Status_INFRA_FAILURE
				b.Proto.StatusDetails = &pb.StatusDetails{Timeout: &pb.StatusDetails_Timeout{}}
				BuildCompleted(ctx, b)
				fields = fv(model.Failure.String(), model.InfraFailure.String(), "", true)
				So(store.Get(ctx, BuildCount.Completed, time.Time{}, fields), ShouldEqual, 1)
			})

			b.Status = pb.Status_SUCCESS
			fields := fv(model.Success.String(), "", "", true)

			Convey("builds/cycle_durations", func() {
				b.Proto.EndTime = pbTS(b.CreateTime.Add(33 * time.Second))
				BuildCompleted(ctx, b)
				val := store.Get(ctx, BuildDuration.Cycle, time.Time{}, fields)
				So(val.(*distribution.Distribution).Sum(), ShouldEqual, 33)
			})

			Convey("builds/run_durations", func() {
				b.Proto.StartTime = pbTS(b.CreateTime.Add(3 * time.Second))
				b.Proto.EndTime = pbTS(b.CreateTime.Add(33 * time.Second))

				BuildCompleted(ctx, b)
				val := store.Get(ctx, BuildDuration.Run, time.Time{}, fields)
				So(val.(*distribution.Distribution).Sum(), ShouldEqual, 30)
			})
		})

		Convey("BuildLeased", func() {
			BuildLeased(ctx, b)
			So(store.Get(ctx, BuildCount.Leased, time.Time{}, fv()), ShouldEqual, 1)
		})

		Convey("BuildLeaseExpired", func() {
			b.Status = pb.Status_FAILURE
			BuildLeaseExpired(ctx, b)
			So(store.Get(ctx, BuildCount.LeaseExpired, time.Time{}, fv("COMPLETED")), ShouldEqual, 1)
		})

		Convey("HeartbeatFailed", func() {
			HeartbeatFailed(ctx)
			So(store.Get(ctx, BuildCount.LeaseExtensionFailed, time.Time{}, nil), ShouldEqual, 1)
		})
	})
}
