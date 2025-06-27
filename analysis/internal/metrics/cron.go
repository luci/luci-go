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
	"context"
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/ingestion/control"
)

var (
	activeRulesGauge = metric.NewInt(
		"analysis/clustering/active_rules",
		"The total number of active rules, by LUCI project.",
		&types.MetricMetadata{Units: "rules"},
		// The LUCI Project.
		field.String("project"))

	presubmitToBuildJoinGauge = metric.NewNonCumulativeDistribution(
		"analysis/ingestion/join/presubmit_to_build_result_by_hour",
		fmt.Sprintf(
			"The age distribution of presubmit builds with a presubmit"+
				" result recorded, broken down by project of the presubmit "+
				" run and whether the builds are joined to a buildbucket "+
				" build result."+
				" Age is measured as hours since the presubmit run result was"+
				" recorded. Only recent data (age < %v hours) is included."+
				" Used to measure LUCI Analysis's performance joining"+
				" presubmit runs to buildbucket builds.", control.JoinStatsHours),
		&types.MetricMetadata{Units: "hours ago"},
		distribution.FixedWidthBucketer(1, control.JoinStatsHours),
		// The LUCI Project of the presubmit run.
		field.String("project"),
		field.Bool("joined"))

	buildToPresubmitJoinGauge = metric.NewNonCumulativeDistribution(
		"analysis/ingestion/join/build_to_presubmit_result_by_hour",
		fmt.Sprintf(
			"The age distribution of presubmit builds with a buildbucket"+
				" build result recorded, broken down by project of the"+
				" buildbucket build and whether the builds are joined to"+
				" a presubmit run result."+
				" Age is measured as hours since the buildbucket build"+
				" result was recorded. Only recent data (age < %v hours)"+
				" is included."+
				" Used to measure LUCI Analysis's performance joining"+
				" builds to presubmit runs.", control.JoinStatsHours),
		&types.MetricMetadata{Units: "hours ago"},
		distribution.FixedWidthBucketer(1, control.JoinStatsHours),
		// The LUCI Project of the buildbucket run.
		field.String("project"),
		field.Bool("joined"))

	invocationToBuildJoinGauge = metric.NewNonCumulativeDistribution(
		"analysis/ingestion/join/invocation_to_build_result_by_hour",
		fmt.Sprintf(
			"The age distribution of builds with a finalized invocation"+
				" recorded, broken down by project of the invocation "+
				" and whether the builds are joined to a buildbucket "+
				" build result."+
				" Age is measured as hours since the finalized invocation was"+
				" recorded. Only recent data (age < %v hours) is included."+
				" Used to measure LUCI Analysis's performance joining"+
				" invocations to buildbucket builds.", control.JoinStatsHours),
		&types.MetricMetadata{Units: "hours ago"},
		distribution.FixedWidthBucketer(1, control.JoinStatsHours),
		// The LUCI Project of the presubmit run.
		field.String("project"),
		field.Bool("joined"))

	buildToInvocationJoinGauge = metric.NewNonCumulativeDistribution(
		"analysis/ingestion/join/build_to_invocation_result_by_hour",
		fmt.Sprintf(
			"The age distribution of builds with a ResultDB invocation"+
				" recorded, broken down by project of the"+
				" buildbucket build and whether the builds are joined to"+
				" a finalized invocation."+
				" Age is measured as hours since the buildbucket build"+
				" result was recorded. Only recent data (age < %v hours)"+
				" is included."+
				" Used to measure LUCI Analysis's performance joining"+
				" builds to invocations.", control.JoinStatsHours),
		&types.MetricMetadata{Units: "hours ago"},
		distribution.FixedWidthBucketer(1, control.JoinStatsHours),
		// The LUCI Project of the buildbucket run.
		field.String("project"),
		field.Bool("joined"))
)

func init() {
	// Register metrics as global metrics, which has the effort of
	// resetting them after every flush.
	tsmon.RegisterGlobalCallback(func(ctx context.Context) {
		// Do nothing -- the metrics will be populated by the cron
		// job itself and does not need to be triggered externally.
	}, activeRulesGauge, presubmitToBuildJoinGauge, buildToPresubmitJoinGauge,
		invocationToBuildJoinGauge, buildToInvocationJoinGauge)
}

// GlobalMetrics handles the "global-metrics" cron job. It reports
// metrics related to overall system state (that are not logically
// reported as part of individual task or cron job executions).
func GlobalMetrics(ctx context.Context) error {
	projectConfigs, err := config.Projects(ctx)
	if err != nil {
		return errors.Fmt("obtain project configs: %w", err)
	}

	// Total number of active rules, broken down by project.
	activeRules, err := rules.ReadTotalActiveRules(span.Single(ctx))
	if err != nil {
		return errors.Fmt("collect total active rules: %w", err)
	}
	for _, project := range projectConfigs.Keys() {
		// If there is no entry in activeRules for this project
		// (e.g. because there are no rules in that project),
		// the read count defaults to zero, which is the correct
		// behaviour.
		count := activeRules[project]
		activeRulesGauge.Set(ctx, count, project)
	}

	// Performance joining presubmit runs to buildbucket builds in ingestion.
	psToBuildJoinStats, err := control.ReadPresubmitToBuildJoinStatistics(span.Single(ctx))
	if err != nil {
		return errors.Fmt("collect presubmit run to buildbucket build join statistics: %w", err)
	}
	reportJoinStats(ctx, presubmitToBuildJoinGauge, psToBuildJoinStats)

	// Performance joining buildbucket builds to presubmit runs in ingestion.
	buildToPSJoinStats, err := control.ReadBuildToPresubmitRunJoinStatistics(span.Single(ctx))
	if err != nil {
		return errors.Fmt("collect buildbucket build to presubmit run join statistics: %w", err)
	}
	reportJoinStats(ctx, buildToPresubmitJoinGauge, buildToPSJoinStats)

	// Performance joining finalized invocations to buildbucket builds in ingestion.
	invToBuildJoinStats, err := control.ReadInvocationToBuildJoinStatistics(span.Single(ctx))
	if err != nil {
		return errors.Fmt("collect invocation to buildbucket build join statistics: %w", err)
	}
	reportJoinStats(ctx, invocationToBuildJoinGauge, invToBuildJoinStats)

	// Performance joining buildbucket builds to finalized invocations in ingestion.
	buildToInvJoinStats, err := control.ReadBuildToInvocationJoinStatistics(span.Single(ctx))
	if err != nil {
		return errors.Fmt("collect buildbucket build to invocation join statistics: %w", err)
	}
	reportJoinStats(ctx, buildToInvocationJoinGauge, buildToInvJoinStats)

	return nil
}

func reportJoinStats(ctx context.Context, metric metric.NonCumulativeDistribution, resultsByProject map[string]control.JoinStatistics) {
	for project, stats := range resultsByProject {
		joinedDist := distribution.New(metric.Bucketer())
		unjoinedDist := distribution.New(metric.Bucketer())

		for hoursAgo := range control.JoinStatsHours {
			joinedBuilds := stats.JoinedByHour[hoursAgo]
			unjoinedBuilds := stats.TotalByHour[hoursAgo] - joinedBuilds
			for range joinedBuilds {
				joinedDist.Add(float64(hoursAgo))
			}
			for range unjoinedBuilds {
				unjoinedDist.Add(float64(hoursAgo))
			}
		}

		joined := true
		metric.Set(ctx, joinedDist, project, joined)
		joined = false
		metric.Set(ctx, unjoinedDist, project, joined)
	}
}
