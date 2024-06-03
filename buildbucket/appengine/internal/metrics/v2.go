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
	"math"

	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"

	bbmetrics "go.chromium.org/luci/buildbucket/metrics"
)

var (
	opt = &metric.Options{
		TargetType: (&bbmetrics.BuilderTarget{}).Type(),
	}
	// V2 is a collection of metric objects for V2 metrics.
	V2 = struct {
		BuildCount              metric.Int
		BuildCountCreated       metric.Counter
		BuildCountStarted       metric.Counter
		BuildCountCompleted     metric.Counter
		BuildDurationCycle      metric.CumulativeDistribution
		BuildDurationRun        metric.CumulativeDistribution
		BuildDurationScheduling metric.CumulativeDistribution
		BuilderPresence         metric.Bool
		ConsecutiveFailureCount metric.Int
		MaxAgeScheduled         metric.Float
	}{
		BuildCount: metric.NewIntWithOptions(
			"buildbucket/v2/builds/count",
			opt,
			"Number of pending/running prod builds",
			nil,
			field.String("status"),
		),
		BuildCountCreated: metric.NewCounterWithOptions(
			"buildbucket/v2/builds/created",
			opt,
			"Build creation",
			nil,
			field.String("experiments"),
		),
		BuildCountStarted: metric.NewCounterWithOptions(
			"buildbucket/v2/builds/started",
			opt,
			"Build start",
			nil,
			field.String("experiments"),
		),
		BuildCountCompleted: metric.NewCounterWithOptions(
			"buildbucket/v2/builds/completed",
			opt,
			"Build completion, including success, failure and cancellation",
			nil,
			field.String("status"),
			field.String("experiments"),
		),
		BuildDurationCycle: metric.NewCumulativeDistributionWithOptions(
			"buildbucket/v2/builds/cycle_durations",
			opt,
			"Duration between build creation and completion",
			&types.MetricMetadata{Units: types.Seconds},
			// Bucketer for 1s..48h range
			//
			// python3 -c "print(((10**0.053)**100) / (60*60))"
			// 55.42395319358006
			distribution.GeometricBucketer(math.Pow(10, 0.053), 100),
			field.String("status"),
			field.String("experiments"),
		),
		BuildDurationRun: metric.NewCumulativeDistributionWithOptions(
			"buildbucket/v2/builds/run_durations",
			opt,
			"Duration between build start and completion",
			&types.MetricMetadata{Units: types.Seconds},
			// Bucketer for 1s..48h range
			distribution.GeometricBucketer(math.Pow(10, 0.053), 100),
			field.String("status"),
			field.String("experiments"),
		),
		BuildDurationScheduling: metric.NewCumulativeDistributionWithOptions(
			"buildbucket/v2/builds/scheduling_durations",
			opt,
			"Duration between build creation and start",
			&types.MetricMetadata{Units: types.Seconds},
			// Bucketer for 1s..48h range
			distribution.GeometricBucketer(math.Pow(10, 0.053), 100),
			field.String("experiments"),
		),
		BuilderPresence: metric.NewBoolWithOptions(
			"buildbucket/v2/builder/presence",
			opt,
			"A constant, always-true metric that indicates the presence of LUCI Builder",
			nil,
		),
		ConsecutiveFailureCount: metric.NewIntWithOptions(
			"buildbucket/v2/builds/consecutive_failure_count",
			opt,
			"Number of consecutive non-successful build terminations since the last successful build.",
			nil,
			field.String("status"),
		),
		MaxAgeScheduled: metric.NewFloatWithOptions(
			"buildbucket/v2/builds/max_age_scheduled",
			opt,
			"Age of the oldest SCHEDULED build",
			&types.MetricMetadata{Units: types.Seconds},
		),
	}
)
