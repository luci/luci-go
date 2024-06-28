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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/analysis/internal/bqutil"
)

// NewReadClient creates a new client for reading test results BigQuery table.
func NewReadClient(ctx context.Context, gcpProject string) (*ReadClient, error) {
	client, err := bqutil.Client(ctx, gcpProject)
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

type ReadTestVerdictsPerSourcePositionOptions struct {
	Project     string
	TestID      string
	VariantHash string
	RefHash     string
	// Only test verdicts with allowed invocation realms can be returned.
	AllowedRealms []string
	// All returned commits has source position greater than PositionMustGreater.
	PositionMustGreater int64
	// The maximum number of commits to return.
	NumCommits int64
}

// CommitWithVerdicts represents a commit with test verdicts.
type CommitWithVerdicts struct {
	// Source position of this commit.
	Position int64
	// Commit hash of this commit.
	CommitHash string
	// Represent a branch in the source control.
	Ref *BQRef
	// Realm of test verdicts at this commit.
	Realm string
	// Returns at most 20 test verdicts at this commit.
	TestVerdicts []*BQTestVerdict
}

type BQTestVerdict struct {
	TestID                string
	VariantHash           string
	RefHash               string
	InvocationID          string
	Status                string
	PartitionTime         time.Time
	PassedAvgDurationUsec bigquery.NullFloat64
	Changelists           []*BQChangelist
	// Whether the caller has access to this test verdict.
	HasAccess bool
}

type BQRef struct {
	Gitiles *BQGitiles
}
type BQGitiles struct {
	Host    bigquery.NullString
	Project bigquery.NullString
	Ref     bigquery.NullString
}

type BQChangelist struct {
	Host      bigquery.NullString
	Change    bigquery.NullInt64
	Patchset  bigquery.NullInt64
	OwnerKind bigquery.NullString
}

// ReadTestVerdictsPerSourcePosition returns commits with test verdicts in source position ascending order.
// Only return commits within the last 90 days.
func (c *ReadClient) ReadTestVerdictsPerSourcePosition(ctx context.Context, options ReadTestVerdictsPerSourcePositionOptions) ([]*CommitWithVerdicts, error) {
	query := `
	WITH
		test_verdicts_precompute AS (
			SELECT
				invocation.id AS invocation_id,
				-- All of these should be the same for all test results in a verdict.
				ANY_VALUE(invocation.realm) AS realm,
				ANY_VALUE(sources) AS sources,
				ANY_VALUE(source_ref) AS source_ref,
				ANY_VALUE(partition_time) as partition_time,
				-- For computing the test verdict.
				COUNTIF(expected) AS expected_count,
				COUNTIF(NOT COALESCE(expected, FALSE)) AS unexpected_count,
				COUNTIF(status = "SKIP") AS skipped_count,
				COUNT(*) AS total_count,
				AVG(IF(status = "PASS",duration_secs, NULL)) AS avg_passed_duration_sec,
			FROM test_results
			WHERE
				project = @project
				AND test_id = @testID
				AND variant_hash = @variantHash
				AND source_ref_hash = @refHash
				AND sources.gitiles_commit.position > @positionMustGreater
				-- Filter out dirty sources, these results are always ignored by changepoint analysis.
				AND not sources.is_dirty
				-- Limit to 90 days of test results to improve query performance, given that the users are most likely not interested in old data.
				AND partition_time > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 90 day)
			GROUP BY invocation.id
		),
		-- Compute test verdicts from test_results table.
		test_verdicts_with_status AS (
			SELECT
				*,
				-- We don't have the exoneration information in the test_results table.
				-- Possible verdicts are EXPECTED, UNEXPECTEDLY_SKIPPED, UNEXPECTED and FLAKY.
				CASE
					WHEN expected_count = total_count THEN 'EXPECTED'
					WHEN unexpected_count = skipped_count AND skipped_count = total_count THEN 'UNEXPECTEDLY_SKIPPED'
					WHEN unexpected_count = total_count THEN 'UNEXPECTED'
					ELSE 'FLAKY'
				END AS status,
			FROM test_verdicts_precompute
		)
	SELECT
		sources.gitiles_commit.position AS Position,
		ANY_VALUE(sources.gitiles_commit.commit_hash) AS CommitHash,
		ANY_VALUE(source_ref) AS Ref,
		ANY_VALUE(realm) AS Realm,
		ARRAY_AGG(STRUCT(
									@testID as TestID,
									@variantHash as VariantHash,
									@refHash as RefHash,
									invocation_id AS InvocationID,
									status AS Status,
									avg_passed_duration_sec AS PassedAvgDurationUsec,
									(SELECT ARRAY_AGG(STRUCT(host AS Host, change AS Change, patchset AS Patchset, owner_kind as OwnerKind)) FROM UNNEST(sources.changelists)) AS Changelists,
									realm IN UNNEST(@allowedRealms) as HasAccess)
							ORDER BY partition_time DESC LIMIT 20) AS TestVerdicts
	FROM test_verdicts_with_status
	GROUP BY sources.gitiles_commit.position
	ORDER BY sources.gitiles_commit.position
	LIMIT @limit
	`
	q := c.client.Query(query)
	q.DefaultDatasetID = "internal"
	q.Parameters = []bigquery.QueryParameter{
		{Name: "project", Value: options.Project},
		{Name: "testID", Value: options.TestID},
		{Name: "variantHash", Value: options.VariantHash},
		{Name: "refHash", Value: options.RefHash},
		{Name: "positionMustGreater", Value: options.PositionMustGreater},
		{Name: "limit", Value: options.NumCommits},
		{Name: "allowedRealms", Value: options.AllowedRealms},
	}
	it, err := q.Read(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "read query results").Err()
	}
	results := []*CommitWithVerdicts{}
	for {
		row := &CommitWithVerdicts{}
		err := it.Next(row)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, errors.Annotate(err, "obtain next commit with test verdicts row").Err()
		}
		results = append(results, row)
	}
	return results, nil
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
				AND sources.gitiles_commit.position <= @startSourcePosition
				AND sources.gitiles_commit.position > @endSourcePosition
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
		sources.gitiles_commit.position AS Position,
		ARRAY_AGG(STRUCT(
								invocation_id AS InvocationID,
								partition_time AS PartitionTime,
								status AS Status,
								(
									SELECT
										ARRAY_AGG(STRUCT(host AS Host, change AS Change, patchset AS Patchset, owner_kind as OwnerKind))
									FROM UNNEST(sources.changelists)
								) AS Changelists
							)	ORDER BY partition_time LIMIT 20) AS TestVerdicts
	FROM test_verdicts_with_status
	GROUP BY sources.gitiles_commit.position
	ORDER BY sources.gitiles_commit.position DESC
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
		return nil, errors.Annotate(err, "running query").Err()
	}

	results := []SourceVerdict{}
	for {
		row := SourceVerdict{}
		err := it.Next(&row)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, errors.Annotate(err, "read next source verdict").Err()
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
