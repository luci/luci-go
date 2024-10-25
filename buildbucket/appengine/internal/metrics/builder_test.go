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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/model"
	modeldefs "go.chromium.org/luci/buildbucket/appengine/model/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestReportBuilderMetrics(t *testing.T) {
	t.Parallel()

	ftt.Run("ReportBuilderMetrics", t, func(t *ftt.Test) {
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

		t.Run("report v2.BuilderPresence", func(t *ftt.Test) {
			assert.Loosely(t, createBuilder("b1"), should.BeNil)
			assert.Loosely(t, createBuilder("b2"), should.BeNil)
			assert.Loosely(t, ReportBuilderMetrics(ctx), should.BeNil)
			assert.Loosely(t, store.Get(target("b1"), V2.BuilderPresence, nil), should.Equal(true))
			assert.Loosely(t, store.Get(target("b2"), V2.BuilderPresence, nil), should.Equal(true))
			assert.Loosely(t, store.Get(target("b3"), V2.BuilderPresence, nil), should.BeNil)

			t.Run("w/o removed builder", func(t *ftt.Test) {
				assert.Loosely(t, deleteBuilder("b1"), should.BeNil)
				assert.Loosely(t, ReportBuilderMetrics(ctx), should.BeNil)
				assert.Loosely(t, store.Get(target("b1"), V2.BuilderPresence, nil), should.BeNil)
				assert.Loosely(t, store.Get(target("b2"), V2.BuilderPresence, nil), should.Equal(true))
			})

			t.Run("w/o inactive builder", func(t *ftt.Test) {
				// Let's pretend that b1 was inactive for 4 weeks, and
				// got unregistered from the BuilderStat.
				assert.Loosely(t, datastore.Delete(ctx, &model.BuilderStat{ID: prj + ":" + bkt + ":b1"}), should.BeNil)
				assert.Loosely(t, ReportBuilderMetrics(ctx), should.BeNil)
				// b1 should no longer be reported in the presence metric.
				assert.Loosely(t, store.Get(target("b1"), V2.BuilderPresence, nil), should.BeNil)
				assert.Loosely(t, store.Get(target("b2"), V2.BuilderPresence, nil), should.Equal(true))
			})
		})

		t.Run("report MaxAgeScheduled", func(t *ftt.Test) {
			assert.Loosely(t, createBuilder("b1"), should.BeNil)
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

			t.Run("never_leased_age >= leased_age", func(t *ftt.Test) {
				builds[0].Proto.CreateTime = timestamppb.New(now.Add(-2 * time.Hour))
				builds[1].Proto.CreateTime = timestamppb.New(now.Add(-1 * time.Hour))
				assert.Loosely(t, datastore.Put(ctx, builds[0], builds[1]), should.BeNil)
				assert.Loosely(t, ReportBuilderMetrics(ctx), should.BeNil)

				age := (2 * time.Hour).Seconds()

				t.Run("v1", func(t *ftt.Test) {
					// the ages should be the same.
					fields := []any{bkt, "b1", true}
					assert.Loosely(t, store.Get(ctx, V1.MaxAgeScheduled, fields), should.Equal(age))
					fields = []any{bkt, "b1", false}
					assert.Loosely(t, store.Get(ctx, V1.MaxAgeScheduled, fields), should.Equal(age))
				})

				t.Run("v2", func(t *ftt.Test) {
					assert.Loosely(t, store.Get(target("b1"), V2.MaxAgeScheduled, nil), should.Equal(age))
				})
			})

			t.Run("v1: never_leased_age < leased_age", func(t *ftt.Test) {
				builds[0].Proto.CreateTime = timestamppb.New(now.Add(-1 * time.Hour))
				builds[1].Proto.CreateTime = timestamppb.New(now.Add(-2 * time.Hour))
				assert.Loosely(t, datastore.Put(ctx, builds[0], builds[1]), should.BeNil)
				assert.Loosely(t, ReportBuilderMetrics(ctx), should.BeNil)

				age := time.Hour.Seconds()

				t.Run("v1", func(t *ftt.Test) {
					// the ages should be different.
					fields := []any{bkt, "b1", true}
					assert.Loosely(t, store.Get(ctx, V1.MaxAgeScheduled, fields), should.Equal(age))
					fields = []any{bkt, "b1", false}
					assert.Loosely(t, store.Get(ctx, V1.MaxAgeScheduled, fields), should.Equal(2*age))
				})

				t.Run("v2", func(t *ftt.Test) {
					assert.Loosely(t, store.Get(target("b1"), V2.MaxAgeScheduled, nil), should.Equal(2*age))
				})

				t.Run("custom", func(t *ftt.Test) {
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

					t.Run("builder no custom metric", func(t *ftt.Test) {
						assert.Loosely(t, ReportBuilderMetrics(ctx), should.BeNil)
						res, err := GetCustomMetricsData(ctx, base, name, nil)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, res, should.BeNil)
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
					assert.Loosely(t, datastore.Put(ctx, bldr), should.BeNil)
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
					assert.Loosely(t, datastore.Put(ctx, bldrMetrics), should.BeNil)
					t.Run("no build populates the metric", func(t *ftt.Test) {
						assert.Loosely(t, ReportBuilderMetrics(ctx), should.BeNil)
						res, err := GetCustomMetricsData(ctx, base, name, nil)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, res, should.BeNil)
					})

					t.Run("work", func(t *ftt.Test) {
						builds[0].CustomBuilderMaxAgeMetrics = []string{name}
						assert.Loosely(t, datastore.Put(ctx, builds[0]), should.BeNil)
						assert.Loosely(t, ReportBuilderMetrics(ctx), should.BeNil)

						res, err := GetCustomMetricsData(ctx, base, name, nil)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, res, should.Equal(age))
					})
				})
			})

			t.Run("w/ swarming config in bucket", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, &model.Bucket{
					Parent: model.ProjectKey(ctx, prj), ID: bkt,
					Proto: &pb.Bucket{Swarming: &pb.Swarming{}},
				}), should.BeNil)
				assert.Loosely(t, datastore.Put(ctx, builds[0], builds[1]), should.BeNil)
				assert.Loosely(t, ReportBuilderMetrics(ctx), should.BeNil)

				t.Run("v1", func(t *ftt.Test) {
					// Data should have been reported with "luci.$project.$bucket"
					fields := []any{bkt, "b1", true}
					assert.Loosely(t, store.Get(ctx, V1.MaxAgeScheduled, fields), should.BeNil)
					fields = []any{"luci." + prj + "." + bkt, "b1", true}
					assert.Loosely(t, store.Get(ctx, V1.MaxAgeScheduled, fields), should.NotBeNilInterface)
				})

				t.Run("v2", func(t *ftt.Test) {
					// V2 doesn't care. It always reports the bucket name as it is.
					assert.Loosely(t, store.Get(target("b1"), V2.MaxAgeScheduled, nil), should.NotBeNilInterface)
				})
			})
		})

		t.Run("report ConsecutiveFailures", func(t *ftt.Test) {
			assert.Loosely(t, createBuilder("b1"), should.BeNil)
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
				return store.Get(target("b1"), V2.ConsecutiveFailureCount, []any{s})
			}
			ts := clock.Now()

			t.Run("w/o success", func(t *ftt.Test) {
				builds := []*model.Build{
					B(pb.Status_CANCELED, ts.Add(-4*time.Minute)),
					B(pb.Status_FAILURE, ts.Add(-3*time.Minute)),
					B(pb.Status_INFRA_FAILURE, ts.Add(-2*time.Minute)),
					B(pb.Status_CANCELED, ts.Add(-1*time.Minute)),
				}
				assert.Loosely(t, datastore.Put(ctx, builds), should.BeNil)
				assert.Loosely(t, ReportBuilderMetrics(ctx), should.BeNil)
				assert.Loosely(t, count("FAILURE"), should.Equal(1))
				assert.Loosely(t, count("INFRA_FAILURE"), should.Equal(1))
				assert.Loosely(t, count("CANCELED"), should.Equal(2))
			})

			t.Run("w/ success only", func(t *ftt.Test) {
				builds := []*model.Build{
					B(pb.Status_SUCCESS, ts.Add(-3*time.Minute)),
					B(pb.Status_SUCCESS, ts.Add(-2*time.Minute)),
					B(pb.Status_SUCCESS, ts.Add(-1*time.Minute)),
				}
				assert.Loosely(t, datastore.Put(ctx, builds), should.BeNil)
				assert.Loosely(t, ReportBuilderMetrics(ctx), should.BeNil)
				// The count for each status should still be reported w/o 0.
				assert.Loosely(t, count("FAILURE"), should.BeZero)
				assert.Loosely(t, count("INFRA_FAILURE"), should.BeZero)
				assert.Loosely(t, count("CANCELED"), should.BeZero)
			})

			t.Run("w/ a series of failures after success", func(t *ftt.Test) {
				builds := []*model.Build{
					B(pb.Status_CANCELED, ts.Add(-6*time.Minute)),
					B(pb.Status_SUCCESS, ts.Add(-5*time.Minute)),
					B(pb.Status_FAILURE, ts.Add(-4*time.Minute)),
					B(pb.Status_FAILURE, ts.Add(-3*time.Minute)),
					B(pb.Status_INFRA_FAILURE, ts.Add(-2*time.Minute)),
					B(pb.Status_CANCELED, ts.Add(-1*time.Minute)),
				}
				assert.Loosely(t, datastore.Put(ctx, builds), should.BeNil)
				assert.Loosely(t, ReportBuilderMetrics(ctx), should.BeNil)
				// 2 failures, 1 infra-failure, and 1 cancel.
				// Note that the first cancel is ignored because it happened before
				// the success.
				assert.Loosely(t, count("FAILURE"), should.Equal(2))
				assert.Loosely(t, count("INFRA_FAILURE"), should.Equal(1))
				assert.Loosely(t, count("CANCELED"), should.Equal(1))

				t.Run("custom", func(t *ftt.Test) {
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
					assert.Loosely(t, datastore.Put(ctx, bldrMetrics), should.BeNil)
					for _, b := range builds {
						b.CustomBuilderConsecutiveFailuresMetrics = []string{name1}
					}
					builds[2].CustomBuilderConsecutiveFailuresMetrics = append(builds[2].CustomBuilderConsecutiveFailuresMetrics, name2)
					assert.Loosely(t, datastore.Put(ctx, builds), should.BeNil)
					assert.Loosely(t, ReportBuilderMetrics(ctx), should.BeNil)
					res, err := GetCustomMetricsData(ctx, base, name1, []any{"FAILURE"})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res, should.Equal(2))
					res, err = GetCustomMetricsData(ctx, base, name1, []any{"INFRA_FAILURE"})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res, should.Equal(1))
					res, err = GetCustomMetricsData(ctx, base, name1, []any{"CANCELED"})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res, should.Equal(1))
					res, err = GetCustomMetricsData(ctx, base, name2, []any{"FAILURE"})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res, should.Equal(1))
					res, err = GetCustomMetricsData(ctx, base, name2, []any{"INFRA_FAILURE"})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res, should.BeZero)
				})
			})
			t.Run("w/ a series of failures before a success", func(t *ftt.Test) {
				builds := []*model.Build{
					B(pb.Status_CANCELED, ts.Add(-5*time.Minute)),
					B(pb.Status_SUCCESS, ts.Add(-4*time.Minute)),
					B(pb.Status_FAILURE, ts.Add(-3*time.Minute)),
					B(pb.Status_INFRA_FAILURE, ts.Add(-2*time.Minute)),
					B(pb.Status_CANCELED, ts.Add(-1*time.Minute)),
					B(pb.Status_SUCCESS, ts.Add(time.Minute)),
				}
				assert.Loosely(t, datastore.Put(ctx, builds), should.BeNil)
				assert.Loosely(t, ReportBuilderMetrics(ctx), should.BeNil)
				assert.Loosely(t, count("FAILURE"), should.BeZero)
				assert.Loosely(t, count("INFRA_FAILURE"), should.BeZero)
				assert.Loosely(t, count("CANCELED"), should.BeZero)
			})
		})
	})
}
