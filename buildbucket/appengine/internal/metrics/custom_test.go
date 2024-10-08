// Copyright 2024 The LUCI Authors.
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
	"sync"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/monitor"

	pb "go.chromium.org/luci/buildbucket/proto"
)

var testContext context.Context

var cfgContent = `
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
		name: "/chrome/infra/custom/builds/max_age",
		extra_fields: "os",
		metric_base: CUSTOM_METRIC_BASE_MAX_AGE_SCHEDULED,
	}
	custom_metrics {
		name: "/chrome/infra/custom/builds/count",
		extra_fields: "os",
		metric_base: CUSTOM_METRIC_BASE_COUNT,
	}
`

func init() {
	testContext = resetCustomMetrics(context.Background())
}

func resetCustomMetrics(ctx context.Context) context.Context {
	cfg := &pb.SettingsCfg{}
	_ = prototext.Unmarshal([]byte(cfgContent), cfg)
	ctx = WithServiceInfo(ctx, "svc", "job", "ins")
	ctx = WithBuilder(ctx, "project", "bucket", "builder")
	ctx, _ = WithCustomMetrics(ctx, cfg)
	return ctx
}

func getCurrentMetricsAndState(ctx context.Context) (map[pb.CustomMetricBase]map[string]CustomMetric, *tsmon.State) {
	cms := GetCustomMetrics(ctx)
	cms.m.RLock()
	defer cms.m.RUnlock()
	return cms.metrics, cms.state
}

func TestWithCustomMetrics(t *testing.T) {
	ctx, _ := tsmon.WithDummyInMemory(testContext)

	ftt.Run("WithCustomMetrics", t, func(t *ftt.Test) {
		globalCfg := &pb.SettingsCfg{}
		_ = prototext.Unmarshal([]byte(cfgContent), globalCfg)

		t.Run("check metrics from test context", func(t *ftt.Test) {
			v2Custom, _ := getCurrentMetricsAndState(ctx)
			startM := v2Custom[pb.CustomMetricBase_CUSTOM_METRIC_BASE_STARTED]["/chrome/infra/custom/builds/started"]
			_, ok := startM.(*counter)
			assert.Loosely(t, ok, should.BeTrue)
			runDurationM := v2Custom[pb.CustomMetricBase_CUSTOM_METRIC_BASE_RUN_DURATIONS]["/chrome/infra/custom/builds/run_duration"]
			_, ok = runDurationM.(*cumulativeDistribution)
			assert.Loosely(t, ok, should.BeTrue)
			maxAgeM := v2Custom[pb.CustomMetricBase_CUSTOM_METRIC_BASE_MAX_AGE_SCHEDULED]["/chrome/infra/custom/builds/max_age"]
			_, ok = maxAgeM.(*float)
			assert.Loosely(t, ok, should.BeTrue)
			countM := v2Custom[pb.CustomMetricBase_CUSTOM_METRIC_BASE_COUNT]["/chrome/infra/custom/builds/count"]
			_, ok = countM.(*int)
			assert.Loosely(t, ok, should.BeTrue)
		})
	})
}

func TestUpdateCustomMetrics(t *testing.T) {
	ctx, _ := tsmon.WithDummyInMemory(testContext)

	ftt.Run("Flush/report", t, func(t *ftt.Test) {
		ctx = resetCustomMetrics(ctx)
		globalCfg := &pb.SettingsCfg{}
		_ = prototext.Unmarshal([]byte(cfgContent), globalCfg)
		t.Run("normal report", func(t *ftt.Test) {
			cms := GetCustomMetrics(ctx)
			// Normal report.
			cms.Report(ctx, &Report{
				Base: pb.CustomMetricBase_CUSTOM_METRIC_BASE_STARTED,
				Name: "/chrome/infra/custom/builds/started",
				FieldMap: map[string]string{
					"os": "linux",
				},
				Value: int64(1),
			})
			err := cms.Flush(ctx, globalCfg, monitor.NewNilMonitor())
			assert.Loosely(t, err, should.BeNil)
		})

		flushAndMultiReports := func(globalCfg *pb.SettingsCfg, buffered *bool) {
			var wg sync.WaitGroup
			var mu sync.Mutex

			wg.Add(1)
			go func() {
				defer wg.Done()
				cms := GetCustomMetrics(ctx)
				_ = cms.Flush(ctx, globalCfg, monitor.NewNilMonitor())
			}()

			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					cms := GetCustomMetrics(ctx)
					directUpdated := cms.Report(ctx, &Report{
						Base: pb.CustomMetricBase_CUSTOM_METRIC_BASE_STARTED,
						Name: "/chrome/infra/custom/builds/started",
						FieldMap: map[string]string{
							"experiments": "None",
							"os":          "linux",
						},
						Value: int64(1),
					})
					mu.Lock()
					if !directUpdated && !*buffered {
						*buffered = true
					}
					mu.Unlock()
				}()
			}
			wg.Wait()
		}

		t.Run("normal report synced", func(t *ftt.Test) {
			oldCms := GetCustomMetrics(ctx)
			oldState := oldCms.state

			buffered := false
			flushAndMultiReports(globalCfg, &buffered)
			assert.Loosely(t, buffered, should.BeFalse)
			newCms := GetCustomMetrics(ctx)
			assert.Loosely(t, newCms.state, should.Equal(oldState))
		})

		t.Run("with a metric updating extra_fields", func(t *ftt.Test) {
			oldCms := GetCustomMetrics(ctx)
			oldState := oldCms.state

			buffered := false
			globalCfg.CustomMetrics[0].ExtraFields = append(globalCfg.CustomMetrics[0].ExtraFields, "new_field")
			flushAndMultiReports(globalCfg, &buffered)
			// This line makes the test flaky.
			//So(buffered, ShouldBeTrue)
			newCms := GetCustomMetrics(ctx)
			newCms.m.RLock()
			assert.Loosely(t, newCms.state, should.NotEqual(oldState))
			newCms.m.RUnlock()
			assert.Loosely(t, len(oldCms.buf), should.BeZero)
			assert.Loosely(t, len(newCms.buf), should.BeZero)
		})
	})
}
