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

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"

	// TODO(crbug/1242998): Remove once safe get becomes datastore default.
	_ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

func fv(vs ...interface{}) []interface{} {
	ret := []interface{}{"luci.project.bucket", "builder"}
	return append(ret, vs...)
}

func pbTS(t time.Time) *timestamppb.Timestamp {
	pbts := timestamppb.New(t)
	return pbts
}

func TestBuildEvents(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ctx, _ := tsmon.WithDummyInMemory(context.Background())
		store := tsmon.Store(ctx)

		b := &model.Build{
			ID: 1,
			Proto: &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Canary: true,
			},
			CreateTime: testclock.TestRecentTimeUTC,
		}

		Convey("buildCreated", func() {
			b.Tags = []string{"os:linux"}
			BuildCreated(ctx, b)
			So(store.Get(ctx, V1.BuildCountCreated, time.Time{}, fv("")), ShouldEqual, 1)

			b.Tags = []string{"user_agent:gerrit"}
			BuildCreated(ctx, b)
			So(store.Get(ctx, V1.BuildCountCreated, time.Time{}, fv("gerrit")), ShouldEqual, 1)
		})

		Convey("buildStarted", func() {
			Convey("build/started", func() {
				b.Proto.Canary = false
				BuildStarted(ctx, b)
				So(store.Get(ctx, V1.BuildCountStarted, time.Time{}, fv(false)), ShouldEqual, 1)

				b.Proto.Canary = true
				BuildStarted(ctx, b)
				So(store.Get(ctx, V1.BuildCountStarted, time.Time{}, fv(true)), ShouldEqual, 1)
			})

			Convey("build/scheduling_durations", func() {
				fields := fv("", "", "", true)
				b.Proto.StartTime = pbTS(b.CreateTime.Add(33 * time.Second))
				BuildStarted(ctx, b)
				val := store.Get(ctx, V1.BuildDurationScheduling, time.Time{}, fields)
				So(val.(*distribution.Distribution).Sum(), ShouldEqual, 33)
			})
		})

		Convey("BuildCompleted", func() {
			Convey("builds/completed", func() {
				b.Status = pb.Status_FAILURE
				BuildCompleted(ctx, b)
				fields := fv(model.Failure.String(), model.BuildFailure.String(), "", true)
				So(store.Get(ctx, V1.BuildCountCompleted, time.Time{}, fields), ShouldEqual, 1)

				b.Status = pb.Status_CANCELED
				BuildCompleted(ctx, b)
				fields = fv(model.Canceled.String(), "", model.ExplicitlyCanceled.String(), true)
				So(store.Get(ctx, V1.BuildCountCompleted, time.Time{}, fields), ShouldEqual, 1)

				b.Status = pb.Status_INFRA_FAILURE
				BuildCompleted(ctx, b)
				fields = fv(model.Failure.String(), model.InfraFailure.String(), "", true)
				So(store.Get(ctx, V1.BuildCountCompleted, time.Time{}, fields), ShouldEqual, 1)

				// timeout
				b.Status = pb.Status_INFRA_FAILURE
				b.Proto.StatusDetails = &pb.StatusDetails{Timeout: &pb.StatusDetails_Timeout{}}
				BuildCompleted(ctx, b)
				fields = fv(model.Failure.String(), model.InfraFailure.String(), "", true)
				So(store.Get(ctx, V1.BuildCountCompleted, time.Time{}, fields), ShouldEqual, 1)
			})

			b.Status = pb.Status_SUCCESS
			fields := fv(model.Success.String(), "", "", true)

			Convey("builds/cycle_durations", func() {
				b.Proto.EndTime = pbTS(b.CreateTime.Add(33 * time.Second))
				BuildCompleted(ctx, b)
				val := store.Get(ctx, V1.BuildDurationCycle, time.Time{}, fields)
				So(val.(*distribution.Distribution).Sum(), ShouldEqual, 33)
			})

			Convey("builds/run_durations", func() {
				b.Proto.StartTime = pbTS(b.CreateTime.Add(3 * time.Second))
				b.Proto.EndTime = pbTS(b.CreateTime.Add(33 * time.Second))

				BuildCompleted(ctx, b)
				val := store.Get(ctx, V1.BuildDurationRun, time.Time{}, fields)
				So(val.(*distribution.Distribution).Sum(), ShouldEqual, 30)
			})
		})
	})
}
