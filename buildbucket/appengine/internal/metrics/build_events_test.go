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

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

// lfv generate field values for legacy metrics
func lfv(vs ...any) []any {
	ret := []any{"luci.project.bucket", "builder"}
	return append(ret, vs...)
}

// fv generate field values for v2 metrics.
func fv(vs ...any) []any {
	return vs
}

func pbTS(t time.Time) *timestamppb.Timestamp {
	pbts := timestamppb.New(t)
	return pbts
}

func setBuildCustomMetrics(b *model.Build, base pb.CustomMetricBase, name string) {
	b.CustomMetrics = []model.CustomMetric{
		{
			Base: base,
			Metric: &pb.CustomMetricDefinition{
				Name:       name,
				Predicates: []string{`build.tags.get_value("os")!=""`},
				ExtraFields: map[string]string{
					"os": `build.tags.get_value("os")`,
				},
			},
		},
	}
}

var cfgWithBuildEventMetrics = `
custom_metrics {
	name: "/chrome/infra/custom/builds/created",
	extra_fields: "os",
	metric_base: CUSTOM_METRIC_BASE_CREATED,
}
custom_metrics {
	name: "/chrome/infra/custom/builds/started",
	extra_fields: "os",
	metric_base: CUSTOM_METRIC_BASE_STARTED,
}
custom_metrics {
	name: "/chrome/infra/custom/builds/completed",
	extra_fields: "os",
	metric_base: CUSTOM_METRIC_BASE_COMPLETED,
}
custom_metrics {
	name: "/chrome/infra/custom/builds/run_duration",
	extra_fields: "os",
	metric_base: CUSTOM_METRIC_BASE_RUN_DURATIONS,
}
custom_metrics {
	name: "/chrome/infra/custom/builds/scheduling_duration",
	extra_fields: "os",
	metric_base: CUSTOM_METRIC_BASE_SCHEDULING_DURATIONS,
}
custom_metrics {
	name: "/chrome/infra/custom/builds/cycle_duration",
	extra_fields: "os",
	metric_base: CUSTOM_METRIC_BASE_CYCLE_DURATIONS,
}
`

