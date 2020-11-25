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
	"golang.org/x/sync/errgroup"
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

	eg, ctx := errgroup.WithContext(ctx)

	withoutChangedFiles := make(chan *evalpb.TestDuration)
	eg.Go(func() error {
		defer close(withoutChangedFiles)
		for {
			var row durationRow
			switch err := it.Next(&row); {
			case err == iterator.Done:
				return nil
			case err != nil:
				return err
			}

			select {
			case withoutChangedFiles <- row.proto():
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	})

	for i := 0; i < 100; i++ {
		eg.Go(func() error {
			for td := range withoutChangedFiles {
				switch err := r.populateChangedFiles(ctx, td.Patchsets[0]); {
				case err == errPatchsetDeleted:
					continue
				case err != nil:
					return err
				}
				if err := callback(td); err != nil {
					return err
				}
			}
			return nil
		})
	}

	return eg.Wait()
}

type durationRow struct {
	Change      int
	Patchset    int
	TestVariant testVariantRow
	Duration    float64
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
				Patchset: int64(r.Patchset),
			},
		},
		TestVariant: r.TestVariant.proto(),
		Duration:    ptypes.DurationProto(time.Duration(r.Duration * 1e9)),
	}
}

const testDurationsSQL = `
WITH
	tryjobs AS (
		SELECT
			b.id,
			ps,
		FROM commit-queue.chromium.attempts a, a.gerrit_changes ps, a.builds b
		WHERE partition_time BETWEEN @startTime AND TIMESTAMP_ADD(@endTime, INTERVAL 1 DAY)
	),
	test_results AS (
		SELECT
			CAST(REGEXP_EXTRACT(exported.id, r'build-(\d+)') as INT64) as build_id,
			IFNULL(test_location.file_name, '') as file_name,
			test_id,
			variant,
			duration,
		FROM luci-resultdb.chromium.try_test_results tr
		WHERE partition_time BETWEEN @startTime and @endTime
			AND RAND() <= @frac
			AND (@test_id_regexp = '' OR REGEXP_CONTAINS(test_id, @test_id_regexp))
			AND (@builder_regexp = '' OR EXISTS (SELECT 0 FROM tr.variant WHERE key='builder' AND REGEXP_CONTAINS(value, @builder_regexp)))
			AND duration > @minDuration

			-- Exclude broken test locations.
			-- TODO(nodir): remove this after crbug.com/1130425 is fixed.
			AND REGEXP_CONTAINS(IFNULL(test_location.file_name, ''), r'(?i)^(|.*\.(cc|html|m|c|cpp))$')
			-- Exclude broken prefixes.
			-- TODO(nodir): remove after crbug.com/1017288 is fixed.
			AND (test_id NOT LIKE 'ninja://:blink_web_tests/%' OR test_location.file_name LIKE '//third_party/%')
	)
SELECT
	ps.change as Change,
	ps.patchset as Patchset,
	STRUCT(
		test_id as ID,
		variant as Variant,
		file_name as FileName
	) as TestVariant,
	duration as Duration
FROM tryjobs t
JOIN test_results tr ON t.id = tr.build_id
`
