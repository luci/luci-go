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

package buildcron

import (
	"context"
	"sort"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/buildbucket/appengine/internal/buildid"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
)

// now needs to be further fresh enough from buildid.beginningOfTheWorld
var now = time.Date(2020, 01, 01, 0, 0, 0, 0, time.UTC)

func setUp() (context.Context, store.Store, *tqtesting.Scheduler) {
	ctx := memory.Use(context.Background())
	datastore.GetTestable(ctx).AutoIndex(true)
	datastore.GetTestable(ctx).Consistent(true)
	ctx = txndefer.FilterRDS(ctx)

	ctx, _ = tsmon.WithDummyInMemory(ctx)
	ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
	ctx = metrics.WithBuilder(ctx, "project", "bucket", "builder")
	ctx, _ = metrics.WithCustomMetrics(ctx, &pb.SettingsCfg{})
	store := tsmon.Store(ctx)
	ctx, sch := tq.TestingContext(ctx, nil)

	ctx, _ = testclock.UseTime(ctx, now)
	return ctx, store, sch
}

func newBuildAndStatus(ctx context.Context, st pb.Status, t time.Time) (*model.Build, *model.BuildStatus) {
	id := buildid.NewBuildIDs(ctx, t, 1)[0]
	b := &model.Build{
		ID: id,
		Proto: &pb.Build{
			Id: id,
			Builder: &pb.BuilderID{
				Project: "project",
				Bucket:  "bucket",
				Builder: "builder",
			},
			Status: st,
		},
	}
	bs := &model.BuildStatus{
		Build:  datastore.KeyForObj(ctx, b),
		Status: st,
	}
	return b, bs
}

func newInfraFromBuild(ctx context.Context, b *model.Build) *model.BuildInfra {
	return &model.BuildInfra{
		Build: datastore.KeyForObj(ctx, b),
		Proto: &pb.BuildInfra{
			Resultdb: &pb.BuildInfra_ResultDB{
				Hostname:   "rdbhost",
				Invocation: "inv",
			},
		},
	}
}

