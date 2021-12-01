// Copyright 2021 The LUCI Authors.
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
	"go.chromium.org/luci/common/tsmon/target"

	// TODO(crbug/1242998): Remove once safe get becomes datastore default.
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	_ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReportBuilderMetrics(t *testing.T) {
	t.Parallel()

	Convey("ReportBuilderMetrics", t, func() {
		ctx, clock := testclock.UseTime(
			WithServiceInfo(memory.Use(context.Background()), "svc", "job", "ins"),
			testclock.TestTimeUTC.Truncate(time.Millisecond),
		)
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		store := tsmon.Store(ctx)
		prj, bkt := "infra", "ci"
		task := &target.Task{
			ServiceName: "service",
			JobName:     "job",
			HostName:    "ins-0",
			TaskNum:     0,
		}
		store.SetDefaultTarget(task)
		target := func(builder string) context.Context {
			return WithBuilder(ctx, prj, bkt, builder)
		}

		createBuilder := func(builder string) error {
			return datastore.Put(
				ctx,
				&model.Bucket{Parent: model.ProjectKey(ctx, prj), ID: bkt},
				&model.Builder{Parent: model.BucketKey(ctx, prj, bkt), ID: builder},
				&model.BuilderStat{ID: prj + ":" + bkt + ":" + builder},
			)
		}
		deleteBuilder := func(builder string) error {
			return datastore.Delete(
				ctx,
				&model.Bucket{Parent: model.ProjectKey(ctx, prj), ID: bkt},
				&model.Builder{Parent: model.BucketKey(ctx, prj, bkt), ID: builder},
				&model.BuilderStat{ID: prj + ":" + bkt + ":" + builder},
			)
		}

		Convey("report v2.BuilderPresence", func() {
			So(createBuilder("b1"), ShouldBeNil)
			So(createBuilder("b2"), ShouldBeNil)
			So(ReportBuilderMetrics(ctx), ShouldBeNil)
			So(store.Get(target("b1"), V2.BuilderPresence, time.Time{}, nil), ShouldEqual, true)
			So(store.Get(target("b2"), V2.BuilderPresence, time.Time{}, nil), ShouldEqual, true)
			So(store.Get(target("b3"), V2.BuilderPresence, time.Time{}, nil), ShouldBeNil)

			Convey("w/o removed builder", func() {
				So(deleteBuilder("b1"), ShouldBeNil)
				So(ReportBuilderMetrics(ctx), ShouldBeNil)
				So(store.Get(target("b1"), V2.BuilderPresence, time.Time{}, nil), ShouldBeNil)
				So(store.Get(target("b2"), V2.BuilderPresence, time.Time{}, nil), ShouldEqual, true)
			})

			Convey("w/o inactive builder", func() {
				// Let's pretend that b1 was inactive for 4 weeks, and
				// got unregistered from the BuilderStat.
				So(datastore.Delete(ctx, &model.BuilderStat{ID: prj + ":" + bkt + ":b1"}), ShouldBeNil)
				So(ReportBuilderMetrics(ctx), ShouldBeNil)
				// b1 should no longer be reported in the presence metric.
				So(store.Get(target("b1"), V2.BuilderPresence, time.Time{}, nil), ShouldBeNil)
				So(store.Get(target("b2"), V2.BuilderPresence, time.Time{}, nil), ShouldEqual, true)
			})
		})

		Convey("report MaxAgeScheduled", func() {
			So(createBuilder("b1"), ShouldBeNil)
			builderID := &pb.BuilderID{
				Project: prj,
				Bucket:  bkt,
				Builder: "b1",
			}
			builds := []*model.Build{
				{
					Proto:        &pb.Build{Id: 1, Builder: builderID, Status: pb.Status_SCHEDULED},
					Experimental: false,
					Tags:         []string{"builder:b1"},
				},
				{
					Proto:        &pb.Build{Id: 2, Builder: builderID, Status: pb.Status_SCHEDULED},
					Experimental: false,
					Tags:         []string{"builder:b1"},
				},
			}
			builds[0].NeverLeased = true
			builds[1].NeverLeased = false
			now := clock.Now()

			Convey("never_leased_age >= leased_age", func() {
				builds[0].Proto.CreateTime = timestamppb.New(now.Add(-2 * time.Hour))
				builds[1].Proto.CreateTime = timestamppb.New(now.Add(-1 * time.Hour))
				So(datastore.Put(ctx, builds[0], builds[1]), ShouldBeNil)
				So(ReportBuilderMetrics(ctx), ShouldBeNil)

				age := (2 * time.Hour).Seconds()

				Convey("v1", func() {
					// the ages should be the same.
					fields := []interface{}{bkt, "b1", true}
					So(store.Get(ctx, V1.MaxAgeScheduled, time.Time{}, fields), ShouldEqual, age)
					fields = []interface{}{bkt, "b1", false}
					So(store.Get(ctx, V1.MaxAgeScheduled, time.Time{}, fields), ShouldEqual, age)
				})

				Convey("v2", func() {
					So(store.Get(target("b1"), V2.MaxAgeScheduled, time.Time{}, nil), ShouldEqual, age)
				})
			})

			Convey("v1: never_leased_age < leased_age", func() {
				builds[0].Proto.CreateTime = timestamppb.New(now.Add(-1 * time.Hour))
				builds[1].Proto.CreateTime = timestamppb.New(now.Add(-2 * time.Hour))
				So(datastore.Put(ctx, builds[0], builds[1]), ShouldBeNil)
				So(ReportBuilderMetrics(ctx), ShouldBeNil)

				age := time.Hour.Seconds()

				Convey("v1", func() {
					// the ages should be different.
					fields := []interface{}{bkt, "b1", true}
					So(store.Get(ctx, V1.MaxAgeScheduled, time.Time{}, fields), ShouldEqual, age)
					fields = []interface{}{bkt, "b1", false}
					So(store.Get(ctx, V1.MaxAgeScheduled, time.Time{}, fields), ShouldEqual, 2*age)
				})

				Convey("v2", func() {
					So(store.Get(target("b1"), V2.MaxAgeScheduled, time.Time{}, nil), ShouldEqual, 2*age)
				})
			})

			Convey("w/ swarming config in bucket", func() {
				So(datastore.Put(ctx, &model.Bucket{
					Parent: model.ProjectKey(ctx, prj), ID: bkt,
					Proto: &pb.Bucket{Swarming: &pb.Swarming{}},
				}), ShouldBeNil)
				So(datastore.Put(ctx, builds[0], builds[1]), ShouldBeNil)
				So(ReportBuilderMetrics(ctx), ShouldBeNil)

				Convey("v1", func() {
					// Data should have been reported with "luci.$project.$bucket"
					fields := []interface{}{bkt, "b1", true}
					So(store.Get(ctx, V1.MaxAgeScheduled, time.Time{}, fields), ShouldBeNil)
					fields = []interface{}{"luci." + prj + "." + bkt, "b1", true}
					So(store.Get(ctx, V1.MaxAgeScheduled, time.Time{}, fields), ShouldNotBeNil)
				})

				Convey("v2", func() {
					// V2 doesn't care. It always reports the bucket name as it is.
					So(store.Get(target("b1"), V2.MaxAgeScheduled, time.Time{}, nil), ShouldNotBeNil)
				})
			})
		})
	})
}
