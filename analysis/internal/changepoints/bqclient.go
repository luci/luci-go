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
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/tracing"
)

// NewClient creates a new client for reading changepints.
func NewClient(ctx context.Context, gcpProject string) (*Client, error) {
	client, err := bq.NewClient(ctx, gcpProject)
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
	Variant     bigquery.NullJSON
	// Point to a branch in the source control.
	Ref     *Ref
	RefHash string
	// The source verdict unexpected rate before the changepoint.
	UnexpectedSourceVerdictRateBefore float64
	// The source verdict unexpected rate after the changepoint.
	UnexpectedSourceVerdictRateAfter float64
	// The current source verdict unexpected rate.
	UnexpectedSourceVerdictRateCurrent float64
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

// ReadChangepoints reads changepoints of a certain week from BigQuery.
// The week parameter can be at any time of that week. A week is defined by Sunday to Satureday in UTC.
func (c *Client) ReadChangepoints(ctx context.Context, project string, week time.Time) (changepoints []*ChangepointRow, err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/changepoints.ReadChangepoints",
		attribute.String("project", project),
		attribute.String("week", week.String()),
	)
	defer func() { tracing.End(s, err) }()
	query := `
		SELECT
			project AS Project,
			test_id_num AS TestIDNum,
			test_id AS TestID,
			variant_hash AS VariantHash,
			ref_hash AS RefHash,
			ref AS Ref,
			variant AS Variant,
			start_hour AS StartHour,
			start_position_lower_bound_99th AS LowerBound99th,
			start_position AS NominalStartPosition,
			start_position_upper_bound_99th AS UpperBound99th,
			unexpected_verdict_rate AS UnexpectedSourceVerdictRateAfter,
			previous_unexpected_verdict_rate AS UnexpectedSourceVerdictRateBefore,
			latest_unexpected_verdict_rate AS UnexpectedSourceVerdictRateCurrent,
			previous_nominal_end_position as PreviousNominalEndPosition
		FROM test_variant_changepoints
		WHERE project = @project
			AND TIMESTAMP_TRUNC(start_hour, WEEK(SUNDAY)) =  TIMESTAMP_TRUNC(@week, WEEK(SUNDAY))
	`
	q := c.client.Query(query)
	q.DefaultDatasetID = "internal"
	q.Parameters = []bigquery.QueryParameter{
		{Name: "project", Value: project},
		{Name: "week", Value: week},
	}
	it, err := q.Read(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "read query results").Err()
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

// ReadChangepointsRealtime reads changepoints of a certain week directly from BigQuery exports of changepoint analysis.
// A week in this context refers to the period from Sunday at 00:00:00 AM UTC (inclusive)
// to the following Sunday at 00:00:00 AM UTC (exclusive).
// The week parameter MUST be a timestamp representing the start of a week (Sunday at 00:00:00 AM UTC).
func (c *Client) ReadChangepointsRealtime(ctx context.Context, week time.Time) (changepoints []*ChangepointRow, err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/changepoints.ReadChangepointsRealtime",
		attribute.String("week", week.String()),
	)
	defer func() { tracing.End(s, err) }()
	if !isBeginOfWeek(week) {
		return nil, errors.New("week should be the start of week, i.e. a Sunday midnight")
	}
	query := `
		WITH
		  merged_table AS (
				SELECT *
				FROM test_variant_segment_updates
				UNION ALL
				SELECT *
				FROM test_variant_segments
			),
			merged_table_grouped AS (
				SELECT
					project, test_id, variant_hash, ref_hash,
					ARRAY_AGG(m ORDER BY version DESC LIMIT 1)[OFFSET(0)] as row
				FROM merged_table m
				GROUP BY project, test_id, variant_hash, ref_hash
			),
			test_variant_segments_realtime AS (
				SELECT
					project, test_id, variant_hash, ref_hash,
					row.variant AS variant,
					row.ref AS ref,
					row.segments AS segments
				FROM merged_table_grouped
			),
			changepoint_with_failure_rate AS (
				SELECT
					tv.* EXCEPT (segments),
					segment.start_hour,
					segment.start_position_lower_bound_99th,
					segment.start_position,
					segment.start_position_upper_bound_99th,
					idx,
					SAFE_DIVIDE(segment.counts.unexpected_verdicts, segment.counts.total_verdicts) AS unexpected_verdict_rate,
					SAFE_DIVIDE(tv.segments[0].counts.unexpected_verdicts, tv.segments[0].counts.total_verdicts) AS latest_unexpected_verdict_rate,
					SAFE_DIVIDE(tv.segments[idx+1].counts.unexpected_verdicts, tv.segments[idx+1].counts.total_verdicts) AS previous_unexpected_verdict_rate,
					tv.segments[idx+1].end_position AS previous_nominal_end_position
				FROM test_variant_segments_realtime tv, UNNEST(segments) segment WITH OFFSET idx
				-- TODO: Filter out test variant branches with more than 10 segments is a bit hacky, but it filter out oscillate test variant branches.
				-- It would be good to find a more elegant solution, maybe explicitly expressing this as a filter on the RPC.
				WHERE ARRAY_LENGTH(segments) >= 2 AND ARRAY_LENGTH(segments) <= 10
				AND idx + 1 < ARRAY_LENGTH(tv.segments)
			),
			-- Obtain the alphabetical ranking for each test ID in each LUCI project.
			test_id_ranking AS (
				SELECT project, test_id, ROW_NUMBER() OVER (PARTITION BY project ORDER BY test_id) AS row_num
				FROM test_variant_segments_realtime
				GROUP BY project, test_id
			)
		SELECT
			cp.project as Project,
			cp.test_id as TestID,
			cp.variant_hash as VariantHash,
			cp.ref_hash as RefHash,
			cp.variant as Variant,
			cp.ref as Ref,
			cp.start_hour AS StartHour,
			cp.start_position_lower_bound_99th AS LowerBound99th,
			cp.start_position AS NominalStartPosition,
			cp.start_position_upper_bound_99th AS UpperBound99th,
			cp.unexpected_verdict_rate AS UnexpectedSourceVerdictRateAfter,
			cp.latest_unexpected_verdict_rate AS UnexpectedSourceVerdictRateCurrent,
			cp.previous_unexpected_verdict_rate AS UnexpectedSourceVerdictRateBefore,
			cp.previous_nominal_end_position as PreviousNominalEndPosition,
			ranking.row_num AS TestIDNum
		FROM changepoint_with_failure_rate cp
		LEFT JOIN test_id_ranking ranking
		ON ranking.project = cp.project and ranking.test_id = cp.test_id
		-- Only keep regressions. A regression is a special changepoint when the later segment has a higher unexpected verdict rate than the earlier segment.
		-- In the future, we might want to return all changepoints to show fixes in the UI.
		WHERE cp.unexpected_verdict_rate - cp.previous_unexpected_verdict_rate > 0
			AND TIMESTAMP_TRUNC(cp.start_hour, WEEK(SUNDAY)) = @week
	`
	q := c.client.Query(query)
	q.DefaultDatasetID = "internal"
	q.Parameters = []bigquery.QueryParameter{
		{Name: "week", Value: week},
	}
	job, err := q.Run(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "starting query").Err()
	}
	// WaitForJob cancels the job with best-effort when context deadline is reached.
	js, err := bq.WaitForJob(ctx, job)
	if err != nil {
		return nil, err
	}
	if js.Err() != nil {
		return nil, js.Err()
	}
	it, err := job.Read(ctx)
	if err != nil {
		return nil, errors.Annotate(js.Err(), "read changepoint rows").Err()
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

// Return whether time is the start of a week.
// A week is defined as Sunday midnight (inclusive) to next Saturday midnight (exclusive) in UTC.
// Therefore, t MUST be a timestamp at Sunday midnight (00:00 AM) UTC for this to return True.
func isBeginOfWeek(t time.Time) bool {
	isSunday := t.Weekday() == time.Sunday
	isMidnight := t.Truncate(24*time.Hour) == t
	return isSunday && isMidnight
}
