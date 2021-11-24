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

	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/target"

	// TODO(crbug/1242998): Remove once safe get becomes datastore default.
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	_ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"

	"go.chromium.org/luci/buildbucket/appengine/model"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReportBuilderMetrics(t *testing.T) {
	t.Parallel()

	Convey("ReportBuilderMetrics", t, func() {
		ctx := memory.Use(context.Background())
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		store := tsmon.Store(ctx)
		service, job, ins := "service", "job", "ins-0"
		prj, bkt := "infra", "ci"
		target := func(builder string) context.Context {
			return target.Set(ctx, &Builder{
				Project:     prj,
				Bucket:      bkt,
				Builder:     builder,
				ServiceName: service,
				JobName:     job,
				InstanceID:  ins,
			})
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
			So(ReportBuilderMetrics(ctx, service, job, ins), ShouldBeNil)
			So(store.Get(target("b1"), V2.BuilderPresence, time.Time{}, nil), ShouldEqual, true)
			So(store.Get(target("b2"), V2.BuilderPresence, time.Time{}, nil), ShouldEqual, true)
			So(store.Get(target("b3"), V2.BuilderPresence, time.Time{}, nil), ShouldBeNil)

			Convey("w/o removed builder", func() {
				So(deleteBuilder("b1"), ShouldBeNil)
				So(ReportBuilderMetrics(ctx, service, job, ins), ShouldBeNil)
				So(store.Get(target("b1"), V2.BuilderPresence, time.Time{}, nil), ShouldBeNil)
				So(store.Get(target("b2"), V2.BuilderPresence, time.Time{}, nil), ShouldEqual, true)
			})

			Convey("w/o inactive builder", func() {
				// Let's pretend that b1 was inactive for 4 weeks, and
				// got unregistered from the BuilderStat.
				So(datastore.Delete(ctx, &model.BuilderStat{ID: prj + ":" + bkt + ":b1"}), ShouldBeNil)
				So(ReportBuilderMetrics(ctx, service, job, ins), ShouldBeNil)
				// b1 should no longer be reported in the presence metric.
				So(store.Get(target("b1"), V2.BuilderPresence, time.Time{}, nil), ShouldBeNil)
				So(store.Get(target("b2"), V2.BuilderPresence, time.Time{}, nil), ShouldEqual, true)
			})
		})
	})
}
