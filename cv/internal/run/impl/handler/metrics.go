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

package handler

import (
	"math"

	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
)

var (
	metricPickupLatencyS = metric.NewCumulativeDistribution(
		"cv/pickup/latency",
		"Time between triggering CV and when CV processing actually starts",
		&types.MetricMetadata{Units: types.Seconds},
		// Bucketer for 1s..2.5h range.
		//
		// $ python3 -c "print(((10**0.04)**100)/60.0/60.0)"
		// 2.7777777777777977
		distribution.GeometricBucketer(math.Pow(10, 0.04), 100),
		field.String("project"),
	)
	metricPickupLatencyAdjustedS = metric.NewCumulativeDistribution(
		"cv/pickup/latency_adjusted",
		"Time between triggering CV and when CV processing actually starts but adjusted by the configured stabilization delay",
		&types.MetricMetadata{Units: types.Seconds},
		distribution.GeometricBucketer(math.Pow(10, 0.04), 100),
		field.String("project"),
	)
)
