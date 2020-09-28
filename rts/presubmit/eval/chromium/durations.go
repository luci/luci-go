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
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/rts/presubmit/eval"
	"google.golang.org/api/iterator"
)

// TestDurationsSample implements eval.Backend, based on Chromium's CQ and
// ResultDB BigQuery tables.
// It returns 100k test durations with 1d TTL.
func (b *Backend) TestDurationsSample(req eval.TestDurationsSampleRequest) (*eval.TestDurationsSampleResponse, error) {
	bq, err := b.bqClient(req.Context, req.Authenticator)
	if err != nil {
		return nil, errors.Annotate(err, "failed to init BigQuery client").Err()
	}

	it, err := bq.Query(testDurationsSQL).Read(req.Context)
	if err != nil {
		return nil, err
	}

	res := &eval.TestDurationsSampleResponse{
		TestDurations: make([]*eval.TestDuration, 0, 100000),
		TTL:           24 * time.Hour,
	}
	for {
		td := &eval.TestDuration{}
		err := it.Next(td)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		res.TestDurations = append(res.TestDurations, td)
	}
	return res, nil
}

const testDurationsSQL = `
WITH
    tryjobs AS (
        SELECT
            b.id,
            ps,
        FROM commit-queue.chromium.attempts a, a.gerrit_changes ps, a.builds b
        WHERE partition_time > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 1 DAY)
    ),
    test_results AS (
        SELECT
            CAST(REGEXP_EXTRACT(exported.id, r'build-(\d+)') as INT64) build_id,
            test_location.file_name,
            test_id,
            duration,
        FROM luci-resultdb.chromium.try_test_results
        WHERE partition_time > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 1 DAY)
            AND duration > 0
            -- Exclude broken test locations.
            -- TODO(nodir): remove this after crbug.com/1130425 is fixed.
            AND REGEXP_CONTAINS(test_location.file_name, r'(?i)\.(cc|html|m|c|cpp)$')
            -- Exclude broken prefixes.
            -- TODO(nodir): remove after crbug.com/1017288 is fixed.
            AND (test_id NOT LIKE 'ninja://:blink_web_tests/%' OR test_location.file_name LIKE '//third_party/%')
    )
SELECT
    STRUCT(
        STRUCT(ps.host as Host, ps.project as Project, ps.change as Number) AS Change,
        ps.patchset as Patchset
    ) Patchset,
    STRUCT(
        test_id as ID,
        file_name as FileName
    ) as Test,
    CAST(duration * 1e9 as INT64) as Duration -- nanoseconds
FROM tryjobs t
JOIN test_results tr ON t.id = tr.build_id
ORDER BY FARM_FINGERPRINT(FORMAT('%s/%d/%d', ps.host, ps.change, ps.patchset))
LIMIT 100000
`
