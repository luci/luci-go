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

package bqexporter

import (
	"context"
	"time"

	"cloud.google.com/go/bigquery"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/analysis/internal/bqutil"
	"go.chromium.org/luci/analysis/internal/config"
)

// MergeTables is the entry point of the merge-test-variant-branches cron job.
// It runs use DML merge to merge the data from test_variant_segment_updates
// table to test_variant_segments table.
func MergeTables(ctx context.Context, gcpProject string) (retErr error) {
	enabled, err := shouldMergeTable(ctx)
	if err != nil {
		return err
	}
	if !enabled {
		logging.Infof(ctx, "Skipped because export is not enabled")
		return nil
	}

	client, err := bq.NewClient(ctx, gcpProject)
	if err != nil {
		return errors.Annotate(err, "create bq client").Err()
	}
	defer func() {
		if err := client.Close(); err != nil && retErr == nil {
			retErr = errors.Annotate(err, "closing bq client").Err()
		}
	}()

	err = ensureTestVariantSegmentsSchema(ctx, client)
	if err != nil {
		return errors.Annotate(err, "ensure schema").Err()
	}

	// DML merge from test-variant-segment-updates to test-variant-segments table.
	err = runDMLMerge(ctx, client)
	if err != nil {
		return errors.Annotate(err, "run DML merge").Err()
	}

	return nil
}

// runDMLMerge merges data from test_variant_segment_updates table to
// test_variant_segments table.
func runDMLMerge(ctx context.Context, client *bigquery.Client) error {
	q := client.Query(`
		MERGE test_variant_segments T
			USING (
				SELECT
					ARRAY_AGG(u ORDER BY version DESC LIMIT 1)[OFFSET(0)] as row
				FROM test_variant_segment_updates u
				GROUP BY project, test_id, variant_hash, ref_hash
			) S
		ON T.project = S.row.project AND T.test_id = S.row.test_id AND T.variant_hash = S.row.variant_hash AND T.ref_hash = S.row.ref_hash
		-- Row in source is newer than target, update.
		WHEN MATCHED AND S.row.version > T.version THEN
			UPDATE SET T.has_recent_unexpected_results = S.row.has_recent_unexpected_results, T.segments = S.row.segments, T.version=S.row.version
		-- Row in source that does not exist in target. Insert.
		WHEN NOT MATCHED BY TARGET THEN
			INSERT (project, test_id, variant_hash, ref_hash, variant, ref, segments, has_recent_unexpected_results, version) VALUES (S.row.project, S.row.test_id, S.row.variant_hash, S.row.ref_hash, S.row.variant, S.row.ref, S.row.segments, S.row.has_recent_unexpected_results, S.row.version)
		-- Delete rows from target older than 90 days that are not being updated.
		WHEN NOT MATCHED BY SOURCE AND T.version < TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 90 DAY) THEN
			DELETE;
		-- If the merge is successful, delete data from test_variant_segment_updates table.
		-- We are conservative and only delete the data older than 20 minutes ago.
		-- It may result in some duplication among merges, but it should not affect
		-- the correctness of data.
		DELETE FROM test_variant_segment_updates
			WHERE version < TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 20 MINUTE);
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
	if js.Err() != nil {
		return errors.Annotate(js.Err(), "merge rows failed").Err()
	}
	return nil
}

func ensureTestVariantSegmentsSchema(ctx context.Context, client *bigquery.Client) error {
	table := client.Dataset(bqutil.InternalDatasetID).Table(stableTableName)
	if err := schemaApplyer.EnsureTable(ctx, table, tableMetadata); err != nil {
		return errors.Annotate(err, "ensuring test_variant_segments table").Err()
	}
	return nil
}

// shouldMergeTable checks config to see whether bq exporter is enabled.
func shouldMergeTable(ctx context.Context) (bool, error) {
	cfg, err := config.Get(ctx)
	if err != nil {
		return false, errors.Annotate(err, "read config").Err()
	}
	if !cfg.GetTestVariantAnalysis().GetEnabled() {
		return false, nil
	}
	if !cfg.GetTestVariantAnalysis().GetBigqueryExportEnabled() {
		return false, nil
	}
	return true, nil
}
