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
		fields: "os",
		metric_base: CUSTOM_BUILD_METRIC_BASE_CREATED,
	}
	custom_metrics {
		name: "/chrome/infra/custom/builds/started",
		fields: "os",
		metric_base: CUSTOM_BUILD_METRIC_BASE_STARTED,
	}
	custom_metrics {
		name: "/chrome/infra/custom/builds/completed",
		fields: "os",
		metric_base: CUSTOM_BUILD_METRIC_BASE_COMPLETED,
	}
	custom_metrics {
		name: "/chrome/infra/custom/builds/run_duration",
		fields: "os",
		metric_base: CUSTOM_BUILD_METRIC_BASE_RUN_DURATIONS,
	}
	custom_metrics {
		name: "/chrome/infra/custom/builds/max_age",
		fields: "os",
		metric_base: CUSTOM_BUILD_METRIC_BASE_MAX_AGE_SCHEDULED,
	}
	custom_metrics {
		name: "/chrome/infra/custom/builds/count",
		fields: "os",
		metric_base: CUSTOM_BUILD_METRIC_BASE_COUNT,
	}
`

func init() {
	testContext = resetCustomMetrics(context.Background())
}

func resetCustomMetrics(ctx context.Context) context.Context {
	cfg := &pb.SettingsCfg{}
	_ = prototext.Unmarshal([]byte(cfgContent), cfg)
	ctx, _ = WithCustomMetrics(ctx, cfg)
	return ctx
}

func TestWithCustomMetrics(t *testing.T) {
	ctx, _ := tsmon.WithDummyInMemory(testContext)

	Convey("WithCustomMetrics", t, func() {
		globalCfg := &pb.SettingsCfg{}
		_ = prototext.Unmarshal([]byte(cfgContent), globalCfg)

		Convey("check metrics from test context", func() {
			cms := getCustomMetrics(ctx)
			v2Custom := cms.metrics
			startM := v2Custom[pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_STARTED]["/chrome/infra/custom/builds/started"]
			_, ok := startM.(*counter)
			So(ok, ShouldBeTrue)
			runDurationM := v2Custom[pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_RUN_DURATIONS]["/chrome/infra/custom/builds/run_duration"]
			_, ok = runDurationM.(*cumulativeDistribution)
			So(ok, ShouldBeTrue)
			maxAgeM := v2Custom[pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_MAX_AGE_SCHEDULED]["/chrome/infra/custom/builds/max_age"]
			_, ok = maxAgeM.(*float)
			So(ok, ShouldBeTrue)
			countM := v2Custom[pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_COUNT]["/chrome/infra/custom/builds/count"]
			_, ok = countM.(*int)
			So(ok, ShouldBeTrue)
		})
	})
}
