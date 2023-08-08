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
	"fmt"
	"net/http"

	"cloud.google.com/go/bigquery"
	"go.chromium.org/luci/bisection/model"
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

func (c *Client) ReadTestFailures(ctx context.Context, task *tpb.TestFailureDetectionTask) ([]*BuilderRegressionGroup, error) {
	bbTableName, err := buildBucketBuildTableName(task.Project)
	if err != nil {
		return nil, errors.Annotate(err, "buildBucketBuildTableName").Err()
	}
	q := c.client.Query(`
	WITH
  segments_with_failure_rate AS (
    SELECT
      *,
      ( segments[0].counts.unexpected_results / segments[0].counts.total_results) AS current_failure_rate,
      ( segments[1].counts.unexpected_results / segments[1].counts.total_results) AS previous_failure_rate,
      segments[0].start_position AS nominal_upper,
      segments[1].end_position AS nominal_lower,
      STRING(variant.builder) AS builder
    FROM test_variant_segments_unexpected_realtime
    WHERE ARRAY_LENGTH(segments) > 1
  ),
  builder_regression_groups AS (
    SELECT
      ref_hash AS RefHash,
      ANY_VALUE(ref) AS Ref,
      nominal_lower AS RegressionStartPosition,
      nominal_upper AS RegressionEndPosition,
      ANY_VALUE(previous_failure_rate) AS StartPositionFailureRate,
      ANY_VALUE(current_failure_rate) AS EndPositionFailureRate,
      ARRAY_AGG(STRUCT(test_id AS TestId, variant_hash AS VariantHash,variant AS Variant) ORDER BY test_id, variant_hash) AS TestVariants,
      ANY_VALUE(segments[0].start_hour) AS StartHour
    FROM segments_with_failure_rate
    WHERE
      current_failure_rate = 1
      AND previous_failure_rate = 0
      AND segments[0].counts.unexpected_passed_results = 0
      AND segments[0].start_hour >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
    GROUP BY ref_hash, builder, nominal_lower, nominal_upper
  ),
  builder_regression_groups_with_latest_build AS (
    SELECT
      ANY_VALUE(g) AS regression_group,
      ANY_VALUE(v.buildbucket_build.id HAVING MAX v.partition_time) AS build_id,
      ANY_VALUE(b.infra.swarming.task_dimensions HAVING MAX v.partition_time) AS task_dimensions,
		FROM builder_regression_groups g
		-- Join with test_verdict table to get the build id of the lastest build for a test variant.
		LEFT JOIN test_verdicts v
		ON g.testVariants[0].TestId = v.test_id
			AND g.testVariants[0].VariantHash = v.variant_hash
			AND g.RefHash = v.source_ref_hash
		-- Join with buildbucket builds table to get the build OS for tests.
		LEFT JOIN ` + bbTableName + ` b
		ON v.buildbucket_build.id  = b.id
		-- Filter by test_verdict.partition_time to only return test failures that have test verdict recently.
		-- 3 days is chosen as we expect tests run at least once every 3 days if they are not disabled.
		-- If this is found to be too restricted, we can increase it later.
		WHERE v.partition_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 3 DAY)
		      AND b.create_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 3 DAY)
		GROUP BY g.testVariants[0].TestId,  g.testVariants[0].VariantHash, g.RefHash
  )
	SELECT
		regression_group.*,
		build_id as BuildID,
		(SELECT value FROM UNNEST(task_dimensions) WHERE KEY = "os" ) AS OS
	FROM builder_regression_groups_with_latest_build
	WHERE  (SELECT LOGICAL_AND((SELECT count(*) > 0 FROM UNNEST(task_dimensions) WHERE KEY = kv.key and value = kv.value)) FROM UNNEST(@dimensionContains) kv)
	ORDER BY regression_group.RegressionStartPosition DESC
	LIMIT 5000
 `)
	q.DefaultDatasetID = task.Project
	q.DefaultProjectID = c.luciAnalysisProject
	q.Parameters = []bigquery.QueryParameter{
		{Name: "dimensionContains", Value: task.DimensionContains},
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

const BuildBucketProject = "cr-buildbucket"

// This returns a qualified BigQuary table name of the builds table
// in BuildBucket for a LUCI project.
// The table name is checked against SQL-Injection.
// Thus, it can be injected into a SQL query.
func buildBucketBuildTableName(luciProject string) (string, error) {
	// Revalidate project as safeguard against SQL-Injection.
	if err := util.ValidateProject(luciProject); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.%s.builds", BuildBucketProject, luciProject), nil
}

type BuildInfo struct {
	BuildID         int64
	Bucket          string
	Builder         string
	StartCommitHash string
	EndCommitHash   string
}

func (c *Client) ReadBuildInfo(ctx context.Context, tf *model.TestFailure) (BuildInfo, error) {
	q := c.client.Query(`
	SELECT
		ANY_VALUE(buildbucket_build.id) AS BuildID,
		ANY_VALUE(buildbucket_build.builder.bucket) AS Bucket,
		ANY_VALUE(buildbucket_build.builder.builder) AS Builder,
		ANY_VALUE(sources.gitiles_commit.commit_hash) AS CommitHash,
		sources.gitiles_commit.position as Position
	FROM test_verdicts
	WHERE test_id = @testID
		AND variant_hash = @variantHash
		AND source_ref_hash = @refHash
		AND sources.gitiles_commit.position in (@startPosition, @endPosition)
		AND partition_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
	GROUP BY sources.gitiles_commit.position
	ORDER BY sources.gitiles_commit.position DESC
`)
	q.DefaultDatasetID = tf.Project
	q.DefaultProjectID = c.luciAnalysisProject
	q.Parameters = []bigquery.QueryParameter{
		{Name: "testID", Value: tf.TestID},
		{Name: "variantHash", Value: tf.VariantHash},
		{Name: "refHash", Value: tf.RefHash},
		{Name: "startPosition", Value: tf.RegressionStartPosition},
		{Name: "endPosition", Value: tf.RegressionEndPosition},
	}
	job, err := q.Run(ctx)
	if err != nil {
		return BuildInfo{}, errors.Annotate(err, "querying test_verdicts").Err()
	}
	it, err := job.Read(ctx)
	if err != nil {
		return BuildInfo{}, err
	}
	rowVals := map[string]bigquery.Value{}
	// First row is for regression end position.
	err = it.Next(&rowVals)
	if err != nil {
		return BuildInfo{}, errors.Annotate(err, "read build info row for regression end position").Err()
	}
	// Make sure the first row is for the end position.
	if rowVals["Position"].(int64) != tf.RegressionEndPosition {
		return BuildInfo{}, errors.New("position should equal to RegressionEndPosition. this suggests something wrong with the query.")
	}
	buildInfo := BuildInfo{
		BuildID:       rowVals["BuildID"].(int64),
		Bucket:        rowVals["Bucket"].(string),
		Builder:       rowVals["Builder"].(string),
		EndCommitHash: rowVals["CommitHash"].(string),
	}
	buildInfo.StartCommitHash = rowVals["CommitHash"].(string)
	// Second row is for regression start position.
	err = it.Next(&rowVals)
	if err != nil {
		return BuildInfo{}, errors.Annotate(err, "read build info row for regression start position").Err()
	}
	// Make sure the second row is for the start position.
	if rowVals["Position"].(int64) != tf.RegressionStartPosition {
		return BuildInfo{}, errors.New("position should equal to RegressionStartPosition. this suggests something wrong with the query.")
	}
	buildInfo.StartCommitHash = rowVals["CommitHash"].(string)
	return buildInfo, nil
}
