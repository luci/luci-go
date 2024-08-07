// Copyright 2024 The LUCI Authors.
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

// Package bqupdator handles the update of bigquery tables related to changepoint analysis.
package bqupdator

import (
	"context"
	"time"

	"cloud.google.com/go/bigquery"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/bqutil"
)

// UpdateChangepointTable is the entry point of the update-changepoint-table cron job.
// It runs DDL to create or replace the test_variant_changepoints table.
func UpdateChangepointTable(ctx context.Context, gcpProject string) (retErr error) {
	client, err := bqutil.Client(ctx, gcpProject)
	if err != nil {
		return errors.Annotate(err, "create bq client").Err()
	}
	defer func() {
		if err := client.Close(); err != nil && retErr == nil {
			retErr = errors.Annotate(err, "closing bq client").Err()
		}
	}()
	return runCreateOrReplace(ctx, client)
}

// runCreateOrReplace create or replace the test_variant_changepoints table.
func runCreateOrReplace(ctx context.Context, client *bigquery.Client) error {
	q := client.Query(`
	CREATE OR REPLACE TABLE test_variant_changepoints
	AS
	WITH
	merged_table AS (
		SELECT *
		FROM test_variant_segment_updates
		WHERE  has_recent_unexpected_results = 1
		UNION ALL
		SELECT *
		FROM test_variant_segments
		WHERE has_recent_unexpected_results = 1
	),
	merged_table_grouped AS (
		SELECT
			project, test_id, variant_hash, ref_hash,
			ARRAY_AGG(m ORDER BY version DESC LIMIT 1)[OFFSET(0)] AS row
		FROM merged_table m
		GROUP BY project, test_id, variant_hash, ref_hash
	),
	segments_with_failure_rate AS (
		SELECT
			project,
			test_id,
			variant_hash,
			ref_hash,
			row.variant,
			row.ref,
			segment,
			idx,
			SAFE_DIVIDE(segment.counts.unexpected_verdicts, segment.counts.total_verdicts) AS unexpected_verdict_rate,
			SAFE_DIVIDE(tv.row.segments[0].counts.unexpected_verdicts, tv.row.segments[0].counts.total_verdicts) AS latest_unexpected_verdict_rate,
			SAFE_DIVIDE(tv.row.segments[idx+1].counts.unexpected_verdicts, tv.row.segments[idx+1].counts.total_verdicts) AS previous_unexpected_verdict_rate,
			tv.row.segments[idx+1].end_position AS previous_nominal_end_position
		FROM merged_table_grouped tv, UNNEST(row.segments) segment WITH OFFSET idx
		-- TODO: Filter out test variant branches with more than 10 segments is a bit hacky, but it filter out oscillate test variant branches.
		-- It would be good to find a more elegant solution, maybe explicitly expressing this as a filter on the RPC.
		WHERE ARRAY_LENGTH(row.segments) >= 2 AND ARRAY_LENGTH(row.segments) <= 10
		AND idx + 1 < ARRAY_LENGTH(tv.row.segments)
	),
	-- Obtain the alphabetical ranking for each test ID in each LUCI project.
	test_id_ranking AS (
		SELECT project, test_id, ROW_NUMBER() OVER (PARTITION BY project ORDER BY test_id) AS row_num
		FROM test_variant_segments
		GROUP BY project, test_id
	)
	SELECT
		segment.* EXCEPT (segment, idx),
		segment.segment.start_hour,
		segment.segment.start_position_lower_bound_99th,
		segment.segment.start_position,
		segment.segment.start_position_upper_bound_99th,
		ranking.row_num AS test_id_num,
	FROM segments_with_failure_rate segment
	LEFT JOIN test_id_ranking ranking
	ON ranking.project = segment.project and ranking.test_id = segment.test_id
	-- Only keep regressions. A regression is a special changepoint when the later segment has a higher unexpected verdict rate than the earlier segment.
	-- In the future, we might want to return all changepoints to show fixes in the UI.
	WHERE segment.unexpected_verdict_rate - segment.previous_unexpected_verdict_rate > 0
	`)
	q.DefaultDatasetID = bqutil.InternalDatasetID

	job, err := q.Run(ctx)
	if err != nil {
		return errors.Annotate(err, "initiate test_variant_changepoints DDL").Err()
	}

	waitCtx, cancel := context.WithTimeout(ctx, time.Minute*9)
	defer cancel()
	js, err := bq.WaitForJob(waitCtx, job)
	if err != nil {
		return errors.Annotate(err, "waiting for query to complete").Err()
	}
	if js.Err() != nil {
		return errors.Annotate(js.Err(), "DDL query failed").Err()
	}
	return nil
}
