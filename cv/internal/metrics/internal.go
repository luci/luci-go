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
	CLIngestionAttempted    metric.Counter
	CLIngestionLatency      metric.CumulativeDistribution
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
		distribution.GeometricBucketer(math.Pow(float64(10*time.Minute/time.Millisecond), 1.0/nBuckets), nBuckets),

		field.String("project"),
		field.String("host"),
		field.String("method"),
		field.String("canonical_code"), // grpc.Code of the result as string in UPPER_CASE.
	),

	CLIngestionAttempted: metric.NewCounter(
		"cv/internal/changelist/ingestion_attempted",
		"Occurrences of CL updates by processing UpdateCLTask with an actual "+
			"fetch operation in the updater backend",
		nil,
		field.String("requester"),
		// Whether the CL Update actually mutates the CL entry.
		// If false, it's either
		// - the CL Update wasn't necessary
		// - Gerrit API returned stale data
		field.Bool("changed"),
		// True if the ingestion was to retrieve the snapshot of a dep CL.
		field.Bool("dep"),
	),
	CLIngestionLatency: metric.NewCumulativeDistribution(
		"cv/internal/changelist/ingestion_latency",
		"Distribution of the time elapsed "+
			"from the time of a Gerrit update event occurrence "+
			"to the time of the snapshot ingested in CV",
		&types.MetricMetadata{Units: types.Seconds},
		// Bucketer for 1s...8h range since anything above 8h is too bad.
		distribution.GeometricBucketer(
			math.Pow(float64(8*time.Hour/time.Second), 1.0/nBuckets), nBuckets,
		),
		field.String("requester"),
		field.Bool("dep"),
	),
}
