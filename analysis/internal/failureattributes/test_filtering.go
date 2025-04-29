// Copyright 2023 The LUCI Authors.
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

package failureattributes

import (
	"context"
	"time"

	"cloud.google.com/go/bigquery"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/config"
)

// NewFilteredRunsAttributionHandler initialises a new TestFilteringHandler instance.
func NewFilteredRunsAttributionHandler(cloudProject string) *FilteredRunsAttributionHandler {
	return &FilteredRunsAttributionHandler{cloudProject: cloudProject}
}

// FilteredRunsAttributionHandler handles the attribute-filtered-test-runs cron job.
type FilteredRunsAttributionHandler struct {
	cloudProject string
}

// CronHandler handles the attribute-filtered-test-runs cron job.
func (h *FilteredRunsAttributionHandler) CronHandler(ctx context.Context) error {
	attributesClient, err := NewClient(ctx, h.cloudProject)
	if err != nil {
		return err
	}
	projectConfigs, err := config.Projects(ctx)
	if err != nil {
		return errors.Annotate(err, "obtain project configs").Err()
	}

	for _, project := range projectConfigs.Keys() {
		err := attributesClient.attributeFilteredRuns(ctx, project)
		if err != nil {
			return errors.Annotate(err, "attribute filtered test runs for %s", project).Err()
		}
	}

	return nil
}

// ingestionDelayThresholdDays is the maximum number of days a test filtering
// event (represented as a skipped test result) need to be ingested after the
// test filtering event occurs (determined by its partition date). If the test
// result is ingested more than `ingestionDelayThresholdDays` days later, it
// will not be attributed to older test failures.
//
// A reasonable threshold should achieve a balance between
//  1. tolerating ingestion delay (particularly due to ingestion failures and
//     retries), and
//  2. reducing the amount of data the attribution query need to process.
//
// TODO: record the partition time of the earliest, unprocessed test filtering
// event so we can increase the partition time window in the query as needed
// instead of relying on a hardcoded threshold.
const ingestionDelayThresholdDays = 3

// attributionRule specifies how the filtered test runs are attributed to the
// test failures in a project.
type attributionRule struct {
	// attributionWindowDays is the maximum number of days a filtered test run
	// can be attributed to a test failure since the failure occurs. i.e. A
	// filtered test run can only be attributed to a test failure if
	// test_run_partition_time - test_failure_partition_time <=
	// attributionWindowDays days.
	attributionWindowDays int64
	// failureFilterSQL is a predicate on the results that defines which results
	// may have filtered test runs being attributed to them.
	failureFilterSQL string
	// partitionSQL is the SQL snippet used to partition the test results.
	// Only results in the same partition as a test failure can be attributed to
	// the test failure.
	partitionSQL string
	// runFilterSQL is a predicate on the results that defines which results
	// represents a run that filtered out the test.
	runFilterSQL string
	// runCountSQL is an expression that defines the distinct runs to count.
	//
	// CAVEAT: the test_verdicts table may contain duplicated rows. To avoid
	// counting duplicates, do not count the number of rows directly.
	runCountSQL string
}

// projectAttributionRule specifies the attribution rule of each project.
// Attribution is only enabled when the attribution rule is defined for a
// project.
//
// TODO: move this to project configs with a more restrictive syntax and less
// dependency on the exact shape of the query.
var projectAttributionRule = map[string]attributionRule{
	// As defined in go/cros-test-filtering.
	"chromeos": {
		attributionWindowDays: 14,
		failureFilterSQL:      `NOT r.expected AND tv.change_verifier_run IS NULL`,
		// Ideally, the skipped test results should be attributed to failures that
		// caused them. i.e. failures in the same milestone if there are enough
		// samples, or failures in the previous milestone otherwise. But given that
		// luci-analysis does not know whether the testing filtering was activated
		// from failures from which milestone, it's hard to write a query that
		// can attribute correctly.
		//
		// Instead, we attribute skipped test results to failures in the any
		// milestone. This is only an issue when
		// 1. the test is failing in a previous milestone for a different root
		// cause, and
		// 2. the failures in the other milestone did not trigger test filtering
		// themselves (therefore already marked as "triggered test filtering").
		// Hopefully, this should be very rare. And we can adjust the attribution
		// logic further if this become an issue.
		partitionSQL: `tv.test_id, STRING(tv.variant.board)`,
		runFilterSQL: `tv.change_verifier_run IS NOT NULL AND r.expected AND r.status='SKIP' ` +
			`AND r.skip_reason='AUTOMATICALLY_DISABLED_FOR_FLAKINESS'`,
		runCountSQL: `r.name`,
	},
}

