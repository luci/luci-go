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

// rejections calls f for each found CQ rejection.
func (r *presubmitHistoryRun) rejections(ctx context.Context, f func(*evalpb.Rejection) error) error {
	q, err := r.bqQuery(ctx, rejectedPatchSetsSQL)
	if err != nil {
		return err
	}
	it, err := q.Read(ctx)
	if err != nil {
		return err
	}

	for {
		var row rejectionRow
		err := it.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		if err := f(row.proto()); err != nil {
			return err
		}
	}
	return nil
}

type rejectionRow struct {
	Change      int
	Patchset    int
	Timestamp   time.Time
	FailedTests []struct {
		ID       string
		FileName string
	}
}

func (r *rejectionRow) proto() *evalpb.Rejection {
	ret := &evalpb.Rejection{
		Patchsets: []*evalpb.GerritPatchset{
			{
				Change: &evalpb.GerritChange{
					// Assume all Chromium source code is in
					// https://chromium.googlesource.com/chromium/src
					// TOOD(nodir): make it fail if it is not.
					Host:    "chromium-review.googlesource.com",
					Project: "chromium/src",
					Number:  int64(r.Change),
				},
				Patchset: int64(r.Patchset),
			},
		},
		FailedTests: make([]*evalpb.Test, len(r.FailedTests)),
	}
	ret.Timestamp, _ = ptypes.TimestampProto(r.Timestamp)
	for i, t := range r.FailedTests {
		ret.FailedTests[i] = &evalpb.Test{Id: t.ID, FileName: t.FileName}
	}
	return ret
}

// rejectedPatchSetsSQL is a BigQuery query that returns patchsets with test
// failures. Ignores tests that don't have a test location.
const rejectedPatchSetsSQL = `
	WITH
		tryjobs AS (
			SELECT
				b.id,
				ps,
				partition_time as ps_approx_timestamp,
			FROM commit-queue.chromium.attempts a, a.gerrit_changes ps, a.builds b
			-- Read next-day attempts too in case the CQ attempt finished soon after 12am.
			-- Note that for test results, the cutoff is still @endTime.
			WHERE partition_time BETWEEN @startTime AND TIMESTAMP_ADD(@endTime, INTERVAL 1 DAY)
		),
		failed_test_variants AS (
			SELECT DISTINCT
				CAST(REGEXP_EXTRACT(exported.id, r'build-(\d+)') as INT64) build_id,
				ANY_VALUE(test_location.file_name) file_name,
				test_id,
			FROM luci-resultdb.chromium.try_test_results tr
			WHERE partition_time BETWEEN @startTime and @endTime
				AND (@test_id_regexp = '' OR REGEXP_CONTAINS(test_id, @test_id_regexp))
				AND (@builder_regexp = '' OR EXISTS (SELECT 0 FROM tr.variant WHERE key='builder' AND REGEXP_CONTAINS(value, @builder_regexp)))
				-- Exclude broken test locations.
				-- TODO(nodir): remove this after crbug.com/1130425 is fixed.
				AND REGEXP_CONTAINS(test_location.file_name, r'(?i)\.(cc|html|m|c|cpp)$')
				-- Exclude broken prefixes.
				-- TODO(nodir): remove after crbug.com/1017288 is fixed.
				AND (test_id NOT LIKE 'ninja://:blink_web_tests/%' OR test_location.file_name LIKE '//third_party/%')
			GROUP BY build_id, test_id, variant_hash
			-- TODO(crbug.com/1112125): consider doing this filtering after joining with patchsets.
			-- This will increase the query cost, but might improve
			-- data quality. Currenly the query is vulnerable to situations where
			-- a test failed in one CQ attempt for the patchset, and succeeded
			-- in another for the same patchset.
			HAVING LOGICAL_AND(NOT expected AND NOT exonerated)
		),
		flat AS (
			SELECT
				ps.change,
				ps.patchset,
				MIN(ps_approx_timestamp) ps_approx_timestamp,
				test_id,
				ANY_VALUE(file_name) as file_name,
			FROM tryjobs t
			JOIN failed_test_variants f ON t.id = f.build_id
			GROUP BY ps.change, ps.patchset, test_id
		)
	SELECT
		change as Change,
		patchset as Patchset,
		ANY_VALUE(ps_approx_timestamp) as Timestamp,
		ARRAY_AGG(STRUCT(test_id as ID, file_name as FileName)) as FailedTests,
	FROM flat
	GROUP BY change, flat.patchset
`
