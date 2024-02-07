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
	Variant     bigquery.NullJSON
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

// ReadChangepoints reads changepoints of a certain week from BigQuery.
// The week parameter can be at any time of that week. A week is defined by Sunday to Satureday in UTC.
func (c *Client) ReadChangepoints(ctx context.Context, project string, week time.Time) ([]*ChangepointRow, error) {
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
			unexpected_verdict_rate AS UnexpectedVerdictRateAfter,
			previous_unexpected_verdict_rate AS UnexpectedVerdictRateBefore,
			latest_unexpected_verdict_rate AS UnexpectedVerdictRateCurrent,
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
