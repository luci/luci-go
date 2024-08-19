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
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/model"
	modeldefs "go.chromium.org/luci/buildbucket/appengine/model/defs"
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
		ctx, _ = WithCustomMetrics(ctx, &pb.SettingsCfg{})
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		store := tsmon.Store(ctx)
		prj, bkt := "infra", "ci"
		task := &target.Task{
			ServiceName: "svc",
			JobName:     "job",
			HostName:    "ins",
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
					Proto: &pb.Build{Id: 1, Builder: builderID, Status: pb.Status_SCHEDULED},
					Tags:  []string{"builder:b1"},
				},
				{
					Proto: &pb.Build{Id: 2, Builder: builderID, Status: pb.Status_SCHEDULED},
					Tags:  []string{"builder:b1"},
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
					fields := []any{bkt, "b1", true}
					So(store.Get(ctx, V1.MaxAgeScheduled, time.Time{}, fields), ShouldEqual, age)
					fields = []any{bkt, "b1", false}
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
					fields := []any{bkt, "b1", true}
					So(store.Get(ctx, V1.MaxAgeScheduled, time.Time{}, fields), ShouldEqual, age)
					fields = []any{bkt, "b1", false}
					So(store.Get(ctx, V1.MaxAgeScheduled, time.Time{}, fields), ShouldEqual, 2*age)
				})

				Convey("v2", func() {
					So(store.Get(target("b1"), V2.MaxAgeScheduled, time.Time{}, nil), ShouldEqual, 2*age)
				})

				Convey("custom", func() {
					base := pb.CustomMetricBase_CUSTOM_METRIC_BASE_MAX_AGE_SCHEDULED
					name := "chrome/infra/custom/builds/max_age"

					globalCfg := &pb.SettingsCfg{
						CustomMetrics: []*pb.CustomMetric{
							{
								Name: name,
								Class: &pb.CustomMetric_MetricBase{
									MetricBase: base,
								},
							},
						},
					}
					ctx = WithBuilder(ctx, prj, bkt, "b1")
					ctx, _ = WithCustomMetrics(ctx, globalCfg)

					Convey("builder no custom metric", func() {
						So(ReportBuilderMetrics(ctx), ShouldBeNil)
						res, err := GetCustomMetricsData(ctx, base, name, time.Time{}, nil)
						So(err, ShouldBeNil)
						So(res, ShouldBeNil)
					})

					bldr := &model.Builder{
						Parent: model.BucketKey(ctx, prj, bkt),
						ID:     "b1",
						Config: &pb.BuilderConfig{
							CustomMetricDefinitions: []*pb.CustomMetricDefinition{
								{
									Name: name,
								},
							},
						},
					}
					So(datastore.Put(ctx, bldr), ShouldBeNil)
					bldrMetrics := &model.CustomBuilderMetrics{
						Key:        model.CustomBuilderMetricsKey(ctx),
						LastUpdate: testclock.TestRecentTimeUTC,
						Metrics: &modeldefs.CustomBuilderMetrics{
							Metrics: []*modeldefs.CustomBuilderMetric{
								{
									Name: name,
									Builders: []*pb.BuilderID{
										{
											Project: prj,
											Bucket:  bkt,
											Builder: "b1",
										},
									},
								},
							},
						},
					}
					So(datastore.Put(ctx, bldrMetrics), ShouldBeNil)
					Convey("no build populates the metric", func() {
						So(ReportBuilderMetrics(ctx), ShouldBeNil)
						res, err := GetCustomMetricsData(ctx, base, name, time.Time{}, nil)
						So(err, ShouldBeNil)
						So(res, ShouldBeNil)
					})

					Convey("work", func() {
						builds[0].CustomBuilderMaxAgeMetrics = []string{name}
						So(datastore.Put(ctx, builds[0]), ShouldBeNil)
						So(ReportBuilderMetrics(ctx), ShouldBeNil)

						res, err := GetCustomMetricsData(ctx, base, name, time.Time{}, nil)
						So(err, ShouldBeNil)
						So(res, ShouldEqual, age)
					})
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
					fields := []any{bkt, "b1", true}
					So(store.Get(ctx, V1.MaxAgeScheduled, time.Time{}, fields), ShouldBeNil)
					fields = []any{"luci." + prj + "." + bkt, "b1", true}
					So(store.Get(ctx, V1.MaxAgeScheduled, time.Time{}, fields), ShouldNotBeNil)
				})

				Convey("v2", func() {
					// V2 doesn't care. It always reports the bucket name as it is.
					So(store.Get(target("b1"), V2.MaxAgeScheduled, time.Time{}, nil), ShouldNotBeNil)
				})
			})
		})

		Convey("report ConsecutiveFailures", func() {
			So(createBuilder("b1"), ShouldBeNil)
			builderID := &pb.BuilderID{
				Project: prj,
				Bucket:  bkt,
				Builder: "b1",
			}
			B := func(status pb.Status, changedAt time.Time) *model.Build {
				return &model.Build{
					Proto: &pb.Build{
						Builder: builderID, Status: status,
						UpdateTime: timestamppb.New(changedAt)},
					Tags: []string{"builder:b1"},
				}
			}
			count := func(s string) any {
				return store.Get(target("b1"), V2.ConsecutiveFailureCount, time.Time{}, []any{s})
			}
			t := clock.Now()

			Convey("w/o success", func() {
				builds := []*model.Build{
					B(pb.Status_CANCELED, t.Add(-4*time.Minute)),
					B(pb.Status_FAILURE, t.Add(-3*time.Minute)),
					B(pb.Status_INFRA_FAILURE, t.Add(-2*time.Minute)),
					B(pb.Status_CANCELED, t.Add(-1*time.Minute)),
				}
				So(datastore.Put(ctx, builds), ShouldBeNil)
				So(ReportBuilderMetrics(ctx), ShouldBeNil)
				So(count("FAILURE"), ShouldEqual, 1)
				So(count("INFRA_FAILURE"), ShouldEqual, 1)
				So(count("CANCELED"), ShouldEqual, 2)
			})

			Convey("w/ success only", func() {
				builds := []*model.Build{
					B(pb.Status_SUCCESS, t.Add(-3*time.Minute)),
					B(pb.Status_SUCCESS, t.Add(-2*time.Minute)),
					B(pb.Status_SUCCESS, t.Add(-1*time.Minute)),
				}
				So(datastore.Put(ctx, builds), ShouldBeNil)
				So(ReportBuilderMetrics(ctx), ShouldBeNil)
				// The count for each status should still be reported w/o 0.
				So(count("FAILURE"), ShouldEqual, 0)
				So(count("INFRA_FAILURE"), ShouldEqual, 0)
				So(count("CANCELED"), ShouldEqual, 0)
			})

			Convey("w/ a series of failures after success", func() {
				builds := []*model.Build{
					B(pb.Status_CANCELED, t.Add(-6*time.Minute)),
					B(pb.Status_SUCCESS, t.Add(-5*time.Minute)),
					B(pb.Status_FAILURE, t.Add(-4*time.Minute)),
					B(pb.Status_FAILURE, t.Add(-3*time.Minute)),
					B(pb.Status_INFRA_FAILURE, t.Add(-2*time.Minute)),
					B(pb.Status_CANCELED, t.Add(-1*time.Minute)),
				}
				So(datastore.Put(ctx, builds), ShouldBeNil)
				So(ReportBuilderMetrics(ctx), ShouldBeNil)
				// 2 failures, 1 infra-failure, and 1 cancel.
				// Note that the first cancel is ignored because it happened before
				// the success.
				So(count("FAILURE"), ShouldEqual, 2)
				So(count("INFRA_FAILURE"), ShouldEqual, 1)
				So(count("CANCELED"), ShouldEqual, 1)

				Convey("custom", func() {
					base := pb.CustomMetricBase_CUSTOM_METRIC_BASE_CONSECUTIVE_FAILURE_COUNT
					name1 := "chrome/infra/custom/builds/failures1"
					name2 := "chrome/infra/custom/builds/failures2"

					globalCfg := &pb.SettingsCfg{
						CustomMetrics: []*pb.CustomMetric{
							{
								Name: name1,
								Class: &pb.CustomMetric_MetricBase{
									MetricBase: base,
								},
							},
							{
								Name: name2,
								Class: &pb.CustomMetric_MetricBase{
									MetricBase: base,
								},
							},
						},
					}
					ctx = WithBuilder(ctx, prj, bkt, "b1")
					ctx, _ = WithCustomMetrics(ctx, globalCfg)

					bldrMetrics := &model.CustomBuilderMetrics{
						Key:        model.CustomBuilderMetricsKey(ctx),
						LastUpdate: testclock.TestRecentTimeUTC,
						Metrics: &modeldefs.CustomBuilderMetrics{
							Metrics: []*modeldefs.CustomBuilderMetric{
								{
									Name: name1,
									Builders: []*pb.BuilderID{
										{
											Project: prj,
											Bucket:  bkt,
											Builder: "b1",
										},
									},
								},
								{
									Name: name2,
									Builders: []*pb.BuilderID{
										{
											Project: prj,
											Bucket:  bkt,
											Builder: "b1",
										},
									},
								},
							},
						},
					}
					So(datastore.Put(ctx, bldrMetrics), ShouldBeNil)
					for _, b := range builds {
						b.CustomBuilderConsecutiveFailuresMetrics = []string{name1}
					}
					builds[2].CustomBuilderConsecutiveFailuresMetrics = append(builds[2].CustomBuilderConsecutiveFailuresMetrics, name2)
					So(datastore.Put(ctx, builds), ShouldBeNil)
					So(ReportBuilderMetrics(ctx), ShouldBeNil)
					res, err := GetCustomMetricsData(ctx, base, name1, time.Time{}, []any{"FAILURE"})
					So(err, ShouldBeNil)
					So(res, ShouldEqual, 2)
					res, err = GetCustomMetricsData(ctx, base, name1, time.Time{}, []any{"INFRA_FAILURE"})
					So(err, ShouldBeNil)
					So(res, ShouldEqual, 1)
					res, err = GetCustomMetricsData(ctx, base, name1, time.Time{}, []any{"CANCELED"})
					So(err, ShouldBeNil)
					So(res, ShouldEqual, 1)
					res, err = GetCustomMetricsData(ctx, base, name2, time.Time{}, []any{"FAILURE"})
					So(err, ShouldBeNil)
					So(res, ShouldEqual, 1)
					res, err = GetCustomMetricsData(ctx, base, name2, time.Time{}, []any{"INFRA_FAILURE"})
					So(err, ShouldBeNil)
					So(res, ShouldEqual, 0)
				})
			})
			Convey("w/ a series of failures before a success", func() {
				builds := []*model.Build{
					B(pb.Status_CANCELED, t.Add(-5*time.Minute)),
					B(pb.Status_SUCCESS, t.Add(-4*time.Minute)),
					B(pb.Status_FAILURE, t.Add(-3*time.Minute)),
					B(pb.Status_INFRA_FAILURE, t.Add(-2*time.Minute)),
					B(pb.Status_CANCELED, t.Add(-1*time.Minute)),
					B(pb.Status_SUCCESS, t.Add(time.Minute)),
				}
				So(datastore.Put(ctx, builds), ShouldBeNil)
				So(ReportBuilderMetrics(ctx), ShouldBeNil)
				So(count("FAILURE"), ShouldEqual, 0)
				So(count("INFRA_FAILURE"), ShouldEqual, 0)
				So(count("CANCELED"), ShouldEqual, 0)
			})
		})
	})
}
