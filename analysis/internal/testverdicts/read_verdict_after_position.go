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

package testverdicts

import (
	"context"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
)

// NewReadClient creates a new client for reading from test verdicts BigQuery table.
func NewReadClient(ctx context.Context, gcpProject string) (*ReadClient, error) {
	client, err := bq.NewClient(ctx, gcpProject)
	if err != nil {
		return nil, err
	}
	return &ReadClient{client: client}, nil
}

// ReadClient represents a client to read test verdicts from BigQuery.
type ReadClient struct {
	client *bigquery.Client
}

// Close releases any resources held by the client.
func (c *ReadClient) Close() error {
	return c.client.Close()
}

type ReadVerdictAtOrAfterPositionOptions struct {
	// The LUCI Project.
	Project string
	// The test identifier.
	TestID string
	// The variant hash, 16 hex characters.
	VariantHash string
	// The source ref hash, 16 hex characters.
	RefHash string
	// The source position along the ref to search for.
	// The first verdict at or after this position will be returned.
	AtOrAfterPosition int64
	// The start of the partition time range to search, inclusive.
	PartitionTimeStart time.Time
	// The end of the parition time range to search, exclusive.
	PartitionTimeEnd time.Time
	// The LUCI realms we are allowed to access.
	AllowedRealms []string
}

type SourceVerdict struct {
	// Source position of this commit.
	Position int64
	// Commit hash of this commit.
	CommitHash string
	// The variant, as a JSON blob.
	Variant string
	// The location of the test, if available.
	TestLocation *TestLocation
	// Represent a branch in the source control.
	Ref *BQRef
	// A selection of test results at the position.
	// May contain 0 or 1 items.
	Results []TestResult
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

type TestLocation struct {
	// The repository, e.g. "https://chromium.googlesource.com/chromium/src".
	Repo string
	// The file name in the repository, e.g.
	// "//third_party/blink/web_tests/external/wpt/html/semantics/scripting-1/the-script-element/json-module-assertions/load-error-events.html".
	FileName string
}

type TestResult struct {
	ParentInvocationID string
	ResultID           string
	Expected           bool
	// One of the analysispb.TestResultStatus values.
	Status               string
	PrimaryFailureReason bigquery.NullString
}

// ReadTestVerdictAfterPosition returns the first source verdict after
// the given position on the given branch.
func (c *ReadClient) ReadTestVerdictAfterPosition(ctx context.Context, options ReadVerdictAtOrAfterPositionOptions) (*SourceVerdict, error) {
	query := `
	SELECT
		sources.gitiles_commit.position as Position,
		ANY_VALUE(sources.gitiles_commit.commit_hash) as CommitHash,
		ANY_VALUE(variant) as Variant,
		ANY_VALUE(STRUCT(
			test_metadata.location.file_name as FileName,
			test_metadata.location.repo as Repo
		)) as TestLocation,
		ANY_VALUE(source_ref) as Ref,
		ARRAY_AGG(STRUCT(
				result.parent.id as ParentInvocationID,
				result.result_id as ResultID,
				result.expected as Expected,
				result.status as Status,
				result.failure_reason.primary_error_message as PrimaryFailureReason
		) ORDER BY result.expected DESC, result.parent.id, result.result_id LIMIT 1) as Results,
	FROM internal.test_verdicts, UNNEST(results) result
	WHERE project = @project
		AND test_id = @testID
		AND variant_hash = @variantHash
		AND source_ref_hash = @refHash
		AND sources.gitiles_commit.position >= @atOrAfterPosition
		AND partition_time >= @partitionTimeStart
		AND partition_time < @partitionTimeEnd
		AND invocation.realm IN UNNEST(@allowedRealms)
	GROUP BY sources.gitiles_commit.position
	ORDER BY sources.gitiles_commit.position
	LIMIT 1
	`
	q := c.client.Query(query)
	q.DefaultDatasetID = "internal"
	q.Parameters = []bigquery.QueryParameter{
		{Name: "project", Value: options.Project},
		{Name: "testID", Value: options.TestID},
		{Name: "variantHash", Value: options.VariantHash},
		{Name: "refHash", Value: options.RefHash},
		{Name: "atOrAfterPosition", Value: options.AtOrAfterPosition},
		{Name: "partitionTimeStart", Value: options.PartitionTimeStart},
		{Name: "partitionTimeEnd", Value: options.PartitionTimeEnd},
		{Name: "allowedRealms", Value: options.AllowedRealms},
	}
	it, err := q.Read(ctx)
	if err != nil {
		return nil, errors.Fmt("running query: %w", err)
	}
	results := []*SourceVerdict{}
	for {
		row := &SourceVerdict{}
		err := it.Next(row)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, errors.Fmt("obtain next source position verdict: %w", err)
		}
		results = append(results, row)
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}
