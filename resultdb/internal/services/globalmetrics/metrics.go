// Copyright 2020 The LUCI Authors.
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

// Package globalmetrics reports metrics that are computationally heavy.
// There must be a single replica of globalmetrics server.
package globalmetrics

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/cron"
	"go.chromium.org/luci/resultdb/internal/spanutil"
)

var (
	oldestExpiredResultMetric = metric.NewInt(
		"resultdb/oldest_expired_result",
		"Unix timestamp of the earliest result not yet purged",
		nil)
	expiredResultsPendingInvocationCount = metric.NewInt(
		"resultdb/expired_results/pending_invocations",
		"Number of pending invocations where expired results were not yet purged",
		nil)
	spannerTestResultsSizeMetrics = metric.NewInt(
		"resultdb/spanner/test_results/sizes",
		"Total size of various columns in the TestResults table",
		&types.MetricMetadata{Units: types.Bytes},
		field.String("project"),
		field.String("column"),
	)
	spannerUnexpectedTestResultsSizeMetrics = metric.NewInt(
		"resultdb/spanner/unexpected_test_results/sizes",
		"Total size of various columns in the UnexpectedTestResults index",
		&types.MetricMetadata{Units: types.Bytes},
		field.String("project"),
		field.String("column"),
	)
)

func init() {
	// Register metrics as global metrics, which has the effort of
	// resetting them after every flush.
	tsmon.RegisterGlobalCallback(func(ctx context.Context) {
		// Do nothing -- the metrics will be populated by the cron
		// job itself and does not need to be triggered externally.
	}, oldestExpiredResultMetric, expiredResultsPendingInvocationCount, spannerTestResultsSizeMetrics, spannerUnexpectedTestResultsSizeMetrics)
}

// Options is global metrics server configuration.
type Options struct {
	// UpdateInterval is how often to update metrics.
	UpdateInterval time.Duration
}

// InitServer initializes a backend server.
func InitServer(srv *server.Server, opts Options) {
	interval := opts.UpdateInterval
	if interval == 0 {
		interval = 5 * time.Minute
	}

	srv.RunInBackground("resultdb.oldest_expired_result", func(ctx context.Context) {
		cron.Run(ctx, interval, updateExpiredResultsMetrics)
	})
	srv.RunInBackground("resultdb.spanner_disk_usage", func(ctx context.Context) {
		cron.Run(ctx, interval, updateSpannerTestResultsSizeMetrics)
	})
}

func updateExpiredResultsMetrics(ctx context.Context) error {
	switch oldest, count, err := expiredResultStats(ctx); {
	case err == spanutil.ErrNoResults:
		return nil
	case err != nil:
		return err
	default:
		oldestExpiredResultMetric.Set(ctx, oldest.Unix())
		expiredResultsPendingInvocationCount.Set(ctx, count)
		return nil
	}
}

// expiredResultStats computes the creation time of the oldest invocation
// pending to be purged in seconds.
func expiredResultStats(ctx context.Context) (oldestResult time.Time, pendingInvocationsCount int64, err error) {
	var earliest spanner.NullTime
	st := spanner.NewStatement(`
		SELECT
			MIN(ExpectedTestResultsExpirationTime) as EarliestExpiration,
			COUNT(*) as pending_count
		FROM UNNEST(GENERATE_ARRAY(0, (
			SELECT MAX(ShardId)
			FROM Invocations@{FORCE_INDEX=InvocationsByExpectedTestResultsExpiration}
			WHERE ExpectedTestResultsExpirationTime IS NOT NULL
		))) AS TargetShard
		JOIN Invocations@{FORCE_INDEX=InvocationsByExpectedTestResultsExpiration}
			ON ShardId = TargetShard
		WHERE ExpectedTestResultsExpirationTime IS NOT NULL
			AND ExpectedTestResultsExpirationTime < CURRENT_TIMESTAMP()
	`)
	err = spanutil.QueryFirstRow(span.Single(ctx), st, &earliest, &pendingInvocationsCount)
	oldestResult = earliest.Time
	return
}

