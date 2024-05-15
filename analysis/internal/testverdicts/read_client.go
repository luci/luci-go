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

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/bqutil"
)

// NewReadClient creates a new client for reading test verdicts.
func NewReadClient(ctx context.Context, gcpProject string) (*ReadClient, error) {
	client, err := bqutil.Client(ctx, gcpProject)
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
	Ref *Ref
	// Returns at most 20 test verdicts at this commit.
	TestVerdicts []*TestVerdict
}
type TestVerdict struct {
	TestID                string
	VariantHash           string
	RefHash               string
	InvocationID          string
	Status                string
	PartitionTime         time.Time
	PassedAvgDurationUsec bigquery.NullFloat64
	Changelists           []*Changelist
	// Whether the caller has access to this test verdict.
	HasAccess bool
}

type Ref struct {
	Gitiles *Gitiles
}
type Gitiles struct {
	Host    bigquery.NullString
	Project bigquery.NullString
	Ref     bigquery.NullString
}

type Changelist struct {
	Host      bigquery.NullString
	Change    bigquery.NullInt64
	Patchset  bigquery.NullInt64
	OwnerKind bigquery.NullString
}

// ReadTestVerdictsPerSourcePosition returns commits with test verdicts in source position ascending order.
// Only return commits within the last 90 days.
func (c *ReadClient) ReadTestVerdictsPerSourcePosition(ctx context.Context, options ReadTestVerdictsPerSourcePositionOptions) ([]*CommitWithVerdicts, error) {
	query := `
	SELECT
		sources.gitiles_commit.position as Position,
		ANY_VALUE(sources.gitiles_commit.commit_hash) as CommitHash,
		ANY_VALUE(source_ref) as Ref,
		ARRAY_AGG(STRUCT(test_id as TestID ,
									variant_hash as VariantHash,
									source_ref_hash as RefHash,
									invocation.id as InvocationID,
									status as Status,
									(SELECT AVG(IF(r.status = "PASS",r.duration , NULL)) FROM UNNEST(results) as r) as PassedAvgDurationUsec,
									(SELECT ARRAY_AGG(STRUCT(host as Host, change as Change, patchset as Patchset, owner_kind as OwnerKind)) FROM UNNEST(sources.changelists)) as Changelists,
									invocation.realm IN UNNEST(@allowedRealms) as HasAccess,
									partition_time as PartitionTime) ORDER BY partition_time DESC LIMIT 20) as TestVerdicts
	FROM internal.test_verdicts
	WHERE project = @project
		AND test_id = @testID
		AND variant_hash = @variantHash
		AND source_ref_hash = @refHash
		AND sources.gitiles_commit.position > @positionMustGreater
		AND partition_time > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 90 day)
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
	job, err := q.Run(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "running query").Err()
	}
	it, err := job.Read(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "read").Err()
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