func TestBuildEvents(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		ctx = WithServiceInfo(ctx, "svc", "job", "ins")
		ctx = WithBuilder(ctx, "project", "bucket", "builder")
		globalStore := tsmon.Store(ctx)
		cfg := &pb.SettingsCfg{}
		_ = prototext.Unmarshal([]byte(cfgWithBuildEventMetrics), cfg)
		ctx, _ = WithCustomMetrics(ctx, cfg)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

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
		assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)

		t.Run("buildCreated", func(t *ftt.Test) {
			b.Tags = []string{"os:linux"}
			BuildCreated(ctx, b)
			assert.Loosely(t, globalStore.Get(ctx, V1.BuildCountCreated, time.Time{}, lfv("")), should.Equal(1))
			assert.Loosely(t, globalStore.Get(ctx, V2.BuildCountCreated, time.Time{}, fv("None")), should.Equal(1))

			// user_agent
			b.Tags = []string{"user_agent:gerrit"}
			BuildCreated(ctx, b)
			assert.Loosely(t, globalStore.Get(ctx, V1.BuildCountCreated, time.Time{}, lfv("gerrit")), should.Equal(1))

			// experiments
			b.Experiments = []string{"+exp1"}
			BuildCreated(ctx, b)
			assert.Loosely(t, globalStore.Get(ctx, V2.BuildCountCreated, time.Time{}, fv("exp1")), should.Equal(1))

			base := pb.CustomMetricBase_CUSTOM_METRIC_BASE_CREATED
			name := "/chrome/infra/custom/builds/created"
			t.Run("predicate", func(t *ftt.Test) {
				setBuildCustomMetrics(b, base, name)
				t.Run("build meets the custome metric predicate", func(t *ftt.Test) {
					b.Tags = []string{"os:linux"}
					assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
					BuildCreated(ctx, b)
					v2Custom, state := getCustomMetricsAndState(ctx)
					customStore := state.Store()
					cm := v2Custom[base][name].(*counter)
					assert.Loosely(t, customStore.Get(ctx, cm, time.Time{}, fv("linux")), should.Equal(1))
					assert.Loosely(t, globalStore.Get(ctx, cm, time.Time{}, fv("linux")), should.BeNil)
				})
				t.Run("build doesn't meet the custome metric predicate", func(t *ftt.Test) {
					b.Tags = []string{"other:linux"}
					assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
					BuildCreated(ctx, b)
					res, err := GetCustomMetricsData(ctx, base, name, time.Time{}, fv("linux"))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res, should.BeNil)
				})
			})
			t.Run("pass predicates but failed to evaluate extra_fields", func(t *ftt.Test) {
				b.CustomMetrics = []model.CustomMetric{
					{
						Base: base,
						Metric: &pb.CustomMetricDefinition{
							Name:       name,
							Predicates: []string{`has(build.summary_markdown)`},
							ExtraFields: map[string]string{
								"os": `build.tags.get_value("os")`,
							},
						},
					},
				}
				b.Proto.SummaryMarkdown = "summary"
				b.Tags = []string{"other:linux"}
				assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
				BuildCreated(ctx, b)
				res, err := GetCustomMetricsData(ctx, base, name, time.Time{}, fv("linux"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.BeNil)
			})
		})

		t.Run("buildStarted", func(t *ftt.Test) {
			t.Run("build/started", func(t *ftt.Test) {
				// canary
				b.Proto.Canary = false
				BuildStarted(ctx, b)
				assert.Loosely(t, globalStore.Get(ctx, V1.BuildCountStarted, time.Time{}, lfv(false)), should.Equal(1))

				b.Proto.Canary = true
				BuildStarted(ctx, b)
				assert.Loosely(t, globalStore.Get(ctx, V1.BuildCountStarted, time.Time{}, lfv(true)), should.Equal(1))
				assert.Loosely(t, globalStore.Get(ctx, V2.BuildCountStarted, time.Time{}, fv("None")), should.Equal(2))

				// experiments
				b.Experiments = []string{"+exp1"}
				BuildStarted(ctx, b)
				assert.Loosely(t, globalStore.Get(ctx, V2.BuildCountStarted, time.Time{}, fv("exp1")), should.Equal(1))

				// Custom metrics
				base := pb.CustomMetricBase_CUSTOM_METRIC_BASE_STARTED
				name := "/chrome/infra/custom/builds/started"
				setBuildCustomMetrics(b, base, name)
				b.Tags = []string{"os:linux"}
				assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
				BuildStarted(ctx, b)
				res, err := GetCustomMetricsData(ctx, base, name, time.Time{}, fv("linux"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Equal(1))
			})

			t.Run("build/scheduling_durations", func(t *ftt.Test) {
				fields := lfv("", "", "", true)
				b.Proto.StartTime = pbTS(b.CreateTime.Add(33 * time.Second))
				BuildStarted(ctx, b)
				val := globalStore.Get(ctx, V1.BuildDurationScheduling, time.Time{}, fields)
				assert.That(t, val.(*distribution.Distribution).Sum(), should.Equal(33.0))
				val = globalStore.Get(ctx, V2.BuildDurationScheduling, time.Time{}, fv("None"))
				assert.That(t, val.(*distribution.Distribution).Sum(), should.Equal(33.0))

				// experiments
				b.Experiments = []string{"+exp1"}
				BuildStarted(ctx, b)
				val = globalStore.Get(ctx, V2.BuildDurationScheduling, time.Time{}, fv("exp1"))
				assert.That(t, val.(*distribution.Distribution).Sum(), should.Equal(33.0))

				// Custom metrics
				base := pb.CustomMetricBase_CUSTOM_METRIC_BASE_SCHEDULING_DURATIONS
				name := "/chrome/infra/custom/builds/scheduling_duration"
				setBuildCustomMetrics(b, base, name)
				b.Tags = []string{"os:linux"}
				assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
				BuildStarted(ctx, b)
				res, err := GetCustomMetricsData(ctx, base, name, time.Time{}, fv("linux"))
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res.(*distribution.Distribution).Sum(), should.Equal(33.0))
			})
		})

		t.Run("BuildCompleted", func(t *ftt.Test) {
			t.Run("builds/completed", func(t *ftt.Test) {
				b.Status = pb.Status_FAILURE
				BuildCompleted(ctx, b)
				v1fs := lfv(model.Failure.String(), model.BuildFailure.String(), "", true)
				assert.Loosely(t, globalStore.Get(ctx, V1.BuildCountCompleted, time.Time{}, v1fs), should.Equal(1))
				v2fs := fv("FAILURE", "None")
				assert.Loosely(t, globalStore.Get(ctx, V2.BuildCountCompleted, time.Time{}, v2fs), should.Equal(1))

				b.Status = pb.Status_CANCELED
				BuildCompleted(ctx, b)
				v1fs = lfv(model.Canceled.String(), "", model.ExplicitlyCanceled.String(), true)
				assert.Loosely(t, globalStore.Get(ctx, V1.BuildCountCompleted, time.Time{}, v1fs), should.Equal(1))
				v2fs[0] = "CANCELED"
				assert.Loosely(t, globalStore.Get(ctx, V2.BuildCountCompleted, time.Time{}, v2fs), should.Equal(1))

				b.Status = pb.Status_INFRA_FAILURE
				BuildCompleted(ctx, b)
				v1fs = lfv(model.Failure.String(), model.InfraFailure.String(), "", true)
				assert.Loosely(t, globalStore.Get(ctx, V1.BuildCountCompleted, time.Time{}, v1fs), should.Equal(1))
				v2fs[0] = "INFRA_FAILURE"
				assert.Loosely(t, globalStore.Get(ctx, V2.BuildCountCompleted, time.Time{}, v2fs), should.Equal(1))

				// timeout
				b.Status = pb.Status_INFRA_FAILURE
				b.Proto.StatusDetails = &pb.StatusDetails{Timeout: &pb.StatusDetails_Timeout{}}
				BuildCompleted(ctx, b)
				v1fs = lfv(model.Failure.String(), model.InfraFailure.String(), "", true)
				assert.Loosely(t, globalStore.Get(ctx, V1.BuildCountCompleted, time.Time{}, v1fs), should.Equal(1))
				v2fs = fv("INFRA_FAILURE", "None")
				assert.Loosely(t, globalStore.Get(ctx, V2.BuildCountCompleted, time.Time{}, v2fs), should.Equal(2))

				// experiments
				b.Status = pb.Status_SUCCESS
				b.Experiments = []string{"+exp1", "+exp2"}
				v2fs = fv("SUCCESS", "exp1|exp2")
				BuildCompleted(ctx, b)
				assert.Loosely(t, globalStore.Get(ctx, V2.BuildCountCompleted, time.Time{}, v2fs), should.Equal(1))

				// Custom metrics
				base := pb.CustomMetricBase_CUSTOM_METRIC_BASE_COMPLETED
				name := "/chrome/infra/custom/builds/completed"
				setBuildCustomMetrics(b, base, name)
				b.Tags = []string{"os:linux"}
				b.Proto.Status = pb.Status_SUCCESS
				assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
				BuildCompleted(ctx, b)
				res, err := GetCustomMetricsData(ctx, base, name, time.Time{}, fv("SUCCESS", "linux"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Equal(1))
			})

			b.Status = pb.Status_SUCCESS
			v1fs := lfv("SUCCESS", "", "", true)
			v2fs := fv("SUCCESS", "None")

			t.Run("builds/cycle_durations", func(t *ftt.Test) {
				b.Proto.EndTime = pbTS(b.CreateTime.Add(33 * time.Second))
				BuildCompleted(ctx, b)
				val := globalStore.Get(ctx, V1.BuildDurationCycle, time.Time{}, v1fs)
				assert.That(t, val.(*distribution.Distribution).Sum(), should.Equal(33.0))
				val = globalStore.Get(ctx, V2.BuildDurationCycle, time.Time{}, v2fs)
				assert.That(t, val.(*distribution.Distribution).Sum(), should.Equal(33.0))

				// experiments
				b.Experiments = []string{"+exp2", "+exp1"}
				BuildCompleted(ctx, b)
				val = globalStore.Get(ctx, V2.BuildDurationCycle, time.Time{}, fv("SUCCESS", "exp1|exp2"))
				assert.That(t, val.(*distribution.Distribution).Sum(), should.Equal(33.0))

				// Custom metrics
				base := pb.CustomMetricBase_CUSTOM_METRIC_BASE_CYCLE_DURATIONS
				name := "/chrome/infra/custom/builds/cycle_duration"
				setBuildCustomMetrics(b, base, name)
				b.Tags = []string{"os:linux"}
				b.Proto.Status = pb.Status_SUCCESS
				assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
				BuildCompleted(ctx, b)
				res, err := GetCustomMetricsData(ctx, base, name, time.Time{}, fv("SUCCESS", "linux"))
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res.(*distribution.Distribution).Sum(), should.Equal(33.0))
			})

			t.Run("builds/run_durations", func(t *ftt.Test) {
				b.Proto.StartTime = pbTS(b.CreateTime.Add(3 * time.Second))
				b.Proto.EndTime = pbTS(b.CreateTime.Add(33 * time.Second))

				BuildCompleted(ctx, b)
				val := globalStore.Get(ctx, V1.BuildDurationRun, time.Time{}, v1fs)
				assert.That(t, val.(*distribution.Distribution).Sum(), should.Equal(30.0))
				val = globalStore.Get(ctx, V2.BuildDurationRun, time.Time{}, v2fs)
				assert.That(t, val.(*distribution.Distribution).Sum(), should.Equal(30.0))

				// experiments
				b.Experiments = []string{"+exp2", "+exp1"}
				BuildCompleted(ctx, b)
				val = globalStore.Get(ctx, V2.BuildDurationRun, time.Time{}, fv("SUCCESS", "exp1|exp2"))
				assert.That(t, val.(*distribution.Distribution).Sum(), should.Equal(30.0))

				// Custom metrics
				base := pb.CustomMetricBase_CUSTOM_METRIC_BASE_RUN_DURATIONS
				name := "/chrome/infra/custom/builds/run_duration"
				setBuildCustomMetrics(b, base, name)
				b.Tags = []string{"os:linux"}
				b.Proto.Status = pb.Status_SUCCESS
				assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
				BuildCompleted(ctx, b)
				res, err := GetCustomMetricsData(ctx, base, name, time.Time{}, fv("SUCCESS", "linux"))
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res.(*distribution.Distribution).Sum(), should.Equal(30.0))
			})
		})

		t.Run("ExpiredLeaseReset", func(t *ftt.Test) {
			b.Status = pb.Status_SCHEDULED
			ExpiredLeaseReset(ctx, b)
			assert.Loosely(t, globalStore.Get(ctx, V1.ExpiredLeaseReset, time.Time{}, lfv("SCHEDULED")), should.Equal(1))
		})
	})
}