func updateSpannerTestResultsSizeMetrics(ctx context.Context) error {
	logging.Infof(ctx, "started updating TestResults spanner table size metrics")

	projectStats, err := spannerTestResultsStats(ctx)
	if err != nil {
		return errors.Fmt("failed to query the stats of the TestResults spanner table: %w", err)
	}

	for _, columnSizes := range projectStats {
		spannerTestResultsSizeMetrics.Set(ctx, columnSizes.InvocationID, columnSizes.Project, "InvocationId")
		spannerTestResultsSizeMetrics.Set(ctx, columnSizes.TestID, columnSizes.Project, "TestId")
		spannerTestResultsSizeMetrics.Set(ctx, columnSizes.ResultID, columnSizes.Project, "ResultId")
		spannerTestResultsSizeMetrics.Set(ctx, columnSizes.Variant, columnSizes.Project, "Variant")
		spannerTestResultsSizeMetrics.Set(ctx, columnSizes.VariantHash, columnSizes.Project, "VariantHash")
		spannerTestResultsSizeMetrics.Set(ctx, columnSizes.CommitTimestamp, columnSizes.Project, "CommitTimestamp")
		spannerTestResultsSizeMetrics.Set(ctx, columnSizes.IsUnexpected, columnSizes.Project, "IsUnexpected")
		spannerTestResultsSizeMetrics.Set(ctx, columnSizes.Status, columnSizes.Project, "Status")
		spannerTestResultsSizeMetrics.Set(ctx, columnSizes.SummaryHTML, columnSizes.Project, "SummaryHTML")
		spannerTestResultsSizeMetrics.Set(ctx, columnSizes.StartTime, columnSizes.Project, "StartTime")
		spannerTestResultsSizeMetrics.Set(ctx, columnSizes.RunDurationUsec, columnSizes.Project, "RunDurationUsec")
		spannerTestResultsSizeMetrics.Set(ctx, columnSizes.Tags, columnSizes.Project, "Tags")
		spannerTestResultsSizeMetrics.Set(ctx, columnSizes.TestMetadata, columnSizes.Project, "TestMetadata")
		spannerTestResultsSizeMetrics.Set(ctx, columnSizes.FailureReason, columnSizes.Project, "FailureReason")
		spannerTestResultsSizeMetrics.Set(ctx, columnSizes.Properties, columnSizes.Project, "Properties")

		spannerUnexpectedTestResultsSizeMetrics.Set(ctx, columnSizes.UnexpectedTestResultsInvocationID, columnSizes.Project, "InvocationId")
		spannerUnexpectedTestResultsSizeMetrics.Set(ctx, columnSizes.UnexpectedTestResultsTestID, columnSizes.Project, "TestId")
		spannerUnexpectedTestResultsSizeMetrics.Set(ctx, columnSizes.UnexpectedTestResultsIsUnexpected, columnSizes.Project, "IsUnexpected")
		spannerUnexpectedTestResultsSizeMetrics.Set(ctx, columnSizes.UnexpectedTestResultsVariantHash, columnSizes.Project, "VariantHash")
		spannerUnexpectedTestResultsSizeMetrics.Set(ctx, columnSizes.UnexpectedTestResultsVariant, columnSizes.Project, "Variant")
	}

	logging.Infof(ctx, "finished updating TestResults spanner table size metrics")

	return nil
}

type testResultsColumnSizes struct {
	Project                           string
	InvocationID                      int64
	TestID                            int64
	ResultID                          int64
	Variant                           int64
	VariantHash                       int64
	CommitTimestamp                   int64
	IsUnexpected                      int64
	Status                            int64
	SummaryHTML                       int64
	StartTime                         int64
	RunDurationUsec                   int64
	Tags                              int64
	TestMetadata                      int64
	FailureReason                     int64
	Properties                        int64
	UnexpectedTestResultsInvocationID int64
	UnexpectedTestResultsTestID       int64
	UnexpectedTestResultsIsUnexpected int64
	UnexpectedTestResultsVariantHash  int64
	UnexpectedTestResultsVariant      int64
}

