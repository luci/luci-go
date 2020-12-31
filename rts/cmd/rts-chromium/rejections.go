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

package main

import (
	"context"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/golang/protobuf/ptypes"
	"google.golang.org/api/iterator"

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

var terminalRejectionFragment = &evalpb.RejectionFragment{
	Terminal: true,
}

// rejections calls the callback for each found CQ rejection.
func (r *presubmitHistoryRun) rejections(ctx context.Context, callback func(*evalpb.RejectionFragment) error) error {
	q, err := r.bqQuery(ctx, rejectedPatchSetsSQL)
	if err != nil {
		return err
	}
	q.Parameters = append(q.Parameters, bigquery.QueryParameter{
		Name:  "minCLFlakes",
		Value: r.minCLFlakes,
	})

	it, err := q.Read(ctx)
	if err != nil {
		return err
	}

	// The result rows are ordered by change and patchset numbers,
	// and so are the rejection fragments.
	// Keep track of the current patchset to detect patchset boundaries.
	var curChange, curPS int
	curPSIsDeleted := false

	maybeFinalizeCurrent := func() error {
		if curChange == 0 || curPSIsDeleted {
			return nil
		}
		return callback(terminalRejectionFragment)
	}

	// Iterate over results and call the callback.
	// One fragment represents at most one BigQuery row.
	for {
		// Read the next row.
		var row rejectionRow
		switch err := it.Next(&row); {
		case err == iterator.Done:
			return maybeFinalizeCurrent()
		case err != nil:
			return err
		}

		rej := &evalpb.Rejection{
			FailedTestVariants: []*evalpb.TestVariant{row.FailedTestVariant},
		}
		// Finalize the current rejection if this patchset is different.
		if curChange != row.Change || curPS != row.Patchset {
			if err := maybeFinalizeCurrent(); err != nil {
				return err
			}

			// Include the patchset-level info only in the first fragment.
			row.populatePatchsetInfo(rej)
			switch err := r.populateChangedFiles(ctx, rej.Patchsets[0]); {
			case err == errPatchsetDeleted:
				curPSIsDeleted = true
			case err != nil:
				return err
			default:
				curPSIsDeleted = false
			}
		}

		if !curPSIsDeleted {
			if err := callback(&evalpb.RejectionFragment{Rejection: rej}); err != nil {
				return err
			}
		}

		curChange = row.Change
		curPS = row.Patchset
	}
}

type rejectionRow struct {
	Change            int
	Patchset          int
	Timestamp         time.Time
	FailedTestVariant *evalpb.TestVariant
}

func (r *rejectionRow) populatePatchsetInfo(rej *evalpb.Rejection) {
	rej.Patchsets = []*evalpb.GerritPatchset{{
		Change: &evalpb.GerritChange{
			// Assume all Chromium source code is in
			// https://chromium.googlesource.com/chromium/src
			// TOOD(nodir): make it fail if it is not.
			Host:    "chromium-review.googlesource.com",
			Project: "chromium/src",
			Number:  int64(r.Change),
		},
		Patchset: int64(r.Patchset),
	}}
	rej.Timestamp, _ = ptypes.TimestampProto(r.Timestamp)
}

// rejectedPatchSetsSQL is a BigQuery query that returns patchsets with test
// failures. Excludes flaky tests.
//
// This query intentionally does not use ARRAY_AGG for failed tests because
// otherwise a result row size may exceed the API limitation of 10MB.
const rejectedPatchSetsSQL = `
	WITH
		-- Select all tryjobs that participated in CQ in the given time window,
		-- along with their patchsets.
		--
		-- Use earliest_equivalent_patchset field instead of patchset, such that
		-- trivial rebases are treated as the same patchset, and to ignore
		-- CL description edits.
		tryjobs AS (
			SELECT
				b.id as build_id,
				ps.change,
				ps.earliest_equivalent_patchset as patchset,
				partition_time as ps_approx_timestamp,
			FROM commit-queue.chromium.attempts a, a.gerrit_changes ps, a.builds b
			WHERE partition_time BETWEEN @startTime AND @endTime
		),
		-- Select 'try' test results. All early filtering is done here.
		test_results AS (
			SELECT
				-- select the build_id to join with tryjobs.
				CAST(REGEXP_EXTRACT(exported.id, r'build-(\d+)') as INT64) as build_id,
				test_id,
				variant_hash,
				variant,
				expected,
				-- Replace NULLs with '' to keep Go code simple.
				IFNULL(test_location.file_name, '') as file_name,
			FROM luci-resultdb.chromium.try_test_results tr
			-- Read prev-day and next-day results too to ensure that we have ALL
			-- results of a given CQ attempt.
			WHERE partition_time BETWEEN TIMESTAMP_SUB(@startTime, INTERVAL 1 DAY) and TIMESTAMP_ADD(@endTime, INTERVAL 1 DAY)
				AND not exonerated
				AND status != 'SKIP' -- not needed for RTS purposes
				AND (@test_id_regexp = '' OR REGEXP_CONTAINS(test_id, @test_id_regexp))
				AND (@builder_regexp = '' OR EXISTS (SELECT 0 FROM tr.variant WHERE key='builder' AND REGEXP_CONTAINS(value, @builder_regexp)))
				-- Exclude broken test locations.
				-- TODO(nodir): remove this after crbug.com/1130425 is fixed.
				AND REGEXP_CONTAINS(IFNULL(test_location.file_name, ''), r'(?i)^(|.*\.(cc|html|m|c|cpp|js))$')
				-- Exclude broken prefixes.
				-- TODO(nodir): remove after crbug.com/1017288 is fixed.
				AND (test_id NOT LIKE 'ninja://:blink_web_tests/%' OR test_location.file_name LIKE '//third_party/%')
		),
		-- Test results annotated with patchset.
		-- Join the two queries above.
		ps_test_results AS (
			SELECT change, patchset, ps_approx_timestamp, r.*
			FROM test_results r
			JOIN tryjobs USING (build_id)
		),

    -- The following two sub-queries detect flaky tests.
		-- It is important to exclude them from analysis because they represent
		-- noise.
		--
		-- A test variant is considered flaky if it has mixed results in >=N
		-- separate CLs. N=1 is too small because otherwise the query is vulnerable
		-- to a single bad patchset that introduces flakiness and never lands.
		--
		-- The first sub-query finds test variants with mixed results per CL.
		-- Note that GROUP BY includes patchset, but SELECT doesn't and uses
		-- DISTINCT.
		-- The second sub-query filters out test variant candidates that have mixed
		-- results win fewer than @minCLFlakes CLs.
		flaky_test_variants_per_cl AS (
			SELECT DISTINCT change, test_id, variant_hash
			FROM ps_test_results
			GROUP BY change, patchset, test_id, variant_hash
			HAVING LOGICAL_OR(expected) AND LOGICAL_OR(NOT expected)
		),
		flaky_test_variants AS (
			SELECT test_id, variant_hash
			FROM flaky_test_variants_per_cl
			GROUP BY test_id, variant_hash
			HAVING COUNT(change) >= @minCLFlakes
		),

		-- Select test variants with unexpected results for each patchset.
		-- Does not filter out flaky test variants just yet.
		failed_test_variants_per_ps AS (
			SELECT
				change,
				patchset,
				test_id,
				variant_hash,
				MIN(ps_approx_timestamp) as ps_approx_timestamp,
				ANY_VALUE(variant) as variant,
				ANY_VALUE(file_name) as file_name
			FROM ps_test_results
			GROUP BY change, patchset, test_id, variant_hash
			HAVING LOGICAL_AND(NOT expected)
		)

	-- Exclude flaky tests from failed_test_variants_per_ps.
	-- Prepare the results for consumption by Go code (fields, order).
	SELECT
		change as Change,
		patchset as Patchset,
		ps_approx_timestamp as Timestamp,
		STRUCT(
			test_id as Id,
			ARRAY(SELECT FORMAT("%s:%s", key, value) kv FROM UNNEST(variant) ORDER BY kv) as Variant,
			file_name as FileName
		) as FailedTestVariant
	FROM failed_test_variants_per_ps
	LEFT JOIN flaky_test_variants flaky USING (test_id, variant_hash)
	-- flaky.test_id is NULL if LEFT JOIN did not find a flaky_test_variants row
	-- for this test variant, i.e. the test is not flaky
	WHERE flaky.test_id IS NULL
	ORDER BY change, patchset
`
