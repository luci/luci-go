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

package metrics

import (
	"math"
	"time"

	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
)

// Internal contains a collection of metric definitions internal to LUCI CV.
var Internal = struct {
	BuildbucketRPCCount     metric.Counter
	BuildbucketRPCDurations metric.CumulativeDistribution
}{
	BuildbucketRPCCount: metric.NewCounter(
		"cv/internal/buildbucket_rpc/count",
		"Total number of RPCs to Buildbucket.",
		nil,

		field.String("project"),
		field.String("host"),
		field.String("method"),
		field.String("canonical_code"), // grpc.Code of the result as string in UPPER_CASE.
	),
	BuildbucketRPCDurations: metric.NewCumulativeDistribution(
		"cv/internal/buildbucket_rpc/durations",
		"Distribution of RPC duration (in milliseconds).",
		&types.MetricMetadata{Units: types.Milliseconds},
		// Bucketer for 1ms..10m range since CV isn't going to wait longer than 10m
		// anyway.
		distribution.GeometricBucketer(math.Pow(float64(10*time.Minute/time.Millisecond), 1.0/numBucket), numBucket),

		field.String("project"),
		field.String("host"),
		field.String("method"),
		field.String("canonical_code"), // grpc.Code of the result as string in UPPER_CASE.
	),
}