func TestTimeoutExpiredBuilds(t *testing.T) {
	t.Parallel()

	ftt.Run("TimeoutExpiredBuilds", t, func(t *ftt.Test) {
		ctx, store, sch := setUp()

		t.Run("skips young, running builds", func(t *ftt.Test) {
			b1, bs1 := newBuildAndStatus(ctx, pb.Status_SCHEDULED, now.Add(-model.BuildMaxCompletionTime))
			b2, bs2 := newBuildAndStatus(ctx, pb.Status_STARTED, now.Add(-model.BuildMaxCompletionTime))
			assert.Loosely(t, datastore.Put(ctx, b1, b2, bs1, bs2), should.BeNil)
			assert.Loosely(t, TimeoutExpiredBuilds(ctx), should.BeNil)

			b := &model.Build{ID: b1.ID}
			bs := &model.BuildStatus{Build: datastore.KeyForObj(ctx, b)}
			assert.Loosely(t, datastore.Get(ctx, b, bs), should.BeNil)
			assert.Loosely(t, b.Proto, should.Match(b1.Proto))
			assert.Loosely(t, bs.Status, should.Equal(pb.Status_SCHEDULED))

			b = &model.Build{ID: b2.ID}
			bs = &model.BuildStatus{Build: datastore.KeyForObj(ctx, b)}
			assert.Loosely(t, datastore.Get(ctx, b, bs), should.BeNil)
			assert.Loosely(t, b.Proto, should.Match(b2.Proto))
			assert.Loosely(t, bs.Status, should.Equal(pb.Status_STARTED))
		})

		t.Run("skips old, completed builds", func(t *ftt.Test) {
			b1, bs1 := newBuildAndStatus(ctx, pb.Status_SUCCESS, now.Add(-model.BuildMaxCompletionTime))
			b2, bs2 := newBuildAndStatus(ctx, pb.Status_FAILURE, now.Add(-model.BuildMaxCompletionTime))
			assert.Loosely(t, datastore.Put(ctx, b1, b2, bs1, bs2), should.BeNil)
			assert.Loosely(t, TimeoutExpiredBuilds(ctx), should.BeNil)

			b := &model.Build{ID: b1.ID}
			assert.Loosely(t, datastore.Get(ctx, b), should.BeNil)
			assert.Loosely(t, b.Proto, should.Match(b1.Proto))

			b = &model.Build{ID: b2.ID}
			assert.Loosely(t, datastore.Get(ctx, b), should.BeNil)
			assert.Loosely(t, b.Proto, should.Match(b2.Proto))
		})

		t.Run("works w/ a large number of expired builds", func(t *ftt.Test) {
			bs := make([]*model.Build, 128)
			bss := make([]*model.BuildStatus, len(bs))
			infs := make([]*model.BuildInfra, len(bs))
			createTime := now.Add(-model.BuildMaxCompletionTime - time.Minute)
			for i := range bs {
				bs[i], bss[i] = newBuildAndStatus(ctx, pb.Status_SCHEDULED, createTime)
				infs[i] = newInfraFromBuild(ctx, bs[i])
			}
			bldr := &model.Builder{
				ID:     "builder",
				Parent: model.BucketKey(ctx, "project", "bucket"),
			}
			assert.Loosely(t, datastore.Put(ctx, bs), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, bss), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, infs), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, bldr), should.BeNil)
			assert.Loosely(t, TimeoutExpiredBuilds(ctx), should.BeNil)
		})

		t.Run("marks old, running builds w/ infra_failure", func(t *ftt.Test) {
			base := pb.CustomMetricBase_CUSTOM_METRIC_BASE_COMPLETED
			name := "/chrome/infra/custom/builds/completed"
			cm := &pb.CustomMetricDefinition{
				Name:       name,
				Predicates: []string{`build.status.to_string()=="INFRA_FAILURE"`},
				ExtraFields: map[string]string{
					"experiments": `build.input.experiments.to_string()`,
				},
			}
			cms := []model.CustomMetric{
				{
					Base:   base,
					Metric: cm,
				},
			}
			b1, bs1 := newBuildAndStatus(ctx, pb.Status_SCHEDULED, now.Add(-model.BuildMaxCompletionTime-time.Minute))
			inf1 := newInfraFromBuild(ctx, b1)
			b1.LegacyProperties.LeaseProperties.IsLeased = true
			b1.CustomMetrics = cms
			b2, bs2 := newBuildAndStatus(ctx, pb.Status_STARTED, now.Add(-model.BuildMaxCompletionTime-time.Minute))
			inf2 := newInfraFromBuild(ctx, b2)
			b2.CustomMetrics = cms
			bldr := &model.Builder{
				ID:     "builder",
				Parent: model.BucketKey(ctx, "project", "bucket"),
				Config: &pb.BuilderConfig{
					MaxConcurrentBuilds: 2,
				},
			}
			assert.Loosely(t, datastore.Put(ctx, b1, b2, bs1, bs2, inf1, inf2, bldr), should.BeNil)

			globalCfg := &pb.SettingsCfg{
				CustomMetrics: []*pb.CustomMetric{
					{
						Name: name,
						Class: &pb.CustomMetric_MetricBase{
							MetricBase: base,
						},
						ExtraFields: []string{"experiments"},
					},
				},
			}
			ctx, _ = metrics.WithCustomMetrics(ctx, globalCfg)

			assert.Loosely(t, TimeoutExpiredBuilds(ctx), should.BeNil)

			b := &model.Build{ID: b1.ID}
			bs := &model.BuildStatus{Build: datastore.KeyForObj(ctx, b)}
			assert.Loosely(t, datastore.Get(ctx, b, bs), should.BeNil)
			assert.Loosely(t, b.Proto.Status, should.Equal(pb.Status_INFRA_FAILURE))
			assert.Loosely(t, b.LegacyProperties.LeaseProperties.IsLeased, should.BeFalse)
			assert.Loosely(t, b.Proto.StatusDetails.GetTimeout(), should.NotBeNil)
			assert.Loosely(t, bs.Status, should.Equal(pb.Status_INFRA_FAILURE))

			b = &model.Build{ID: b2.ID}
			bs = &model.BuildStatus{Build: datastore.KeyForObj(ctx, b)}
			assert.Loosely(t, datastore.Get(ctx, b, bs), should.BeNil)
			assert.Loosely(t, b.Proto.Status, should.Equal(pb.Status_INFRA_FAILURE))
			assert.Loosely(t, b.Proto.StatusDetails.GetTimeout(), should.NotBeNil)
			assert.Loosely(t, bs.Status, should.Equal(pb.Status_INFRA_FAILURE))

			t.Run("reports metrics", func(t *ftt.Test) {
				fv := []any{
					"INFRA_FAILURE", /* metric:status */
					"None",          /* metric:experiments */
				}
				assert.Loosely(t, store.Get(ctx, metrics.V2.BuildCountCompleted, fv), should.Equal(2))

				// Custom metrics
				res, err := metrics.GetCustomMetricsData(ctx, base, name, fv)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Equal(2))
			})

			t.Run("adds TQ tasks", func(t *ftt.Test) {
				// TQ tasks for pubsub-notification, bq-export, and invocation-finalization.
				tasks := sch.Tasks()
				bqIDs := []int64{}
				rdbIDs := []int64{}
				expected := []int64{b1.ID, b2.ID}
				notifyGoIDs := []int64{}
				popPendingIDs := []int64{}

				for _, task := range tasks {
					switch v := task.Payload.(type) {
					case *taskdefs.ExportBigQueryGo:
						bqIDs = append(bqIDs, v.GetBuildId())
					case *taskdefs.FinalizeResultDBGo:
						rdbIDs = append(rdbIDs, v.GetBuildId())
					case *taskdefs.NotifyPubSubGoProxy:
						notifyGoIDs = append(notifyGoIDs, v.GetBuildId())
					case *taskdefs.PopPendingBuildTask:
						popPendingIDs = append(popPendingIDs, v.GetBuildId())

					default:
						panic("invalid task payload")
					}
				}

				sortIDs := func(ids []int64) {
					sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
				}
				sortIDs(bqIDs)
				sortIDs(rdbIDs)
				sortIDs(expected)
				sortIDs(notifyGoIDs)
				sortIDs(popPendingIDs)

				assert.Loosely(t, bqIDs, should.HaveLength(2))
				assert.Loosely(t, bqIDs, should.Match(expected))
				assert.Loosely(t, rdbIDs, should.HaveLength(2))
				assert.Loosely(t, rdbIDs, should.Match(expected))
				assert.Loosely(t, notifyGoIDs, should.HaveLength(2))
				assert.Loosely(t, notifyGoIDs, should.Match(expected))
				assert.Loosely(t, popPendingIDs, should.HaveLength(2))
				assert.Loosely(t, popPendingIDs, should.Match(expected))
			})
		})
	})
}
