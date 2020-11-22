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

// durations calls the callback for each found test duration.
func (r *presubmitHistoryRun) durations(ctx context.Context, callback func(*evalpb.TestDuration) error) error {
	q, err := r.bqQuery(ctx, testDurationsSQL)
	if err != nil {
		return err
	}

	q.Parameters = append(q.Parameters,
		bigquery.QueryParameter{
			Name:  "frac",
			Value: r.durationDataFrac,
		},
		bigquery.QueryParameter{
			Name:  "minDuration",
			Value: r.minDuration.Seconds(),
		},
	)

	it, err := q.Read(ctx)
	if err != nil {
		return err
	}

	for {
		var row durationRow
		switch err := it.Next(&row); {
		case err == iterator.Done:
			return nil
		case err != nil:
			return err
		default:
			if err := callback(row.proto()); err != nil {
				return err
			}
		}
	}
}

type durationRow struct {
	Change       int
	Patchset     int
	ChangedFiles []string
	TestVariant  testVariantRow
	Duration     float64
}

func (r *durationRow) proto() *evalpb.TestDuration {
	return &evalpb.TestDuration{
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
				Patchset:     int64(r.Patchset),
				ChangedFiles: toSourceFiles(r.ChangedFiles),
			},
		},
		TestVariant: r.TestVariant.proto(),
		Duration:    ptypes.DurationProto(time.Duration(r.Duration * 1e9)),
	}
}

const testDurationsSQL = queryHeader + `
	some_test_results AS (
		SELECT *
		FROM test_results
		WHERE RAND() <= @frac
	)
SELECT
	ps.change as Change,
	ps.patchset as Patchset,
	changed_files as ChangedFiles,
	STRUCT(
		test_id as ID,
		variant as Variant,
		file_name as FileName
	) as TestVariant,
	duration as Duration
FROM tryjobs_with_files t
JOIN test_results tr ON t.id = tr.build_id
`
