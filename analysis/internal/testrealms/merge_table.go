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
		return errors.Annotate(err, "create bq client").Err()
	}
	defer func() {
		if err := client.Close(); err != nil && retErr == nil {
			retErr = errors.Annotate(err, "closing bq client").Err()
		}
	}()

	err = ensureTestRealmsSchema(ctx, client)
	if err != nil {
		return errors.Annotate(err, "ensure schema").Err()
	}

	// DML merge from test_results to test_realms table.
	err = runDMLMerge(ctx, client)
	if err != nil {
		return errors.Annotate(err, "run DML merge").Err()
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
				LOWER(test_id) as test_id_lower,
				STRUCT(
					LOWER(ANY_VALUE(test_id_structured).module_name) as module_name,
					ANY_VALUE(test_id_structured).module_scheme as module_scheme,
					LOWER(ANY_VALUE(test_id_structured).coarse_name) as coarse_name,
					LOWER(ANY_VALUE(test_id_structured).fine_name) as fine_name,
					LOWER(ANY_VALUE(test_id_structured).case_name) as case_name
				) as test_id_structured_lower,
			  -- Use partition time to determine the last seen (retention) time, not the insert time, as all data
				-- retention intervals are based on partition time.
				MAX(partition_time) as last_seen
			FROM test_verdicts
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
			INSERT (project, test_id, realm, test_id_lower, test_id_structured, test_id_structured_lower, last_seen) VALUES (S.project, S.test_id, S.realm, S.test_id_lower, S.test_id_structured, S.test_id_structured_lower, S.last_seen)
		-- Delete rows that are older than 90 days (that are not being updated).
		WHEN NOT MATCHED BY SOURCE AND T.last_seen < TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 90 DAY) THEN
			DELETE;
	`)
	q.DefaultDatasetID = bqutil.InternalDatasetID

	job, err := q.Run(ctx)
	if err != nil {
		return errors.Annotate(err, "initiate merge query").Err()
	}

	waitCtx, cancel := context.WithTimeout(ctx, time.Minute*9)
	defer cancel()
	js, err := bq.WaitForJob(waitCtx, job)
	if err != nil {
		return errors.Annotate(err, "waiting for merging to complete").Err()
	}
	if err := js.Err(); err != nil {
		return errors.Annotate(err, "merge rows failed").Err()
	}
	return nil
}

func ensureTestRealmsSchema(ctx context.Context, client *bigquery.Client) error {
	table := client.Dataset(bqutil.InternalDatasetID).Table(tableName)
	if err := schemaApplyer.EnsureTable(ctx, table, tableMetadata); err != nil {
		return errors.Annotate(err, "ensuring %s table", tableName).Err()
	}
	return nil
}
