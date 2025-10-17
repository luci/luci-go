// Copyright 2025 The LUCI Authors.
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

package testrealms

import (
	"context"
	"time"

	"cloud.google.com/go/bigquery"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/bqutil"
)

// MergeTables is the entry point of the merge-test-realms cron job.
// It runs use DML merge to merge the data from test_variants table to
// test_realms table.
func MergeTables(ctx context.Context, gcpProject string) (retErr error) {
	client, err := bq.NewClient(ctx, gcpProject)
	if err != nil {
		return errors.Fmt("create bq client: %w", err)
	}
	defer func() {
		if err := client.Close(); err != nil && retErr == nil {
			retErr = errors.Fmt("closing bq client: %w", err)
		}
	}()

	err = ensureTestRealmsSchema(ctx, client)
	if err != nil {
		return errors.Fmt("ensure schema: %w", err)
	}

	// DML merge from test_results to test_realms table.
	err = runDMLMerge(ctx, client)
	if err != nil {
		return errors.Fmt("run DML merge: %w", err)
	}

	return nil
}

// runDMLMerge merges data from test_variants table to the test_realms table.
func runDMLMerge(ctx context.Context, client *bigquery.Client) error {
	q := client.Query(`
		MERGE test_realms T
		USING (
			SELECT
				project,
				test_id,
				STRUCT(
					ANY_VALUE(test_id_structured).module_name as module_name,
					ANY_VALUE(test_id_structured).module_scheme as module_scheme,
					ANY_VALUE(test_id_structured).coarse_name as coarse_name,
					ANY_VALUE(test_id_structured).fine_name as fine_name,
					ANY_VALUE(test_id_structured).case_name as case_name
				) as test_id_structured,
				-- Use exported invocation realm instead for for consistency with TestResults spanner table.
				-- (Consistent ACLing with test history view.)
				invocation.realm,
				-- Test IDs in lowercase to support faster case-insensitive matching.
				LOWER(test_id) as test_id_lower,
				STRUCT(
					LOWER(ANY_VALUE(test_id_structured).module_name) as module_name,
					ANY_VALUE(test_id_structured).module_scheme as module_scheme,
					LOWER(ANY_VALUE(test_id_structured).coarse_name) as coarse_name,
					LOWER(ANY_VALUE(test_id_structured).fine_name) as fine_name,
					LOWER(ANY_VALUE(test_id_structured).case_name) as case_name
				) as test_id_structured_lower,
				-- Select test name, preferring to take it from rows that have it set.
				ANY_VALUE(test_metadata.name HAVING MAX COALESCE(IF(test_metadata.name <> '', 10, 0), 0)) as test_name,
				LOWER(ANY_VALUE(test_metadata.name HAVING MAX COALESCE(IF(test_metadata.name <> '', 10, 0), 0))) as test_name_lower,
			  -- Use partition time to determine the last seen (retention) time, not the insert time, as all data
				-- retention intervals are based on partition time.
				MAX(partition_time) as last_seen
			FROM test_verdicts
			-- Query recent data (e.g. last 2 days) but not the super new data as that is not
			-- yet compacted and slower to query. It doesn't matter if test search does not
			-- update immediately.
			WHERE insert_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 1 HOUR)
   		  AND partition_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 2 DAY)
			GROUP BY project, test_id, invocation.realm
		) S
		ON T.project = S.project AND T.test_id = S.test_id AND T.realm = S.realm
		-- Row in source is newer than target, update.
		WHEN MATCHED AND S.last_seen > T.last_seen THEN
			UPDATE SET T.last_seen = S.last_seen
		-- Row in source that does not exist in target. Insert.
		WHEN NOT MATCHED BY TARGET THEN
			INSERT (project, test_id, realm, test_id_lower, test_id_structured, test_id_structured_lower, test_metadata, last_seen) VALUES (S.project, S.test_id, S.realm, S.test_id_lower, S.test_id_structured, S.test_id_structured_lower, S.test_metadata, S.last_seen)
		-- Delete tests which have not been seen within the last 90 days.
		WHEN NOT MATCHED BY SOURCE AND T.last_seen < TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 90 DAY) THEN
			DELETE;
	`)
	q.DefaultDatasetID = bqutil.InternalDatasetID

	job, err := q.Run(ctx)
	if err != nil {
		return errors.Fmt("initiate merge query: %w", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, time.Minute*9)
	defer cancel()
	js, err := bq.WaitForJob(waitCtx, job)
	if err != nil {
		return errors.Fmt("waiting for merging to complete: %w", err)
	}
	if err := js.Err(); err != nil {
		return errors.Fmt("merge rows failed: %w", err)
	}
	return nil
}

func ensureTestRealmsSchema(ctx context.Context, client *bigquery.Client) error {
	table := client.Dataset(bqutil.InternalDatasetID).Table(tableName)
	if err := schemaApplyer.EnsureTable(ctx, table, tableMetadata); err != nil {
		return errors.Fmt("ensuring %s table: %w", tableName, err)
	}
	return nil
}
