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

package eventbox

import (
	"context"
	"math"

	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
)

var (
	metricListDurationsS = metric.NewCumulativeDistribution(
		"cv/internal/eventbox/list/duration",
		"Duration of a list op.",
		&types.MetricMetadata{Units: types.Milliseconds},
		// Bucketer for 1ms..10m range since anything above 10m is bad.
		//
		// $ python3 -c "print(((10**0.058)**100)/1e3/60.0)"
		// 10.515955741336601
		distribution.GeometricBucketer(math.Pow(10, 0.058), 100),
		field.String("recipient"),
		field.String("result"),
	)

	metricSent = metric.NewCounter(
		"cv/internal/eventbox/sent",
		"Number of events sent.",
		nil,
		field.String("recipient"),
	)

	metricRemoved = metric.NewCounter(
		"cv/internal/eventbox/removed",
		"Approximate and likely exaggerated number of events removed.",
		nil,
		field.String("recipient"),
	)

	metricSize = metric.NewInt(
		"cv/internal/eventbox/size",
		"Number of the events. Updated from successful list ops only.",
		nil,
		field.String("recipient"),
	)

	metricOldestAgeS = metric.NewFloat(
		"cv/internal/eventbox/oldest_age",
		"Age of the oldest event. Updated from successful list ops only.",
		&types.MetricMetadata{Units: types.Seconds},
		field.String("recipient"),
	)
)

func monitoringResult(err error) string {
	switch {
	case err == nil:
		return "OK"
	case err == context.DeadlineExceeded:
		return "TIMEOUT"
	default:
		return "FAILURE"
	}
}