// spannerTestResultsStats computes the size of each column in the TestResults
// spanner table, broken down by projects.
func spannerTestResultsStats(ctx context.Context) (projectStats []testResultsColumnSizes, err error) {
	st := spanner.NewStatement(`
		WITH test_result_sizes AS (
			SELECT
				InvocationId,
				Realm,
				IsUnexpected,
				(LENGTH(InvocationId) + 8) AS InvocationIdSize,
				(LENGTH(TestId) + 8) AS TestIdSize,
				(LENGTH(ResultId) + 8) AS ResultIdSize,
				(IF(Variant IS NULL, 0, LENGTH(ARRAY_TO_STRING(Variant, '')) + ARRAY_LENGTH(Variant) * 8 + 8)) AS VariantSize,
				(LENGTH(VariantHash)) AS VariantHashSize,
				(12 + 8) AS CommitTimestampSize,
				(IF(IsUnexpected IS NULL, 0, 1 + 8)) AS IsUnexpectedSize,
				(8 + 8) AS StatusSize,
				(IF(SummaryHTML IS NULL, 0, LENGTH(SummaryHTML) + 8)) AS SummaryHTMLSize,
				(IF(StartTime IS NULL, 0, 12 + 8)) AS StartTimeSize,
				(IF(RunDurationUsec IS NULL, 0, 8 + 8)) AS RunDurationUsecSize,
				(IF(tr.Tags IS NULL, 0, LENGTH(ARRAY_TO_STRING(tr.Tags, '')) + ARRAY_LENGTH(tr.Tags) * 8 + 8)) AS TagsSize,
				(IF(TestMetadata IS NULL, 0, LENGTH(TestMetadata) + 8)) AS TestMetadataSize,
				(IF(FailureReason IS NULL, 0, LENGTH(FailureReason) + 8)) AS FailureReasonSize,
				(IF(tr.Properties IS NULL, 0, LENGTH(tr.Properties) + 8)) AS PropertiesSize,
			FROM TestResults tr
				JOIN@{JOIN_METHOD=MERGE_JOIN,FORCE_JOIN_ORDER=TRUE} Invocations inv USING (InvocationId)
			WHERE
				-- Sample 1/256 invocations to reduce the amount of the splits we need to
				-- scan.
				--
				-- It's ideal to keep this as large as possible so the we can ensure
				-- that projects with very few invocations (e.g. infra), or projects
				-- with invocations that varies a lot in the size of the invocation
				-- (e.g. chromeos), have enough invocations sampled.
				STARTS_WITH(InvocationId, "00")

				-- Within each invocation, sample 1/256 test results to reduce the cost
				-- of sampling an invocation. This helps keeping the number of sampled
				-- invocations large without causing the query to timeout.
				--
				-- TestId based sampling is used because
				-- 1. It's faster than TABLESAMPLE BERNOULLI.
				-- 2. CommitTimestamp based sampling many cause the entire invocation to
				-- be skipped when all results are committed in the same transaction.
				-- 3. The CoV is low enough
				-- (go/resultdb-test-results-table-disk-usage-test-id-based-sampling).
				AND MOD(FARM_FINGERPRINT(TestId), 256) = 0
		)
		SELECT
			-- Extract project from realm.
			-- Projects like chrome-m100, chrome-m101 will be treated as chrome-m to
			-- prevent the number of projects exploding.
			IFNULL(REGEXP_EXTRACT(realm, r'^([^:-]+-m)[0-9]+:'), SUBSTR(realm, 0, STRPOS(realm, ':') - 1)) AS Project,
			SUM(InvocationIdSize) * 65536 AS InvocationIdSize,
			SUM(TestIdSize) * 65536 AS TestIdSize,
			SUM(ResultIdSize) * 65536 AS ResultIdSize,
			SUM(VariantSize) * 65536 AS VariantSize,
			SUM(VariantHashSize) * 65536 AS VariantHashSize,
			SUM(CommitTimestampSize) * 65536 AS CommitTimestampSize,
			SUM(IsUnexpectedSize) * 65536 AS IsUnexpectedSize,
			SUM(StatusSize) * 65536 AS StatusSize,
			SUM(SummaryHTMLSize) * 65536 AS SummaryHTMLSize,
			SUM(StartTimeSize) * 65536 AS StartTimeSize,
			SUM(RunDurationUsecSize) * 65536 AS RunDurationUsecSize,
			SUM(TagsSize) * 65536 AS TagsSize,
			SUM(TestMetadataSize) * 65536 AS TestMetadataSize,
			SUM(FailureReasonSize) * 65536 AS FailureReasonSize,
			SUM(PropertiesSize) * 65536 AS PropertiesSize,
			SUM(IF(IsUnexpected, InvocationIdSize, 0)) * 65536 AS UnexpectedTestResults_InvocationIdSize,
			SUM(IF(IsUnexpected, TestIdSize, 0)) * 65536 AS UnexpectedTestResults_TestIdSize,
			SUM(IF(IsUnexpected, IsUnexpectedSize, 0)) * 65536 AS UnexpectedTestResults_IsUnexpectedSize,
			SUM(IF(IsUnexpected, VariantHashSize, 0)) * 65536 AS UnexpectedTestResults_VariantHashSize,
			SUM(IF(IsUnexpected, VariantSize, 0)) * 65536 AS UnexpectedTestResults_VariantSize,
		FROM test_result_sizes
		GROUP BY Project
	`)

	projectStats = []testResultsColumnSizes{}
	var b spanutil.Buffer
	err = spanutil.Query(span.Single(ctx), st, func(row *spanner.Row) error {
		columnSizes := testResultsColumnSizes{}
		err := b.FromSpanner(
			row,
			&columnSizes.Project,
			&columnSizes.InvocationID,
			&columnSizes.TestID,
			&columnSizes.ResultID,
			&columnSizes.Variant,
			&columnSizes.VariantHash,
			&columnSizes.CommitTimestamp,
			&columnSizes.IsUnexpected,
			&columnSizes.Status,
			&columnSizes.SummaryHTML,
			&columnSizes.StartTime,
			&columnSizes.RunDurationUsec,
			&columnSizes.Tags,
			&columnSizes.TestMetadata,
			&columnSizes.FailureReason,
			&columnSizes.Properties,
			&columnSizes.UnexpectedTestResultsInvocationID,
			&columnSizes.UnexpectedTestResultsTestID,
			&columnSizes.UnexpectedTestResultsIsUnexpected,
			&columnSizes.UnexpectedTestResultsVariantHash,
			&columnSizes.UnexpectedTestResultsVariant,
		)
		if err != nil {
			return err
		}
		projectStats = append(projectStats, columnSizes)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return projectStats, nil
}
