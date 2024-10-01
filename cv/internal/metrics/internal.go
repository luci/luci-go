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
	BuildbucketRPCCount            metric.Counter
	BuildbucketRPCDurations        metric.CumulativeDistribution
	CLIngestionAttempted           metric.Counter
	CLIngestionLatency             metric.CumulativeDistribution
	CLIngestionLatencyWithoutFetch metric.CumulativeDistribution
	CLTriggererTaskCompleted       metric.Counter
	CLTriggererTaskDuration        metric.CumulativeDistribution
	BigQueryExportDelay            metric.CumulativeDistribution
	RunTryjobResultReportDelay     metric.CumulativeDistribution
	RunResetTriggerAttempted       metric.Counter
	QuotaOp                        metric.Counter
	CreateToFirstTryjobLatency     metric.CumulativeDistribution
	StartToFirstTryjobLatency      metric.CumulativeDistribution
}{
	BuildbucketRPCCount: metric.NewCounter(
		"cv/internal/buildbucket_rpc/count",
		"Total number of RPCs to Buildbucket.",
		nil,
		field.String("project"),
		field.String("host"),
		field.String("method"),
		field.String("canonical_code"), // status.Code of the result as string in UPPER_CASE.
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
		field.String("canonical_code"), // status.Code of the result as string in UPPER_CASE.
	),
	CLIngestionAttempted: metric.NewCounter(
		"cv/internal/changelist/ingestion_attempted",
		"Occurrences of CL updates by processing UpdateCLTask with an actual "+
			"fetch operation in the updater backend",
		nil,
		field.String("requester"),
		// Whether the CL ingestion mutated the CL entry.
		// If false, it's either
		// - the CL Update wasn't necessary
		// - Gerrit API returned stale data
		field.Bool("changed"),
		// True if the ingestion was to retrieve the snapshot of a dep CL.
		field.Bool("dep"),
		// The LUCI project
		field.String("project"),
		// Whether the CL ingestion mutated the snapshot in the CL entity.
		field.Bool("changed_snapshot"),
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
		field.String("project"),
		field.Bool("changed_snapshot"),
	),
	CLIngestionLatencyWithoutFetch: metric.NewCumulativeDistribution(
		"cv/internal/changelist/ingestion_latency_without_fetch",
		"Distribution of the time elapsed "+
			"from the time of a Gerrit update event occurrence "+
			"to the time of the snapshot ingested in CV, but excluding "+
			"the time taken to fetch the snapshot from the backend",
		&types.MetricMetadata{Units: types.Seconds},
		// Bucketer for 1s...8h range since anything above 8h is too bad.
		distribution.GeometricBucketer(
			math.Pow(float64(8*time.Hour/time.Second), 1.0/nBuckets), nBuckets,
		),
		field.String("requester"),
		field.Bool("dep"),
		field.String("project"),
		field.Bool("changed_snapshot"),
	),
	CLTriggererTaskCompleted: metric.NewCounter(
		"cv/internal/cltriggerer/tasks/completed",
		"Count of Chained CQ vote tasks completed",
		nil,
		// LUCI project
		field.String("project"),
		// Config Group name
		field.String("config_group"),
		// # of deps to trigger
		field.Int("num_deps"),
		// Status - skipped | succeeded | failed | canceled
		field.String("status"),
	),
	CLTriggererTaskDuration: metric.NewCumulativeDistribution(
		"cv/internal/cltriggerer/tasks/duration",
		"Distribution of processing time for chained CQ vote processes",
		&types.MetricMetadata{Units: types.Milliseconds},
		// Bucketer for 1ms...20m range.
		distribution.GeometricBucketer(
			math.Pow(float64(20*time.Minute/time.Millisecond), 1.0/nBuckets), nBuckets,
		),
		// LUCI project
		field.String("project"),
		// Config Group name
		field.String("config_group"),
		// # of deps to trigger
		field.Int("num_deps"),
		// Status - skipped | succeeded | failed | canceled
		field.String("status"),
	),
	BigQueryExportDelay: metric.NewCumulativeDistribution(
		"cv/internal/runs/bq_export_delay",
		"Distribution of the time elapsed from the time a Run ends to the "+
			"time CV exports this Run to BigQuery",
		&types.MetricMetadata{Units: types.Milliseconds},
		// Bucketer for 1ms...8h range.
		distribution.GeometricBucketer(
			math.Pow(float64(8*time.Hour/time.Millisecond), 1.0/nBuckets), nBuckets,
		),
		field.String("project"),
		field.String("config_group"),
		field.String("mode"),
	),
	RunTryjobResultReportDelay: metric.NewCumulativeDistribution(
		"cv/internal/runs/tryjob_result_report_delay",
		"Distribution of the time elapsed from the time Run Tryjob execution has "+
			"completed to the time LUCI CV successfully reports the result to the CL",
		&types.MetricMetadata{Units: types.Milliseconds},
		// Bucketer for 1ms...8h range.
		distribution.GeometricBucketer(
			math.Pow(float64(8*time.Hour/time.Millisecond), 1.0/nBuckets), nBuckets,
		),
		field.String("project"),
		field.String("config_group"),
		field.String("mode"),
	),
	RunResetTriggerAttempted: metric.NewCounter(
		"cv/internal/runs/reset_trigger_attempted",
		"Record the number of attempts to reset the triggers of Run",
		nil,
		field.String("project"),
		field.String("config_group"),
		field.String("mode"),
		field.Bool("succeeded"),
		field.String("gerrit_error"),
	),
	QuotaOp: metric.NewCounter(
		"cv/internal/quota/op",
		"Count of server quota operation",
		nil,
		field.String("project"),
		field.String("policy_namespace"),
		field.String("policy_name"),
		field.String("policy_resource"),
		field.String("op"),
		field.String("status"),
	),
	CreateToFirstTryjobLatency: metric.NewCumulativeDistribution(
		"cv/internal/runs/create_to_first_tryjob_latency",
		"Time elapsed from the Run creation to the first successful tryjob launch "+
			"time. Runs that do not successfully launch any Tryjob are not reported. "+
			"It's possible due to tryjob reuse, launch failure or 0 tryjob configured.",
		&types.MetricMetadata{Units: types.Milliseconds},
		// Bucketer for 1ms...1h range.
		distribution.GeometricBucketer(
			math.Pow(float64(1*time.Hour/time.Millisecond), 1.0/nBuckets), nBuckets,
		),
		field.String("project"),
		field.String("config_group"),
		field.String("mode"),
	),
	StartToFirstTryjobLatency: metric.NewCumulativeDistribution(
		"cv/internal/runs/start_to_first_tryjob_latency",
		"Time elapsed from the Run start to the first successful tryjob launch "+
			"time. Runs that do not successfully launch any Tryjob are not reported. "+
			"It's possible due to tryjob reuse, launch failure or 0 tryjob configured.",
		&types.MetricMetadata{Units: types.Milliseconds},
		// Bucketer for 1ms...1h range.
		distribution.GeometricBucketer(
			math.Pow(float64(1*time.Hour/time.Millisecond), 1.0/nBuckets), nBuckets,
		),
		field.String("project"),
		field.String("config_group"),
		field.String("mode"),
	),
}