// attributeFilteredRuns runs a query to attribute the filtered test runs to
// failures that cause the test to be filtered out. Then saves the result to
// failure_attributes table.
func (s *Client) attributeFilteredRuns(ctx context.Context, project string) error {
	if err := s.ensureSchema(ctx); err != nil {
		return errors.Annotate(err, "ensure schema").Err()
	}

	rule, ok := projectAttributionRule[project]
	if !ok {
		return nil
	}

	q := s.bqClient.Query(`
		MERGE INTO internal.failure_attributes T
		USING (
			WITH post_finalization_attributes AS (
				SELECT
					tv.invocation.id AS verdict_invocation_id,
					r.name AS result_name,
					tv.partition_time AS partition_time,
					` + rule.failureFilterSQL + ` AS is_attribution_target,
					-- COUNT(DISTINCT IF(...)) cannot be used when window declaration has
					-- ORDER BY. Gather the items in an array so we can count the distinct
					-- items in the array later.
					ARRAY_AGG(IF(
						` + rule.runFilterSQL + `,
						` + rule.runCountSQL + `,
						NULL
					)) OVER attribution_source AS attributed_filtered_runs,
				FROM internal.test_verdicts AS tv, UNNEST(results) AS r
				-- Postsubmit failures from more than @attributionWindowMs milliseconds
				-- ago will no longer have new skip results being attributed to them.
				-- Add @delayThresholdMs milliseconds to account for test results
				-- ingestion delay (particularly due to ingestion failures and retries).
				WHERE
					tv.partition_time >= TIMESTAMP_SUB(
						CURRENT_TIMESTAMP(),
						INTERVAL @attributionWindowMs + @delayThresholdMs MILLISECOND
					)
					AND tv.project = @project
				WINDOW attribution_source AS (
					PARTITION BY ` + rule.partitionSQL + `
					ORDER BY UNIX_MILLIS(tv.partition_time)
					RANGE BETWEEN 1 FOLLOWING AND @attributionWindowMs FOLLOWING
				)
			)
			SELECT
				@project AS project,
				"resultdb" AS test_result_system,
				verdict_invocation_id AS ingested_invocation_id,
				result_name AS test_result_id,
				ANY_VALUE(partition_time) AS partition_time,
				ANY_VALUE((SELECT COUNT(DISTINCT run) FROM UNNEST(attributed_filtered_runs) AS run)) AS attributed_filtered_run_count,
			FROM post_finalization_attributes
			WHERE is_attribution_target
			-- test_verdicts table may contain duplicated rows. Use a group statement
			-- to prevent the duplicated rows from causing the DML merge to fail.
			GROUP BY verdict_invocation_id, result_name
		) S
		ON S.partition_time = T.partition_time
			AND S.project = T.project
			AND S.test_result_system = T.test_result_system
  		AND S.ingested_invocation_id = T.ingested_invocation_id
			AND S.test_result_id = T.test_result_id
		WHEN MATCHED
			AND S.attributed_filtered_run_count > T.attributed_filtered_run_count
		THEN
			UPDATE SET attributed_filtered_run_count = S.attributed_filtered_run_count
		WHEN NOT MATCHED THEN
			INSERT (project, test_result_system, ingested_invocation_id, test_result_id, partition_time, attributed_filtered_run_count)
			VALUES (
				S.project,
				S.test_result_system,
				S.ingested_invocation_id,
				S.test_result_id,
				S.partition_time,
				S.attributed_filtered_run_count
			);
	`)
	q.Parameters = append(q.Parameters,
		bigquery.QueryParameter{Name: "project", Value: project},
		bigquery.QueryParameter{Name: "attributionWindowMs", Value: rule.attributionWindowDays * 24 * 60 * 60 * 1000},
		bigquery.QueryParameter{Name: "delayThresholdMs", Value: ingestionDelayThresholdDays * 24 * 60 * 60 * 1000},
	)

	job, err := q.Run(ctx)
	if err != nil {
		return err
	}

	waitCtx, cancel := context.WithTimeout(ctx, time.Minute*9)
	defer cancel()
	js, err := bq.WaitForJob(waitCtx, job)
	if err != nil {
		return errors.Annotate(err, "waiting for filtered test run attribution query to complete").Err()
	}
	if err := js.Err(); err != nil {
		return errors.Annotate(err, "filtered test run attribution query failed").Err()
	}
	return nil
}
