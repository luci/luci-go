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

// lfv generate field values for legacy metrics
func lfv(vs ...interface{}) []interface{} {
	ret := []interface{}{"luci.project.bucket", "builder"}
	return append(ret, vs...)
}

// fv generate field values for v2 metrics.
func fv(vs ...interface{}) []interface{} {
	return vs
}

func pbTS(t time.Time) *timestamppb.Timestamp {
	pbts := timestamppb.New(t)
	return pbts
}

func TestBuildEvents(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ctx, _ := tsmon.WithDummyInMemory(context.Background())
		ctx = WithServiceInfo(ctx, "svc", "job", "ins")
		ctx = WithBuilder(ctx, "project", "bucket", "builder")
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
			So(store.Get(ctx, V1.BuildCountCreated, time.Time{}, lfv("")), ShouldEqual, 1)
			So(store.Get(ctx, V2.BuildCountCreated, time.Time{}, fv("None")), ShouldEqual, 1)

			// user_agent
			b.Tags = []string{"user_agent:gerrit"}
			BuildCreated(ctx, b)
			So(store.Get(ctx, V1.BuildCountCreated, time.Time{}, lfv("gerrit")), ShouldEqual, 1)

			// experiments
			b.Experiments = []string{"+exp1"}
			BuildCreated(ctx, b)
			So(store.Get(ctx, V2.BuildCountCreated, time.Time{}, fv("exp1")), ShouldEqual, 1)
		})

		Convey("buildStarted", func() {
			Convey("build/started", func() {
				// canary
				b.Proto.Canary = false
				BuildStarted(ctx, b)
				So(store.Get(ctx, V1.BuildCountStarted, time.Time{}, lfv(false)), ShouldEqual, 1)

				b.Proto.Canary = true
				BuildStarted(ctx, b)
				So(store.Get(ctx, V1.BuildCountStarted, time.Time{}, lfv(true)), ShouldEqual, 1)
				So(store.Get(ctx, V2.BuildCountStarted, time.Time{}, fv("None")), ShouldEqual, 2)

				// experiments
				b.Experiments = []string{"+exp1"}
				BuildStarted(ctx, b)
				So(store.Get(ctx, V2.BuildCountStarted, time.Time{}, fv("exp1")), ShouldEqual, 1)
			})

			Convey("build/scheduling_durations", func() {
				fields := lfv("", "", "", true)
				b.Proto.StartTime = pbTS(b.CreateTime.Add(33 * time.Second))
				BuildStarted(ctx, b)
				val := store.Get(ctx, V1.BuildDurationScheduling, time.Time{}, fields)
				So(val.(*distribution.Distribution).Sum(), ShouldEqual, 33)
				val = store.Get(ctx, V2.BuildDurationScheduling, time.Time{}, fv("None"))
				So(val.(*distribution.Distribution).Sum(), ShouldEqual, 33)

				// experiments
				b.Experiments = []string{"+exp1"}
				BuildStarted(ctx, b)
				val = store.Get(ctx, V2.BuildDurationScheduling, time.Time{}, fv("exp1"))
				So(val.(*distribution.Distribution).Sum(), ShouldEqual, 33)
			})
		})

		Convey("BuildCompleted", func() {
			Convey("builds/completed", func() {
				b.Status = pb.Status_FAILURE
				BuildCompleted(ctx, b)
				v1fs := lfv(model.Failure.String(), model.BuildFailure.String(), "", true)
				So(store.Get(ctx, V1.BuildCountCompleted, time.Time{}, v1fs), ShouldEqual, 1)
				v2fs := fv("FAILURE", "None")
				So(store.Get(ctx, V2.BuildCountCompleted, time.Time{}, v2fs), ShouldEqual, 1)

				b.Status = pb.Status_CANCELED
				BuildCompleted(ctx, b)
				v1fs = lfv(model.Canceled.String(), "", model.ExplicitlyCanceled.String(), true)
				So(store.Get(ctx, V1.BuildCountCompleted, time.Time{}, v1fs), ShouldEqual, 1)
				v2fs[0] = "CANCELED"
				So(store.Get(ctx, V2.BuildCountCompleted, time.Time{}, v2fs), ShouldEqual, 1)

				b.Status = pb.Status_INFRA_FAILURE
				BuildCompleted(ctx, b)
				v1fs = lfv(model.Failure.String(), model.InfraFailure.String(), "", true)
				So(store.Get(ctx, V1.BuildCountCompleted, time.Time{}, v1fs), ShouldEqual, 1)
				v2fs[0] = "INFRA_FAILURE"
				So(store.Get(ctx, V2.BuildCountCompleted, time.Time{}, v2fs), ShouldEqual, 1)

				// timeout
				b.Status = pb.Status_INFRA_FAILURE
				b.Proto.StatusDetails = &pb.StatusDetails{Timeout: &pb.StatusDetails_Timeout{}}
				BuildCompleted(ctx, b)
				v1fs = lfv(model.Failure.String(), model.InfraFailure.String(), "", true)
				So(store.Get(ctx, V1.BuildCountCompleted, time.Time{}, v1fs), ShouldEqual, 1)
				v2fs = fv("INFRA_FAILURE", "None")
				So(store.Get(ctx, V2.BuildCountCompleted, time.Time{}, v2fs), ShouldEqual, 2)

				// experiments
				b.Status = pb.Status_SUCCESS
				b.Experiments = []string{"+exp1", "+exp2"}
				v2fs = fv("SUCCESS", "exp1|exp2")
				BuildCompleted(ctx, b)
				So(store.Get(ctx, V2.BuildCountCompleted, time.Time{}, v2fs), ShouldEqual, 1)
			})

			b.Status = pb.Status_SUCCESS
			v1fs := lfv("SUCCESS", "", "", true)
			v2fs := fv("SUCCESS", "None")

			Convey("builds/cycle_durations", func() {
				b.Proto.EndTime = pbTS(b.CreateTime.Add(33 * time.Second))
				BuildCompleted(ctx, b)
				val := store.Get(ctx, V1.BuildDurationCycle, time.Time{}, v1fs)
				So(val.(*distribution.Distribution).Sum(), ShouldEqual, 33)
				val = store.Get(ctx, V2.BuildDurationCycle, time.Time{}, v2fs)
				So(val.(*distribution.Distribution).Sum(), ShouldEqual, 33)

				// experiments
				b.Experiments = []string{"+exp2", "+exp1"}
				BuildCompleted(ctx, b)
				val = store.Get(ctx, V2.BuildDurationCycle, time.Time{}, fv("SUCCESS", "exp1|exp2"))
				So(val.(*distribution.Distribution).Sum(), ShouldEqual, 33)
			})

			Convey("builds/run_durations", func() {
				b.Proto.StartTime = pbTS(b.CreateTime.Add(3 * time.Second))
				b.Proto.EndTime = pbTS(b.CreateTime.Add(33 * time.Second))

				BuildCompleted(ctx, b)
				val := store.Get(ctx, V1.BuildDurationRun, time.Time{}, v1fs)
				So(val.(*distribution.Distribution).Sum(), ShouldEqual, 30)
				val = store.Get(ctx, V2.BuildDurationRun, time.Time{}, v2fs)
				So(val.(*distribution.Distribution).Sum(), ShouldEqual, 30)

				// experiments
				b.Experiments = []string{"+exp2", "+exp1"}
				BuildCompleted(ctx, b)
				val = store.Get(ctx, V2.BuildDurationRun, time.Time{}, fv("SUCCESS", "exp1|exp2"))
				So(val.(*distribution.Distribution).Sum(), ShouldEqual, 30)
			})
		})
	})
}
