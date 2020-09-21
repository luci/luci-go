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

// RejectedPatchSets implements eval.RejectedPatchSetProvider.
func RejectedPatchSets(req eval.RejectedPatchSetRequest) ([]*eval.RejectedPatchSet, error) {
	// Create a BigQuery client.
	creds, err := req.Authenticator.PerRPCCredentials()
	if err != nil {
		return nil, err
	}
	bq, err := bigquery.NewClient(req.Context, "luci-resultdb", option.WithGRPCDialOption(grpc.WithPerRPCCredentials(creds)))
	if err != nil {
		return nil, errors.Annotate(err, "failed to init BigQuery client").Err()
	}

	q := bq.Query(rejectedPatchSetQueryText)
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

const rejectedPatchSetQueryText = `
	WITH
		patchsets AS (
			SELECT
					partition_time as timestamp,
					ps,
					ARRAY(SELECT b.id FROM a.builds b) builds,
			# TODO(nodir): unhardcode the table/view name.
			FROM commit-queue.chromium.attempts a, a.gerrit_changes ps
			WHERE partition_time BETWEEN @startTime AND @endTime
					# TODO(nodir): add support for multi-patchset CQ attempts
					AND ARRAY_LENGTH(a.gerrit_changes) = 1
		),
		failed_test_variants AS (
			SELECT DISTINCT
					CAST(REGEXP_EXTRACT(exported.id, r'build-(\d+)') as INT64) build_id,
					ANY_VALUE(test_location.file_name) file_name,
					test_id
			FROM luci-resultdb.chromium.try_test_results
			WHERE partition_time BETWEEN @startTime and @endTime

				AND (test_location.file_name LIKE '%.cc' OR test_location.file_name LIKE '%.html')
				-- Exclude broken prefixes
				AND (test_id NOT LIKE 'ninja://:blink_web_tests/%' OR test_location.file_name LIKE '//third_party/%')

			GROUP BY build_id, test_id, variant_hash
			HAVING LOGICAL_AND(NOT expected AND NOT exonerated)
		),
		flat AS (
			SELECT
				ps.ps.host,
				ps.ps.change,
				ps.ps.patchset,
				MIN(timestamp) timestamp,
				test_id,
				ANY_VALUE(file_name) as file_name,
			FROM patchsets ps, ps.builds b
			JOIN failed_test_variants t ON b = t.build_id
			GROUP BY ps.ps.host, ps.ps.change, ps.ps.patchset, test_id
		)
	SELECT
		STRUCT(STRUCT(host, change as number) AS Change, patchset) Patchset,
		ANY_VALUE(Timestamp) as Timestamp,
		ARRAY_AGG(STRUCT(test_id as ID, file_name as FileName)) as FailedTests,
	FROM flat
	GROUP BY host, change, flat.patchset
`
