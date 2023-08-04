// Copyright 2023 The LUCI Authors.
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

// Package lucianalysis contains methods to query test failures maintained in BigQuery.
package lucianalysis

import (
	"context"
	"net/http"

	"cloud.google.com/go/bigquery"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// NewClient creates a new client for reading test failures from LUCI Analysis.
// Close() MUST be called after you have finished using this client.
// GCP project where the query operations are billed to, either luci-bisection or luci-bisection-dev.
// luciAnalysisProject is the gcp project that contains the BigQuery table we want to query.
func NewClient(ctx context.Context, gcpProject, luciAnalysisProject string) (*Client, error) {
	if gcpProject == "" {
		return nil, errors.New("GCP Project must be specified")
	}
	if luciAnalysisProject == "" {
		return nil, errors.New("LUCI analysis Project must be specified")
	}
	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(bigquery.Scope))
	if err != nil {
		return nil, err
	}
	client, err := bigquery.NewClient(ctx, gcpProject, option.WithHTTPClient(&http.Client{
		Transport: tr,
	}))
	if err != nil {
		return nil, err
	}
	return &Client{
		client:              client,
		luciAnalysisProject: luciAnalysisProject,
	}, nil
}

// Client may be used to read LUCI Analysis test failures.
type Client struct {
	client              *bigquery.Client
	luciAnalysisProject string
}

// Close releases any resources held by the client.
func (c *Client) Close() error {
	return c.client.Close()
}

// BuilderRegressionGroup contains a list of test variants
// which use the same builder and have the same regression range.
type BuilderRegressionGroup struct {
	RefHash                  bigquery.NullString
	Ref                      *Ref
	RegressionStartPosition  bigquery.NullInt64
	RegressionEndPosition    bigquery.NullInt64
	StartPositionFailureRate float64
	EndPositionFailureRate   float64
	TestVariants             []*TestVariant
	StartHour                bigquery.NullTimestamp
}

type Ref struct {
	Gitiles *Gitiles
}
type Gitiles struct {
	Host    bigquery.NullString
	Project bigquery.NullString
	Ref     bigquery.NullString
}

type TestVariant struct {
	TestID      bigquery.NullString
	VariantHash bigquery.NullString
	Variant     bigquery.NullJSON
}

type ReadTestFailuresOptions struct {
	// The LUCI Project, e.g. chromium.
	Project          string
	VariantPredicate *tpb.VariantPredicate
}

func (c *Client) ReadTestFailures(ctx context.Context, opts ReadTestFailuresOptions) ([]*BuilderRegressionGroup, error) {
	variantPredicateSQL := ""
	if opts.VariantPredicate.GetContains() != nil {
		variantPredicateSQL = `AND (SELECT LOGICAL_AND(kv IN UNNEST(SPLIT(REGEXP_REPLACE( TO_JSON_STRING(variant), r"[{} \"]", '')))) FROM UNNEST(@variantPredicate) kv)`
	}
	// TODO(beining): update query to filter by build OS.
	q := c.client.Query(`
	WITH
  segments_with_failure_rate AS (
    SELECT
      *,
      (
        segments[0].counts.unexpected_results
        / segments[0].counts.total_results) AS current_failure_rate,
      (
        segments[1].counts.unexpected_results
        / segments[1].counts.total_results) AS previous_failure_rate,
      segments[0].start_position AS nominal_upper,
      segments[1].end_position AS nominal_lower,
      STRING(variant.builder) AS builder
    FROM test_variant_segments_unexpected_realtime
    WHERE ARRAY_LENGTH(segments) > 1
  )
	SELECT
		ref_hash AS RefHash,
		ANY_VALUE(ref) AS Ref,
		nominal_lower AS RegressionStartPosition,
		nominal_upper AS RegressionEndPosition,
		ANY_VALUE(previous_failure_rate) as StartPositionFailureRate,
		ANY_VALUE(current_failure_rate) as EndPositionFailureRate,
		ARRAY_AGG(STRUCT(test_id AS TestId, variant_hash AS VariantHash, variant AS Variant)
																			ORDER BY test_id, variant_hash) AS TestVariants,
		ANY_VALUE(segments[0].start_hour) AS StartHour
	FROM segments_with_failure_rate
	WHERE
		current_failure_rate = 1
		AND previous_failure_rate = 0
		AND segments[0].counts.unexpected_passed_results = 0
		AND segments[0].start_hour
			>= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
    ` + variantPredicateSQL + `
	GROUP BY ref_hash, builder, nominal_lower, nominal_upper
	ORDER BY nominal_lower DESC
	LIMIT 5000
 `)
	q.DefaultDatasetID = opts.Project
	q.DefaultProjectID = c.luciAnalysisProject
	q.Parameters = []bigquery.QueryParameter{
		{Name: "variantPredicate", Value: util.VariantToStrings(opts.VariantPredicate.GetContains())},
	}
	job, err := q.Run(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "querying test failures").Err()
	}
	it, err := job.Read(ctx)
	if err != nil {
		return nil, err
	}
	groups := []*BuilderRegressionGroup{}
	for {
		row := &BuilderRegressionGroup{}
		err := it.Next(row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, errors.Annotate(err, "obtain next test failure group row").Err()
		}
		groups = append(groups, row)
	}
	return groups, nil
}
