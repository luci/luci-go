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
	"fmt"
	"strconv"
	"time"

	"cloud.google.com/go/bigquery"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/pagination"
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

// ChangepointDetailRow represents a changepoint of a test variant branch with more details including TestIDNum and source verdict rates.
type ChangepointDetailRow struct {
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

// ChangepointRow represents a changepoint of a test variant branch.
type ChangepointRow struct {
	Project     string
	TestID      string
	VariantHash string
	Variant     bigquery.NullJSON
	// Point to a branch in the source control.
	Ref     *Ref
	RefHash string
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
func (c *Client) ReadChangepoints(ctx context.Context, project string, week time.Time) (changepoints []*ChangepointDetailRow, err error) {
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
		return nil, errors.Fmt("read query results: %w", err)
	}
	results := []*ChangepointDetailRow{}
	for {
		row := &ChangepointDetailRow{}
		err := it.Next(row)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, errors.Fmt("obtain next changepoint row: %w", err)
		}
		results = append(results, row)
	}
	return results, nil
}

// ReadChangepointsRealtime reads changepoints of a certain week directly from BigQuery exports of changepoint analysis.
// A week in this context refers to the period from Sunday at 00:00:00 AM UTC (inclusive)
// to the following Sunday at 00:00:00 AM UTC (exclusive).
// The week parameter MUST be a timestamp representing the start of a week (Sunday at 00:00:00 AM UTC) and within the last 90 days.
func (c *Client) ReadChangepointsRealtime(ctx context.Context, week time.Time) (changepoints []*ChangepointDetailRow, err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/changepoints.ReadChangepointsRealtime",
		attribute.String("week", week.String()),
	)
	defer func() { tracing.End(s, err) }()
	if !isBeginOfWeek(week) {
		return nil, errors.New("week should be the start of week, i.e. a Sunday midnight")
	}
	// The table test_variant_segments_unexpected_realtime only contains test variant branches with unexpected results in the last 90 days.
	// Changepints before 90 days might not exist in test_variant_segments_unexpected_realtime.
	if week.Before(time.Now().Add(-90 * 24 * time.Hour)) {
		return nil, errors.New("week should be the within the last 90 days")
	}
	query := `
		WITH
			changepoint_with_failure_rate AS (
				SELECT
					tv.* EXCEPT (segments, version),
					segment.start_hour,
					segment.start_position_lower_bound_99th,
					segment.start_position,
					segment.start_position_upper_bound_99th,
					idx,
					SAFE_DIVIDE(segment.counts.unexpected_verdicts, segment.counts.total_verdicts) AS unexpected_verdict_rate,
					SAFE_DIVIDE(tv.segments[0].counts.unexpected_verdicts, tv.segments[0].counts.total_verdicts) AS latest_unexpected_verdict_rate,
					SAFE_DIVIDE(tv.segments[idx+1].counts.unexpected_verdicts, tv.segments[idx+1].counts.total_verdicts) AS previous_unexpected_verdict_rate,
					tv.segments[idx+1].end_position AS previous_nominal_end_position
				FROM test_variant_segments_unexpected_realtime tv, UNNEST(segments) segment WITH OFFSET idx
				-- TODO: Filter out test variant branches with more than 10 segments is a bit hacky, but it filter out oscillate test variant branches.
				-- It would be good to find a more elegant solution, maybe explicitly expressing this as a filter on the RPC.
				WHERE ARRAY_LENGTH(segments) >= 2 AND ARRAY_LENGTH(segments) <= 10
				AND idx + 1 < ARRAY_LENGTH(tv.segments)
			),
			-- Obtain the alphabetical ranking for each test ID in each LUCI project.
			test_id_ranking AS (
				SELECT project, test_id, ROW_NUMBER() OVER (PARTITION BY project ORDER BY test_id) AS row_num
				FROM test_variant_segments
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
		return nil, errors.Fmt("starting query: %w", err)
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
		return nil, errors.Fmt("read changepoint rows: %w", err)
	}
	results := []*ChangepointDetailRow{}
	for {
		row := &ChangepointDetailRow{}
		err := it.Next(row)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, errors.Fmt("obtain next changepoint row: %w", err)
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

type GroupSummary struct {
	CanonicalChangepoint               ChangepointRow
	Total                              int64
	UnexpectedSourceVerdictRateBefore  RateDistribution
	UnexpectedSourceVerdictRateAfter   RateDistribution
	UnexpectedSourceVerdictRateCurrent RateDistribution
	UnexpectedSourveVerdictRateChange  RateChangeDistribution
}

type RateDistribution struct {
	Mean                    float64
	Less5Percent            int64
	Above5LessThan95Percent int64
	Above95Percent          int64
}

type RateChangeDistribution struct {
	Increase0to20percent   int64
	Increase20to50percent  int64
	Increase50to100percent int64
}

type ReadChangepointGroupSummariesOptions struct {
	Project       string
	TestIDContain string
	PageSize      int
	PageToken     string
}

func parseReadChangepointGroupSummariesPageToken(pageToken string) (afterStartHourUnix int, afterTestID, afterVariantHash, afterRefHash string, afterNominalStartPosition int, err error) {
	tokens, err := pagination.ParseToken(pageToken)
	if err != nil {
		return 0, "", "", "", 0, err
	}

	if len(tokens) != 5 {
		return 0, "", "", "", 0, pagination.InvalidToken(errors.Fmt("expected 5 components, got %d", len(tokens)))
	}
	afterStartHourUnix, err = strconv.Atoi(tokens[0])
	if err != nil {
		return 0, "", "", "", 0, pagination.InvalidToken(errors.New("expect the first page_token component to be an integer"))
	}
	afterNominalStartPosition, err = strconv.Atoi(tokens[4])
	if err != nil {
		return 0, "", "", "", 0, pagination.InvalidToken(errors.New("expect the fifth page_token component to be an integer"))
	}
	return afterStartHourUnix, tokens[1], tokens[2], tokens[3], afterNominalStartPosition, nil
}

// ReadChangepointGroupSummaries reads summaries of changepoint groups started at a week which is within the last 90 days.
func (c *Client) ReadChangepointGroupSummaries(ctx context.Context, opts ReadChangepointGroupSummariesOptions) (groups []*GroupSummary, nextPageToken string, err error) {
	var afterStartHourUnix, afterNominalStartPosition int
	var afterTestID, afterVariantHash, afterRefHash string
	paginationFilter := "(TRUE)"
	if opts.PageToken != "" {
		afterStartHourUnix, afterTestID, afterVariantHash, afterRefHash, afterNominalStartPosition, err = parseReadChangepointGroupSummariesPageToken(opts.PageToken)
		if err != nil {
			return nil, "", err
		}
		paginationFilter = `(
			UNIX_SECONDS(CanonicalChangepoint.StartHour) < @afterStartHourUnix
				OR (UNIX_SECONDS(CanonicalChangepoint.StartHour) = @afterStartHourUnix AND CanonicalChangepoint.TestID > @afterTestID)
				OR (UNIX_SECONDS(CanonicalChangepoint.StartHour) = @afterStartHourUnix AND CanonicalChangepoint.TestID = @afterTestID AND CanonicalChangepoint.VariantHash > @afterVariantHash)
				OR (UNIX_SECONDS(CanonicalChangepoint.StartHour) = @afterStartHourUnix AND CanonicalChangepoint.TestID = @afterTestID
							AND CanonicalChangepoint.VariantHash = @afterVariantHash AND CanonicalChangepoint.RefHash > @afterRefHash)
				OR (UNIX_SECONDS(CanonicalChangepoint.StartHour) = @afterStartHourUnix AND CanonicalChangepoint.TestID = @afterTestID
							AND CanonicalChangepoint.VariantHash = @afterVariantHash AND CanonicalChangepoint.RefHash = @afterRefHash AND CanonicalChangepoint.NominalStartPosition < @afterNominalStartPosition)
			)`
	}
	testIDContainFilter := "(TRUE)"
	if opts.TestIDContain != "" {
		testIDContainFilter = "(CONTAINS_SUBSTR(test_id, @testIDContain ))"
	}

	aggregateRateCount := func(name string) string {
		return `
		STRUCT(
				COUNTIF(` + name + ` >= 0.95) as Above95Percent,
				COUNTIF(` + name + ` < 0.05) as Less5Percent,
				COUNTIF(` + name + ` < 0.95 AND ` + name + ` >= 0.05) as Above5LessThan95Percent,
				avg(` + name + `) as Mean
			)
		`
	}
	query := `
		WITH
		latest AS (
			SELECT start_hour_week, MAX(version) as version
			FROM grouped_changepoints
			 -- Only return changepoints started at a week which is within the last 90 days.
			 -- Because we can only efficiently get the current source verdict unexpected rate
			 -- for test variant branches have unexpected result within the last 90 days.
			 -- That is join with the test_variant_segments_unexpected_realtime table.
			WHERE start_hour_week > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), interval 90 day)
			GROUP BY start_hour_week
		),
		grouped_changepoint_latest AS (
			SELECT g.*
			FROM grouped_changepoints g
			INNER JOIN latest l
				ON g.start_hour_week = l.start_hour_week
				AND g.version = l.version
		),
		filtered_grouped_changepoint_latest AS (
			SELECT *
			FROM grouped_changepoint_latest
			WHERE project = @project AND ` + testIDContainFilter + `
		),
		filtered_segments AS (
			SELECT *
			FROM test_variant_segments_unexpected_realtime
		WHERE project = @project AND ` + testIDContainFilter + `
		),
		grouped_changepoint_latest_with_current_rate as (
			select
				cp.*,
				SAFE_DIVIDE(seg.segments[0].counts.unexpected_verdicts, seg.segments[0].counts.total_verdicts) as latest_unexpected_source_verdict_rate
			FROM filtered_grouped_changepoint_latest cp
			INNER JOIN filtered_segments seg
        ON cp.project = seg.project
				AND cp.test_id = seg.test_id
				AND cp.variant_hash = seg.variant_hash
				AND cp.ref_hash = seg.ref_hash
		)
		SELECT
			ARRAY_AGG(struct(
				project AS Project,
				test_id AS TestID,
				variant_hash AS VariantHash,
				ref_hash AS RefHash,
				ref AS Ref,
				variant AS Variant,
				start_hour AS StartHour,
				start_position_lower_bound_99th AS LowerBound99th,
				start_position AS NominalStartPosition,
				start_position_upper_bound_99th AS UpperBound99th,
				previous_nominal_end_position as PreviousNominalEndPosition
			) ORDER BY project, test_id, variant_hash, ref_hash, start_position LIMIT 1)[OFFSET(0)] as CanonicalChangepoint,
			COUNT(*) as Total,
			` + aggregateRateCount("previous_unexpected_source_verdict_rate") + ` as UnexpectedSourceVerdictRateBefore,
			` + aggregateRateCount("unexpected_source_verdict_rate") + ` as UnexpectedSourceVerdictRateAfter,
			` + aggregateRateCount("latest_unexpected_source_verdict_rate") + ` as UnexpectedSourceVerdictRateCurrent,
			struct (
				COUNTIF(unexpected_source_verdict_rate- previous_unexpected_source_verdict_rate < 0.2 ) as Increase0to20percent,
				COUNTIF(unexpected_source_verdict_rate- previous_unexpected_source_verdict_rate >= 0.2 AND
					unexpected_source_verdict_rate- previous_unexpected_source_verdict_rate < 0.5 ) as Increase20to50percent,
				COUNTIF(unexpected_source_verdict_rate- previous_unexpected_source_verdict_rate >= 0.5 ) as Increase50to100percent
			) as UnexpectedSourveVerdictRateChange
		FROM grouped_changepoint_latest_with_current_rate
		GROUP BY group_id
		HAVING ` + paginationFilter + `
		ORDER BY CanonicalChangepoint.StartHour DESC, CanonicalChangepoint.TestID, CanonicalChangepoint.VariantHash, CanonicalChangepoint.RefHash, CanonicalChangepoint.NominalStartPosition DESC
		LIMIT @limit
