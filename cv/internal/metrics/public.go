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

	bbmetrics "go.chromium.org/luci/buildbucket/metrics"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
)

const nBuckets = 100

// Public contains a collection of public LUCI CV metric definitions.
var Public = struct {
	RunCreated         metric.Counter
	RunStarted         metric.Counter
	RunEnded           metric.Counter
	RunDuration        metric.CumulativeDistribution
	RunTotalDuration   metric.CumulativeDistribution
	ActiveRunCount     metric.Int
	ActiveRunDuration  metric.NonCumulativeDistribution
	PendingRunCount    metric.Int
	PendingRunDuration metric.NonCumulativeDistribution
	MaxPendingRunAge   metric.Int

	TryjobLaunched        metric.Counter
	TryjobEnded           metric.Counter
	TryjobBuilderPresence metric.Bool

	RunQuotaRejection metric.Counter
}{
	RunCreated: metric.NewCounter(
		"cv/runs/created",
		"Count of the newly created Runs",
		nil,
		field.String("project"),
		field.String("config_group"),
		field.String("mode"),
	),
	RunStarted: metric.NewCounter(
		"cv/runs/started",
		"Count of the started Runs",
		nil,
		field.String("project"),
		field.String("config_group"),
		field.String("mode"),
	),
	RunEnded: metric.NewCounter(
		"cv/runs/ended",
		"Count of the ended Runs",
		nil,
		field.String("project"),
		field.String("config_group"),
		field.String("mode"),
		field.String("status"),
		field.Bool("successfully_started"),
	),
	RunDuration: metric.NewCumulativeDistribution(
		"cv/runs/ended/durations",
		"The distribution of time elapsed from Run start to Run end",
		&types.MetricMetadata{Units: types.Seconds},
		// Bucketer for 1s .. 2d range.
		distribution.GeometricBucketer(math.Pow(float64(2*24*time.Hour/time.Second), 1.0/nBuckets), nBuckets),
		field.String("project"),
		field.String("config_group"),
		field.String("mode"),
		field.String("status"),
	),
	RunTotalDuration: metric.NewCumulativeDistribution(
		"cv/runs/ended/total_durations",
		"The distribution of time elapsed from Run creation to Run end",
		&types.MetricMetadata{Units: types.Seconds},
		// Bucketer for 1s .. 2d range.
		distribution.GeometricBucketer(math.Pow(float64(2*24*time.Hour/time.Second), 1.0/nBuckets), nBuckets),
		field.String("project"),
		field.String("config_group"),
		field.String("mode"),
		field.String("status"),
		field.Bool("successfully_started"),
	),
	ActiveRunCount: metric.NewInt(
		"cv/runs/active",
		"The current count of the Runs that are active (started but not ended yet)",
		nil,
		field.String("project"),
		field.String("config_group"),
		field.String("mode"),
	),
	ActiveRunDuration: metric.NewNonCumulativeDistribution(
		"cv/runs/active/durations",
		"The distribution of all the currently active run durations (started but not ended yet)",
		&types.MetricMetadata{Units: types.Seconds},
		// Bucketer for 1s .. 2d range.
		distribution.GeometricBucketer(math.Pow(float64(2*24*time.Hour/time.Second), 1.0/nBuckets), nBuckets),
		field.String("project"),
		field.String("config_group"),
		field.String("mode"),
	),
	PendingRunCount: metric.NewInt(
		"cv/runs/pending",
		"The current count of the Runs that are pending (created but not started yet)",
		nil,
		field.String("project"),
		field.String("config_group"),
		field.String("mode"),
	),
	PendingRunDuration: metric.NewNonCumulativeDistribution(
		"cv/runs/pending/durations",
		"The distribution of all the currently pending Run durations (created but not started yet)",
		&types.MetricMetadata{Units: types.Milliseconds},
		// Bucketer for 1ms .. 2h range.
		distribution.GeometricBucketer(math.Pow(float64(2*time.Hour/time.Millisecond), 1.0/nBuckets), nBuckets),
		field.String("project"),
		field.String("config_group"),
		field.String("mode"),
	),
	MaxPendingRunAge: metric.NewInt(
		"cv/runs/max_pending_age",
		"The age of the oldest run that has been created but not started yet (aka age of the oldest pending run)",
		&types.MetricMetadata{Units: types.Milliseconds},
		field.String("project"),
		field.String("config_group"),
		field.String("mode"),
	),
	TryjobLaunched: metric.NewCounterWithTargetType(
		"cv/tryjobs/launched",
		((*bbmetrics.BuilderTarget)(nil)).Type(),
		"Count of Tryjobs launched by LUCI CV",
		nil,
		field.String("cv_project"),
		field.String("config_group"),
		field.Bool("critical"),
		field.Bool("retry"),
	),
	TryjobEnded: metric.NewCounterWithTargetType(
		"cv/tryjobs/ended",
		((*bbmetrics.BuilderTarget)(nil)).Type(),
		"Count of Tryjobs launched by LUCI CV that have ended",
		nil,
		field.String("cv_project"),
		field.String("config_group"),
		field.Bool("critical"),
		field.Bool("retry"),
		field.String("status"),
	),
	TryjobBuilderPresence: metric.NewBoolWithTargetType(
		"cv/tryjobs/builders/presence",
		((*bbmetrics.BuilderTarget)(nil)).Type(),
		"Provides a list of configured builders in the Project config",
		nil,
		field.String("cv_project"),
		field.String("config_group"),
		field.Bool("includable_only"),
		field.Bool("path_cond"),
		field.Bool("experimental"),
	),
	RunQuotaRejection: metric.NewCounter(
		"cv/runs/quota/rejected",
		"Count of run rejection",
		nil,
		field.String("project"),
		field.String("config_group"),
		field.String("gerrit_account_id"), // `{gerrit_host}/{account_id}`.
	),
}
