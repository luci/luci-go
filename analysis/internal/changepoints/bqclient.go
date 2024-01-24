// Copyright 2024 The LUCI Authors.
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

package changepoints

import (
	"context"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/bqutil"
)

// NewClient creates a new client for reading changepints.
func NewClient(ctx context.Context, gcpProject string) (*Client, error) {
	client, err := bqutil.Client(ctx, gcpProject)
	if err != nil {
		return nil, err
	}
	return &Client{client: client}, nil
}

// Client to read LUCI Analysis changepoints.
type Client struct {
	client *bigquery.Client
}

// Close releases any resources held by the client.
func (c *Client) Close() error {
	return c.client.Close()
}

// ChangepointRow represents a changepoint of a test variant branch.
type ChangepointRow struct {
	Project string
	// TestIDNum is the alphabetical ranking of the test ID in this LUCI project.
	TestIDNum   int64
	TestID      string
	VariantHash string
	// Point to a branch in the source control.
	Ref     *Ref
	RefHash string
	// The verdict unexpected rate before the changepoint.
	UnexpectedVerdictRateBefore float64
	// The verdict unexpected rate after the changepoint.
	UnexpectedVerdictRateAfter float64
	// The current verdict unexpected rate.
	UnexpectedVerdictRateCurrent float64
	// The nominal start hour of the segment after the changepoint.
	StartHour                  time.Time
	LowerBound99th             int64
	UpperBound99th             int64
	NominalStartPosition       int64
	PreviousNominalEndPosition int64
}

type Ref struct {
	Gitiles *Gitiles
}
type Gitiles struct {
	Host    bigquery.NullString
	Project bigquery.NullString
	Ref     bigquery.NullString
}

// ReadChangepoints reads changepoints of the current week from BigQuery.
// TODO: Allow reading changepoints for any week.
func (c *Client) ReadChangepoints(ctx context.Context, project string) ([]*ChangepointRow, error) {
	query := `
		-- TODO: extract this SQL into a new view to avoid the keeping multiple copies of this view around.
		WITH merged_table AS (
			SELECT *
			FROM internal.test_variant_segment_updates
			WHERE project = @project AND has_recent_unexpected_results = 1
			UNION ALL
			SELECT *
			FROM internal.test_variant_segments
			WHERE project = @project AND has_recent_unexpected_results = 1
		),
		merged_table_grouped AS (
			SELECT
				project, test_id, variant_hash, ref_hash,
				ARRAY_AGG(m ORDER BY version DESC LIMIT 1)[OFFSET(0)] AS row
			FROM merged_table m
			GROUP BY project, test_id, variant_hash, ref_hash
		),
		segments_with_failure_rate AS (
			SELECT
				project,
				test_id,
				variant_hash,
				ref_hash,
				row.variant AS variant,
				row.ref AS ref,
				segment,
				idx,
				SAFE_DIVIDE(segment.counts.unexpected_verdicts, segment.counts.total_verdicts) AS unexpected_verdict_rate,
				SAFE_DIVIDE(tv.row.segments[0].counts.unexpected_verdicts, tv.row.segments[0].counts.total_verdicts) AS latest_unexpected_verdict_rate,
				SAFE_DIVIDE(tv.row.segments[idx+1].counts.unexpected_verdicts, tv.row.segments[idx+1].counts.total_verdicts) AS previous_unexpected_verdict_rate,
				tv.row.segments[idx+1].end_position AS previous_nominal_end_position
			FROM merged_table_grouped tv, UNNEST(row.segments) segment WITH OFFSET idx
			-- TODO: Filter out test variant branches with more than 10 segments is a bit hacky, but it filter out oscillate test variant branches.
			-- It would be good to find a more elegant solution, maybe explicitly expressing this as a filter on the RPC.
			WHERE ARRAY_LENGTH(row.segments) >= 2 AND ARRAY_LENGTH(row.segments) <= 10
			AND idx + 1 < ARRAY_LENGTH(tv.row.segments)
		),
		-- Obtain the alphabetical ranking for each test ID in this LUCI project.
		test_id_ranking AS (
			SELECT test_id, ROW_NUMBER() OVER (ORDER BY test_id) AS row_num
			FROM internal.test_variant_segments
			WHERE project = @project
			GROUP BY test_id
		)
		SELECT
			segment.project AS Project,
			ranking.row_num AS TestIDNum,
			segment.test_id AS TestID,
			segment.variant_hash AS VariantHash,
			segment.ref_hash AS RefHash,
			segment.ref AS Ref,
			segment.variant AS Variant,
			segment.segment.start_hour AS StartHour,
			segment.segment.start_position_lower_bound_99th AS LowerBound99th,
			segment.segment.start_position AS NominalStartPosition,
			segment.segment.start_position_upper_bound_99th AS UpperBound99th,
			segment.unexpected_verdict_rate AS UnexpectedVerdictRateAfter,
			segment.previous_unexpected_verdict_rate AS UnexpectedVerdictRateBefore,
			segment.latest_unexpected_verdict_rate AS UnexpectedVerdictRateCurrent,
			segment.previous_nominal_end_position as PreviousNominalEndPosition
		FROM segments_with_failure_rate segment
		LEFT JOIN test_id_ranking ranking
		ON ranking.test_id = segment.test_id
		-- Only keep regressions. A regression is a special changepoint when the later segment has a higher unexpected verdict rate than the earlier segment.
		-- In the future, we might want to return all changepoints to show fixes in the UI.
		WHERE segment.unexpected_verdict_rate - segment.previous_unexpected_verdict_rate > 0
		AND segment.segment.start_hour >= TIMESTAMP_TRUNC(CURRENT_TIMESTAMP(), WEEK(SUNDAY))
	`
	q := c.client.Query(query)
	q.DefaultDatasetID = "internal"
	q.Parameters = []bigquery.QueryParameter{
		{Name: "project", Value: project},
	}
	job, err := q.Run(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "running query").Err()
	}
	it, err := job.Read(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "read").Err()
	}
	results := []*ChangepointRow{}
	for {
		row := &ChangepointRow{}
		err := it.Next(row)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, errors.Annotate(err, "obtain next changepoint row").Err()
		}
		results = append(results, row)
	}
	return results, nil
}
