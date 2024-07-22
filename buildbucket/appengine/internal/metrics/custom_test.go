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

	"go.chromium.org/luci/common/tsmon"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
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
	cms := getCustomMetrics(ctx)
	cms.m.RLock()
	defer cms.m.RUnlock()
	return cms.metrics, cms.state
}

func TestWithCustomMetrics(t *testing.T) {
	ctx, _ := tsmon.WithDummyInMemory(testContext)

	Convey("WithCustomMetrics", t, func() {
		globalCfg := &pb.SettingsCfg{}
		_ = prototext.Unmarshal([]byte(cfgContent), globalCfg)

		Convey("check metrics from test context", func() {
			v2Custom, _ := getCurrentMetricsAndState(ctx)
			startM := v2Custom[pb.CustomMetricBase_CUSTOM_METRIC_BASE_STARTED]["/chrome/infra/custom/builds/started"]
			_, ok := startM.(*counter)
			So(ok, ShouldBeTrue)
			runDurationM := v2Custom[pb.CustomMetricBase_CUSTOM_METRIC_BASE_RUN_DURATIONS]["/chrome/infra/custom/builds/run_duration"]
			_, ok = runDurationM.(*cumulativeDistribution)
			So(ok, ShouldBeTrue)
			maxAgeM := v2Custom[pb.CustomMetricBase_CUSTOM_METRIC_BASE_MAX_AGE_SCHEDULED]["/chrome/infra/custom/builds/max_age"]
			_, ok = maxAgeM.(*float)
			So(ok, ShouldBeTrue)
			countM := v2Custom[pb.CustomMetricBase_CUSTOM_METRIC_BASE_COUNT]["/chrome/infra/custom/builds/count"]
			_, ok = countM.(*int)
			So(ok, ShouldBeTrue)
		})
	})
}

func TestUpdateCustomMetrics(t *testing.T) {
	ctx, _ := tsmon.WithDummyInMemory(testContext)

	Convey("Flush/report", t, func() {
		ctx = resetCustomMetrics(ctx)
		globalCfg := &pb.SettingsCfg{}
		_ = prototext.Unmarshal([]byte(cfgContent), globalCfg)
		Convey("normal report", func() {
			cms := getCustomMetrics(ctx)
			// Normal report.
			cms.Report(ctx, &Report{
				Base: pb.CustomMetricBase_CUSTOM_METRIC_BASE_STARTED,
				Name: "/chrome/infra/custom/builds/started",
				FieldMap: map[string]string{
					"os": "linux",
				},
				Value: int64(1),
			})
			err := cms.Flush(ctx, globalCfg)
			So(err, ShouldBeNil)
		})

		flushAndMultiReports := func(globalCfg *pb.SettingsCfg, buffered *bool) {
			var wg sync.WaitGroup
			var mu sync.Mutex

			wg.Add(1)
			go func() {
				defer wg.Done()
				cms := getCustomMetrics(ctx)
				_ = cms.Flush(ctx, globalCfg)
			}()

			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					cms := getCustomMetrics(ctx)
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

		Convey("normal report synced", func() {
			oldCms := getCustomMetrics(ctx)
			oldState := oldCms.state

			buffered := false
			flushAndMultiReports(globalCfg, &buffered)
			So(buffered, ShouldBeFalse)
			newCms := getCustomMetrics(ctx)
			So(newCms.state, ShouldEqual, oldState)
		})

		Convey("with a metric updating extra_fields", func() {
			oldCms := getCustomMetrics(ctx)
			oldState := oldCms.state

			buffered := false
			globalCfg.CustomMetrics[0].ExtraFields = append(globalCfg.CustomMetrics[0].ExtraFields, "new_field")
			flushAndMultiReports(globalCfg, &buffered)
			// This line makes the test flaky.
			//So(buffered, ShouldBeTrue)
			newCms := getCustomMetrics(ctx)
			newCms.m.RLock()
			So(newCms.state, ShouldNotEqual, oldState)
			newCms.m.RUnlock()
			So(len(oldCms.buf), ShouldEqual, 0)
			So(len(newCms.buf), ShouldEqual, 0)
		})
	})
}
