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

	"github.com/golang/protobuf/ptypes"
	"google.golang.org/api/iterator"

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

var terminalRejectionFragment = &evalpb.RejectionFragment{
	Terminal: true,
}

// rejections calls f for each found CQ rejection.
func (r *presubmitHistoryRun) rejections(ctx context.Context, f func(*evalpb.RejectionFragment) error) error {
	q, err := r.bqQuery(ctx, rejectedPatchSetsSQL)
	if err != nil {
		return err
	}
	it, err := q.Read(ctx)
	if err != nil {
		return err
	}

	// The result rows are ordered by change and patchset numbers,
	// and so are the rejection fragments.
	// Keep track of the current patchset to detect patchset boundaries.
	var curChange, curPS int

	maybeFinalizeCurrent := func() error {
		if curChange == 0 {
			return nil
		}
		return f(terminalRejectionFragment)
	}

	// Iterate over results and call f.
	// One fragment represents at most one BigQuery row.
	for {
		var row rejectionRow
		switch err := it.Next(&row); {
		case err == iterator.Done:
			return maybeFinalizeCurrent()
		case err != nil:
			return err
		}

		newPS := curChange != row.Change || curPS != row.Patchset
		if newPS {
			if err := maybeFinalizeCurrent(); err != nil {
				return err
			}
		}
		curChange = row.Change
		curPS = row.Patchset

		// Report the failure.
		rej := &evalpb.Rejection{
			FailedTestVariants: []*evalpb.TestVariant{row.FailedTestVariant.proto()},
		}
		// If this is the first fragment of a rejection, include patchset-level info.
		if newPS {
			rej.Patchsets = []*evalpb.GerritPatchset{{
				Change: &evalpb.GerritChange{
					// Assume all Chromium source code is in
					// https://chromium.googlesource.com/chromium/src
					// TOOD(nodir): make it fail if it is not.
					Host:    "chromium-review.googlesource.com",
					Project: "chromium/src",
					Number:  int64(row.Change),
				},
				Patchset: int64(row.Patchset),
			}}
			rej.Timestamp, _ = ptypes.TimestampProto(row.Timestamp)
		}
		if err := f(&evalpb.RejectionFragment{Rejection: rej}); err != nil {
			return err
		}
	}
}

type rejectionRow struct {
	Change            int
	Patchset          int
	Timestamp         time.Time
	FailedTestVariant testVariantRow
}

type testVariantRow struct {
	ID       string
	FileName string
	Variant  []struct {
		Key   string
		Value string
	}
}

func (t *testVariantRow) proto() *evalpb.TestVariant {
	ret := &evalpb.TestVariant{
		Id:       t.ID,
		FileName: t.FileName,
		Variant:  make(map[string]string, len(t.Variant)),
	}
	for _, kv := range t.Variant {
		ret.Variant[kv.Key] = kv.Value
	}
	return ret
}

// rejectedPatchSetsSQL is a BigQuery query that returns patchsets with test
// failures. Ignores tests that don't have a test location.
//
// This query intentionally does not use ARRAY_AGG for failed tests because
// otherwise a result row size may exceed the API limitation of 10MB.
const rejectedPatchSetsSQL = `
	WITH
		tryjobs AS (
			SELECT
				b.id as build_id,
				ps.change,
				ps.patchset,
				partition_time as ps_approx_timestamp,
			FROM commit-queue.chromium.attempts a, a.gerrit_changes ps, a.builds b
			-- Read next-day attempts too in case the CQ attempt finished soon after 12am.
			-- Note that for test results, the cutoff is still @endTime.
			WHERE partition_time BETWEEN @startTime AND TIMESTAMP_ADD(@endTime, INTERVAL 1 DAY)
		),
		test_results AS (
			SELECT
				CAST(REGEXP_EXTRACT(exported.id, r'build-(\d+)') as INT64) as build_id,
				test_id,
				variant_hash,
				variant,
				status = 'PASS' as pass,
				expected,
				test_location.file_name,
			FROM luci-resultdb.chromium.try_test_results tr
			WHERE partition_time BETWEEN @startTime and @endTime
				AND not exonerated
				AND status IN ('PASS', 'FAIL', 'CRASH')
				AND (@test_id_regexp = '' OR REGEXP_CONTAINS(test_id, @test_id_regexp))
				AND (@builder_regexp = '' OR EXISTS (SELECT 0 FROM tr.variant WHERE key='builder' AND REGEXP_CONTAINS(value, @builder_regexp)))
				-- Exclude broken test locations.
				-- TODO(nodir): remove this after crbug.com/1130425 is fixed.
				AND REGEXP_CONTAINS(IFNULL(test_location.file_name, ''), r'(?i)^(|.*\.(cc|html|m|c|cpp))$')
				-- Exclude broken prefixes.
				-- TODO(nodir): remove after crbug.com/1017288 is fixed.
				AND (test_id NOT LIKE 'ninja://:blink_web_tests/%' OR test_location.file_name LIKE '//third_party/%')
		),
		-- Test results annotated with patchset.
		ps_test_results AS (
			SELECT change, patchset, ps_approx_timestamp, r.*
			FROM test_results r
			JOIN tryjobs USING (build_id)
		),

		-- Test variants that are considered flaky.
		-- They will be excluded from the analysis.
		flaky_test_variants AS (
			SELECT DISTINCT test_id, variant_hash
			FROM ps_test_results
			GROUP BY change, patchset, test_id, variant_hash
			HAVING LOGICAL_OR(pass) AND LOGICAL_OR(NOT pass)
		),

		-- Failed tests per patchset, including potentially flaky tests.
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
	SELECT
		change as Change,
		patchset as Patchset,
		ps_approx_timestamp as Timestamp,
		STRUCT(
			test_id as ID,
			variant as Variant,
			file_name as FileName
		) as FailedTestVariant
	FROM failed_test_variants_per_ps
	LEFT JOIN flaky_test_variants flaky USING (test_id, variant_hash)
	WHERE flaky.test_id IS NULL -- not flaky
	ORDER BY change, patchset
`
