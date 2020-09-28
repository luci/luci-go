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

package chromium

import (
	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/rts/presubmit/eval"
)

// RejectedPatchSets implements eval.Backend, based on Chromium's CQ and
// ResultDB BigQuery tables.
func (b *Backend) RejectedPatchSets(req eval.RejectedPatchSetsRequest) ([]*eval.RejectedPatchSet, error) {
	// Create a BigQuery client.
	creds, err := req.Authenticator.PerRPCCredentials()
	if err != nil {
		return nil, err
	}
	bq, err := bigquery.NewClient(req.Context, "chrome-trooper-analytics", option.WithGRPCDialOption(grpc.WithPerRPCCredentials(creds)))
	if err != nil {
		return nil, errors.Annotate(err, "failed to init BigQuery client").Err()
	}

	q := bq.Query(rejectedPatchSetsSQL)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "startTime", Value: req.StartTime},
		{Name: "endTime", Value: req.EndTime},
	}
	it, err := q.Read(req.Context)
	if err != nil {
		return nil, err
	}

	var ret []*eval.RejectedPatchSet
	for {
		rp := &eval.RejectedPatchSet{}
		err := it.Next(rp)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		ret = append(ret, rp)
	}
	return ret, nil
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
			FROM luci-resultdb.chromium.try_test_results
			WHERE partition_time BETWEEN @startTime and @endTime
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
				ps.host,
				ANY_VALUE(ps.project) project,
				ps.change,
				ps.patchset,
				MIN(ps_approx_timestamp) ps_approx_timestamp,
				test_id,
				ANY_VALUE(file_name) as file_name,
			FROM tryjobs t
			JOIN failed_test_variants f ON t.id = f.build_id
			GROUP BY ps.host, ps.change, ps.patchset, test_id
		)
	SELECT
		STRUCT(
			STRUCT(host as Host, ANY_VALUE(project) as Project, change as Number) AS Change,
			patchset as Patchset
		) Patchset,
		ANY_VALUE(ps_approx_timestamp) as Timestamp,
		ARRAY_AGG(STRUCT(test_id as ID, file_name as FileName)) as FailedTests,
	FROM flat
	GROUP BY host, change, flat.patchset
`
