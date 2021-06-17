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
	"fmt"
	"math"

	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/gae/service/datastore"
)

var metricListingDurationsMS = metric.NewCumulativeDistribution(
	"cv/internal/eventbox/listing/durations",
	"Duration of listing incoming events before transacting on a recipient.",
	&types.MetricMetadata{Units: types.Milliseconds},
	// Bucketer for 1ms..10m range since anything above 10m is bad.
	//
	// $ python3 -c "print(((10**0.058)**100)/1e3/60.0)"
	// 10.515955741336601
	distribution.GeometricBucketer(math.Pow(10, 0.058), 100),
	field.String("recipient"),
	field.String("result"),
)

var metricListingCounts = metric.NewCumulativeDistribution(
	"cv/internal/eventbox/listing/count",
	"Count of the incoming events listed before transacting on a recipient. Updated only on successful listing.",
	nil,
	// Bucketer for 1..10^6 range since anything above that shouldn't be listed
	// at once and probably won't be listed before a timeout anyway.
	//
	// $ python3 -c "print(((10**0.06)**100)/1e6)"
	// 1.0000000000000064
	distribution.GeometricBucketer(math.Pow(10, 0.06), 100),
	field.String("recipient"),
)

var metricListingOldestAgeS = metric.NewFloat(
	"cv/internal/eventbox/listing/oldest",
	"Age of the oldest listed event. Updated only on successful listing.",
	&types.MetricMetadata{Units: types.Seconds},
	field.String("recipient"),
)

func monitoringRecipient(r *datastore.Key) string {
	return fmt.Sprintf("%s/%s", r.Kind(), r.StringID())
}

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