`
	q := c.client.Query(query)
	q.DefaultDatasetID = "internal"
	q.Parameters = []bigquery.QueryParameter{
		{Name: "project", Value: opts.Project},
		{Name: "testIDContain", Value: opts.TestIDContain},
		{Name: "afterStartHourUnix", Value: afterStartHourUnix},
		{Name: "afterTestID", Value: afterTestID},
		{Name: "afterVariantHash", Value: afterVariantHash},
		{Name: "afterRefHash", Value: afterRefHash},
		{Name: "afterNominalStartPosition", Value: afterNominalStartPosition},
		{Name: "limit", Value: opts.PageSize},
	}
	it, err := q.Read(ctx)
	if err != nil {
		return nil, "", errors.Fmt("read query results: %w", err)
	}
	results := []*GroupSummary{}
	for {
		row := &GroupSummary{}
		err := it.Next(row)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, "", errors.Fmt("obtain next changepoint group row: %w", err)
		}
		results = append(results, row)
	}
	if opts.PageSize != 0 && len(results) == int(opts.PageSize) {
		lastCanonicalChangepoint := results[len(results)-1].CanonicalChangepoint
		nextPageToken = pagination.Token(fmt.Sprint(lastCanonicalChangepoint.StartHour.Unix()), lastCanonicalChangepoint.TestID, lastCanonicalChangepoint.VariantHash, lastCanonicalChangepoint.RefHash, fmt.Sprint(lastCanonicalChangepoint.NominalStartPosition))
	}
	return results, nextPageToken, nil
}

type ReadChangepointsInGroupOptions struct {
	Project       string
	TestIDContain string
	// TestID, VariantHash, RefHash and StartPosition define a changepoint.
	TestID        string
	VariantHash   string
	RefHash       string
	StartPosition int64
}

// ReadChangepointsInGroup read changepoints in the same group as the changepoint specified in option.
// At most 1000 changepoints are returned, ordered by NominalStartPosition DESC, TestID, VariantHash, RefHash.
func (c *Client) ReadChangepointsInGroup(ctx context.Context, opts ReadChangepointsInGroupOptions) (changepoints []*ChangepointRow, err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/changepoints.ReadChangepointsInGroup")
	defer func() { tracing.End(s, err) }()

	testIDContainFilter := "(TRUE)"
	if opts.TestIDContain != "" {
		testIDContainFilter = "(CONTAINS_SUBSTR(test_id, @testIDContain ))"
	}
	query := `
		WITH
			latest AS (
				SELECT start_hour_week, MAX(version) as version
				FROM grouped_changepoints
				GROUP BY start_hour_week
			),
			grouped_changepoint_latest AS (
				SELECT g.*
				FROM grouped_changepoints g
				INNER JOIN latest l
					ON g.start_hour_week = l.start_hour_week
					AND g.version = l.version
			),
			select_group as (
				SELECT group_id
				FROM grouped_changepoint_latest
				WHERE project = @project
				AND test_id = @testID
				AND variant_hash = @variantHash
				AND ref_hash = @refHash
				AND start_position_upper_bound_99th >= @startPosition
				AND start_position_lower_bound_99th <= @startPosition
				ORDER BY ABS(start_position - @startPosition)
				LIMIT 1
			)
		SELECT
			project AS Project,
			test_id AS TestID,
			variant_hash AS VariantHash,
			ref_hash AS RefHash,
			ref AS Ref,
			variant AS Variant,
			start_hour AS StartHour,
			start_position_lower_bound_99th AS LowerBound99th,
			start_position AS NominalStartPosition,
			start_position_upper_bound_99th AS UpperBound99th,
			previous_nominal_end_position as PreviousNominalEndPosition
		FROM grouped_changepoint_latest cp
		INNER JOIN select_group g
			ON cp.group_id = g.group_id
		WHERE project = @project AND ` + testIDContainFilter + `
		ORDER BY NominalStartPosition DESC, TestID, VariantHash, RefHash
		LIMIT 1000
		`

	q := c.client.Query(query)
	q.DefaultDatasetID = "internal"
	q.Parameters = []bigquery.QueryParameter{
		{Name: "project", Value: opts.Project},
		{Name: "testID", Value: opts.TestID},
		{Name: "variantHash", Value: opts.VariantHash},
		{Name: "refHash", Value: opts.RefHash},
		{Name: "startPosition", Value: opts.StartPosition},
		{Name: "testIDContain", Value: opts.TestIDContain},
	}
	it, err := q.Read(ctx)
	if err != nil {
		return nil, errors.Fmt("read query results: %w", err)
	}
	results := []*ChangepointRow{}
	for {
		row := &ChangepointRow{}
		err := it.Next(row)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, errors.Fmt("obtain next changepoint row: %w", err)
		}
		results = append(results, row)
	}
	return results, nil
}
