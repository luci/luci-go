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

package testresults

import (
	"context"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth/realms"
)

// NewReadClient creates a new client for reading test results BigQuery table.
func NewReadClient(ctx context.Context, gcpProject string) (*ReadClient, error) {
	client, err := bq.NewClient(ctx, gcpProject)
	if err != nil {
		return nil, err
	}
	return &ReadClient{client: client}, nil
}

// ReadClient represents a client to read test results table from BigQuery.
type ReadClient struct {
	client *bigquery.Client
}

// Close releases any resources held by the client.
func (c *ReadClient) Close() error {
	return c.client.Close()
}

type BQChangelist struct {
	Host      bigquery.NullString
	Change    bigquery.NullInt64
	Patchset  bigquery.NullInt64
	OwnerKind bigquery.NullString
}

type ReadSourceVerdictsOptions struct {
	Project     string
	TestID      string
	VariantHash string
	RefHash     string
	// Only test verdicts with allowed invocation subrealms can be returned.
	AllowedSubrealms []string
	// The maximum source position to return, inclusive.
	StartSourcePosition int64
	// The minimum source position to return, exclusive.
	EndSourcePosition int64
	// The last partition time to include in the results, exclusive.
	EndPartitionTime time.Time
}

// SourceVerdict aggregates all test results at a source position.
type SourceVerdict struct {
	// The source position.
	Position int64
	// Test verdicts at the position. Limited to 20.
	Verdicts []SourceVerdictTestVerdict
}

// SourceVerdictTestVerdict is a test verdict that is part of a source verdict.
type SourceVerdictTestVerdict struct {
	// The invocation for which the verdict is.
	InvocationID string
	// Partition time of the test verdict.
	PartitionTime time.Time
	// Status is one of SKIPPED, EXPECTED, UNEXPECTED, FLAKY.
	Status string
	// Changelists tested by the verdict.
	Changelists []BQChangelist
}

// ReadSourceVerdicts reads source verdicts on the specified test variant
// branch, for a nominated source position range.
func (c *ReadClient) ReadSourceVerdicts(ctx context.Context, options ReadSourceVerdictsOptions) ([]SourceVerdict, error) {
	query := `
	WITH
		test_verdicts_precompute AS (
			SELECT
				invocation.id AS invocation_id,
				-- All of these should be the same for all test results in a verdict.
				ANY_VALUE(sources) AS sources,
				ANY_VALUE(partition_time) as partition_time,
				-- For computing the test verdict.
				COUNTIF(status <> "SKIP" AND expected) AS expected_unskipped_count,
				COUNTIF(status <> "SKIP" AND NOT COALESCE(expected, FALSE)) AS unexpected_unskipped_count,
			FROM test_results
			WHERE
				project = @project
				AND test_id = @testID
				AND variant_hash = @variantHash
				AND source_ref_hash = @refHash
				AND COALESCE(sources.gitiles_commit.position, sources.submitted_android_build.build_id) <= @startSourcePosition
				AND COALESCE(sources.gitiles_commit.position, sources.submitted_android_build.build_id) > @endSourcePosition
				AND partition_time < @partitionTimeMax
				-- Filter out dirty sources, these results are always ignored by changepoint analysis.
				AND not sources.is_dirty
				-- Limit to realms we can see.
				AND invocation.realm IN UNNEST(@allowedRealms)
			GROUP BY invocation.id
		),
		-- Compute test verdicts from test_results table.
		test_verdicts_with_status AS (
			SELECT
				*,
				-- Compute the status of the test verdict as seen by changepoint analysis.
				-- Possible verdicts are EXPECTED, UNEXPECTEDLY_SKIPPED, UNEXPECTED and SKIPPED.
				CASE
					WHEN (expected_unskipped_count + unexpected_unskipped_count) = 0 THEN 'SKIPPED'
					WHEN expected_unskipped_count > 0 AND unexpected_unskipped_count > 0 THEN 'FLAKY'
					WHEN expected_unskipped_count > 0 THEN 'EXPECTED'
					ELSE 'UNEXPECTED'
				END AS status,
			FROM test_verdicts_precompute
		)
	SELECT
		COALESCE(sources.gitiles_commit.position, sources.submitted_android_build.build_id) AS Position,
		ARRAY_AGG(STRUCT(
								invocation_id AS InvocationID,
								partition_time AS PartitionTime,
								status AS Status,
								(
									SELECT
										ARRAY_AGG(STRUCT(host AS Host, change AS Change, patchset AS Patchset, owner_kind as OwnerKind))
									FROM UNNEST(sources.changelists)
								) AS Changelists
							)	ORDER BY partition_time LIMIT 20) AS Verdicts
	FROM test_verdicts_with_status
	GROUP BY 1
	ORDER BY 1 DESC
	`
	q := c.client.Query(query)
	q.DefaultDatasetID = "internal"
	q.Parameters = []bigquery.QueryParameter{
		{Name: "project", Value: options.Project},
		{Name: "testID", Value: options.TestID},
		{Name: "variantHash", Value: options.VariantHash},
		{Name: "refHash", Value: options.RefHash},
		{Name: "startSourcePosition", Value: options.StartSourcePosition},
		{Name: "endSourcePosition", Value: options.EndSourcePosition},
		{Name: "partitionTimeMax", Value: options.EndPartitionTime},
		{Name: "allowedRealms", Value: subrealmsToRealms(options.Project, options.AllowedSubrealms)},
	}
	it, err := q.Read(ctx)
	if err != nil {
		return nil, errors.Fmt("running query: %w", err)
	}

	results := []SourceVerdict{}
	for {
		row := SourceVerdict{}
		err := it.Next(&row)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, errors.Fmt("read next source verdict: %w", err)
		}
		results = append(results, row)
	}
	return results, nil
}

func subrealmsToRealms(project string, subrealms []string) []string {
	var results []string
	for _, subrealm := range subrealms {
		results = append(results, realms.Join(project, subrealm))
	}
	return results
}
