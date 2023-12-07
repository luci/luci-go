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
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"text/template"

	"cloud.google.com/go/bigquery"
	"go.chromium.org/luci/bisection/model"
	configpb "go.chromium.org/luci/bisection/proto/config"
	pb "go.chromium.org/luci/bisection/proto/v1"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	"go.chromium.org/luci/server/auth"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

var readFailureTemplate = template.Must(template.New("").Parse(
	`
{{define "basic" -}}
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
      ANY_VALUE(segments[0].start_hour) AS StartHour,
      ANY_VALUE(segments[0].end_hour) AS EndHour
    FROM segments_with_failure_rate
    WHERE
      current_failure_rate = 1
      AND previous_failure_rate = 0
      AND segments[0].counts.unexpected_passed_results = 0
      AND segments[0].start_hour >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
      -- We only consider test failures with non-skipped result in the last 24 hour.
      AND segments[0].end_hour >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 24 HOUR)
    GROUP BY ref_hash, builder, nominal_lower, nominal_upper
  ),
  builder_regression_groups_with_latest_build AS (
    SELECT
      ANY_VALUE(g) AS regression_group,
      ANY_VALUE(v.buildbucket_build.id HAVING MAX v.partition_time) AS build_id,
      ANY_VALUE(REGEXP_EXTRACT(v.results[0].parent.id, r'^task-{{.SwarmingProject}}.appspot.com-([0-9a-f]+)$') HAVING MAX v.partition_time) AS swarming_run_id,
      ANY_VALUE(COALESCE(b2.infra.swarming.task_dimensions, b.infra.swarming.task_dimensions) HAVING MAX v.partition_time) AS task_dimensions,
      ANY_VALUE(b.builder.bucket HAVING MAX v.partition_time) AS bucket,
      ANY_VALUE(JSON_VALUE_ARRAY(b.input.properties, "$.sheriff_rotations") HAVING MAX v.partition_time) AS SheriffRotations,
      ANY_VALUE(JSON_VALUE(b.input.properties, "$.builder_group") HAVING MAX v.partition_time) AS BuilderGroup,
    FROM builder_regression_groups g
    -- Join with test_verdict table to get the build id of the lastest build for a test variant.
    LEFT JOIN test_verdicts v
    ON g.testVariants[0].TestId = v.test_id
      AND g.testVariants[0].VariantHash = v.variant_hash
      AND g.RefHash = v.source_ref_hash
    -- Join with buildbucket builds table to get the buildbucket related information for tests.
    LEFT JOIN (select * from {{.BBTableName}} where create_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 3 DAY)) b
    ON v.buildbucket_build.id  = b.id
    -- JOIN with buildbucket builds table again to get task dimensions of parent builds.
    LEFT JOIN (select * from {{.BBTableName}} where create_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 3 DAY)) b2
    ON JSON_VALUE(b.input.properties, "$.parent_build_id") = CAST(b2.id AS string)
    -- Filter by test_verdict.partition_time to only return test failures that have test verdict recently.
    -- 3 days is chosen as we expect tests run at least once every 3 days if they are not disabled.
    -- If this is found to be too restricted, we can increase it later.
    WHERE v.partition_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 3 DAY)
    GROUP BY g.testVariants[0].TestId,  g.testVariants[0].VariantHash, g.RefHash
  )
{{- if .ExcludedPools}}
{{- template "withExcludedPools" .}}
{{- else}}
{{- template "withoutExcludedPools" .}}
{{- end -}}
ORDER BY regression_group.RegressionEndPosition DESC
LIMIT 5000
{{- end}}

{{- define "withoutExcludedPools"}}
SELECT regression_group.*,
  -- use empty array instead of null so we can read into []NullString.
  IFNULL(SheriffRotations, []) as SheriffRotations
FROM builder_regression_groups_with_latest_build
WHERE {{.DimensionExcludeFilter}} AND (bucket NOT IN UNNEST(@excludedBuckets))
  -- We need to compare ARRAY_LENGTH with null because of unexpected Bigquery behaviour b/138262091.
  AND ((BuilderGroup IN UNNEST(@allowedBuilderGroups)) OR ARRAY_LENGTH(@allowedBuilderGroups) = 0 OR ARRAY_LENGTH(@allowedBuilderGroups) IS NULL)
  AND (BuilderGroup NOT IN UNNEST(@excludedBuilderGroups))
{{end}}

{{define "withExcludedPools"}}
SELECT regression_group.*,
  -- use empty array instead of null so we can read into []NullString.
  IFNULL(SheriffRotations, []) as SheriffRotations
FROM builder_regression_groups_with_latest_build g
LEFT JOIN {{.SwarmingProject}}.swarming.task_results_run s
ON g.swarming_run_id = s.run_id
WHERE s.end_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 3 DAY)
  AND {{.DimensionExcludeFilter}} AND (bucket NOT IN UNNEST(@excludedBuckets))
  AND (s.bot.pools[0] NOT IN UNNEST(@excludedPools))
  -- We need to compare ARRAY_LENGTH with null because of unexpected Bigquery behaviour b/138262091.
  AND ((BuilderGroup IN UNNEST(@allowedBuilderGroups)) OR ARRAY_LENGTH(@allowedBuilderGroups) = 0 OR ARRAY_LENGTH(@allowedBuilderGroups) IS NULL)
  AND (BuilderGroup NOT IN UNNEST(@excludedBuilderGroups))
{{end}}
	`))

// NewClient creates a new client for reading test failures from LUCI Analysis.
// Close() MUST be called after you have finished using this client.
// GCP project where the query operations are billed to, either luci-bisection or luci-bisection-dev.
// luciAnalysisProject is the function that returns the gcp project that contains the BigQuery table we want to query.
func NewClient(ctx context.Context, gcpProject string, luciAnalysisProjectFunc func(luciProject string) string) (*Client, error) {
	if gcpProject == "" {
		return nil, errors.New("GCP Project must be specified")
	}
	if luciAnalysisProjectFunc == nil {
		return nil, errors.New("LUCI Analysis Project function must be specified")
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
		client:                  client,
		luciAnalysisProjectFunc: luciAnalysisProjectFunc,
	}, nil
}

// Client may be used to read LUCI Analysis test failures.
type Client struct {
	client *bigquery.Client
	// luciAnalysisProjectFunc is a function that return LUCI Analysis project
	// given a LUCI Project.
	luciAnalysisProjectFunc func(luciProject string) string
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
	EndHour                  bigquery.NullTimestamp
	SheriffRotations         []bigquery.NullString
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

func (c *Client) ReadTestFailures(ctx context.Context, task *tpb.TestFailureDetectionTask, filter *configpb.FailureIngestionFilter) ([]*BuilderRegressionGroup, error) {
	dimensionExcludeFilter := "(TRUE)"
	if len(task.DimensionExcludes) > 0 {
		dimensionExcludeFilter = "(NOT (SELECT LOGICAL_OR((SELECT count(*) > 0 FROM UNNEST(task_dimensions) WHERE KEY = kv.key and value = kv.value)) FROM UNNEST(@dimensionExcludes) kv))"
	}

	queryStm, err := generateTestFailuresQuery(task, dimensionExcludeFilter, filter.ExcludedTestPools)
	if err != nil {
		return nil, errors.Annotate(err, "generate test failures query").Err()
	}
	q := c.client.Query(queryStm)
	q.DefaultDatasetID = task.Project
	q.DefaultProjectID = c.luciAnalysisProjectFunc(task.Project)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "dimensionExcludes", Value: task.DimensionExcludes},
		{Name: "excludedBuckets", Value: filter.GetExcludedBuckets()},
		{Name: "excludedPools", Value: filter.GetExcludedTestPools()},
		{Name: "allowedBuilderGroups", Value: filter.GetAllowedBuilderGroups()},
		{Name: "excludedBuilderGroups", Value: filter.GetExcludedBuilderGroups()},
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

func generateTestFailuresQuery(task *tpb.TestFailureDetectionTask, dimensionExcludeFilter string, excludedPools []string) (string, error) {
	bbTableName, err := buildBucketBuildTableName(task.Project)
	if err != nil {
		return "", errors.Annotate(err, "buildBucketBuildTableName").Err()
	}

	swarmingProject := ""
	switch task.Project {
	case "chromium":
		swarmingProject = "chromium-swarm"
	case "chrome":
		swarmingProject = "chrome-swarming"
	default:
		return "", errors.Reason("couldn't get swarming project for project %s", task.Project).Err()
	}

	var b bytes.Buffer
	err = readFailureTemplate.ExecuteTemplate(&b, "basic", map[string]any{
		"SwarmingProject":        swarmingProject,
		"DimensionExcludeFilter": dimensionExcludeFilter,
		"BBTableName":            bbTableName,
		"ExcludedPools":          excludedPools,
	})
	if err != nil {
		return "", errors.Annotate(err, "execute template").Err()
	}
	return b.String(), nil
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
		sources.gitiles_commit.position AS Position
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
	q.DefaultProjectID = c.luciAnalysisProjectFunc(tf.Project)
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

type TestVerdictKey struct {
	TestID      string
	VariantHash string
	RefHash     string
}

type TestVerdictResultRow struct {
	TestID      bigquery.NullString
	VariantHash bigquery.NullString
	RefHash     bigquery.NullString
	TestName    bigquery.NullString
	Status      bigquery.NullString
}

type TestVerdictResult struct {
	TestName string
	Status   pb.TestVerdictStatus
}

// ReadLatestVerdict queries LUCI Analysis for latest verdict.
// It supports querying for multiple keys at a time to save time and resources.
// Returns a map of TestVerdictKey -> latest verdict.
func (c *Client) ReadLatestVerdict(ctx context.Context, project string, keys []TestVerdictKey) (map[TestVerdictKey]TestVerdictResult, error) {
	if len(keys) == 0 {
		return nil, errors.New("no key specified")
	}
	err := validateTestVerdictKeys(keys)
	if err != nil {
		return nil, errors.Annotate(err, "validate keys").Err()
	}
	clauses := make([]string, len(keys))
	for i, key := range keys {
		clauses[i] = fmt.Sprintf("(test_id = %q AND variant_hash = %q AND source_ref_hash = %q)", key.TestID, key.VariantHash, key.RefHash)
	}
	whereClause := fmt.Sprintf("(%s)", strings.Join(clauses, " OR "))

	// We expect a test to have result in the last 3 days.
	// Set the partition time to 3 days to reduce the cost.
	query := `
		SELECT
			test_id AS TestID,
			variant_hash AS VariantHash,
			source_ref_hash AS RefHash,
			ARRAY_AGG (
				(	SELECT value FROM UNNEST(tv.results[0].tags) WHERE KEY = "test_name")
					ORDER BY tv.partition_time DESC
					LIMIT 1
				)[OFFSET(0)] AS TestName,
			ANY_VALUE(status HAVING MAX tv.partition_time) AS Status
		FROM test_verdicts tv
		WHERE ` + whereClause + `
		AND partition_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 3 DAY)
		GROUP BY test_id, variant_hash, source_ref_hash
 	`
	logging.Infof(ctx, "Running query %s", query)
	q := c.client.Query(query)
	q.DefaultDatasetID = project
	q.DefaultProjectID = c.luciAnalysisProjectFunc(project)
	job, err := q.Run(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "querying test name").Err()
	}
	it, err := job.Read(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "read").Err()
	}
	results := map[TestVerdictKey]TestVerdictResult{}
	for {
		row := &TestVerdictResultRow{}
		err := it.Next(row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, errors.Annotate(err, "obtain next row").Err()
		}
		key := TestVerdictKey{
			TestID:      row.TestID.String(),
			VariantHash: row.VariantHash.String(),
			RefHash:     row.RefHash.String(),
		}
		results[key] = TestVerdictResult{
			TestName: row.TestName.String(),
			Status:   pb.TestVerdictStatus(pb.TestVerdictStatus_value[row.Status.String()]),
		}
	}
	return results, nil
}

type CountRow struct {
	Count bigquery.NullInt64
}

// TestIsUnexpectedConsistently queries LUCI Analysis to see if a test is
// still unexpected deterministically since a commit position.
// This is to be called before we take a culprit action, in case a test
// status has changed.
func (c *Client) TestIsUnexpectedConsistently(ctx context.Context, project string, key TestVerdictKey, sinceCommitPosition int64) (bool, error) {
	err := validateTestVerdictKeys([]TestVerdictKey{key})
	if err != nil {
		return false, errors.Annotate(err, "validate keys").Err()
	}
	// If there is a row with counts.total_non_skipped > counts.unexpected_non_skipped,
	// It means there are some expected non skipped results.
	query := `
		SELECT
			COUNT(*) as count
		FROM test_verdicts
		WHERE test_id = @testID AND variant_hash = @variantHash AND source_ref_hash = @refHash
		AND counts.total_non_skipped > counts.unexpected_non_skipped
		AND sources.gitiles_commit.position > @sinceCommitPosition
		AND partition_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 3 DAY)
 	`
	logging.Infof(ctx, "Running query %s", query)
	q := c.client.Query(query)
	q.DefaultDatasetID = project
	q.DefaultProjectID = c.luciAnalysisProjectFunc(project)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "testID", Value: key.TestID},
		{Name: "variantHash", Value: key.VariantHash},
		{Name: "refHash", Value: key.RefHash},
		{Name: "sinceCommitPosition", Value: sinceCommitPosition},
	}

	job, err := q.Run(ctx)
	if err != nil {
		return false, errors.Annotate(err, "running query").Err()
	}
	it, err := job.Read(ctx)
	if err != nil {
		return false, errors.Annotate(err, "read").Err()
	}
	row := &CountRow{}
	err = it.Next(row)
	if err == iterator.Done {
		return false, errors.New("cannot get count")
	}
	if err != nil {
		return false, errors.Annotate(err, "obtain next row").Err()
	}
	return row.Count.Int64 == 0, nil
}

type ChangepointResult struct {
	TestID      string
	VariantHash string
	RefHash     string
	Segments    []*Segment
}
type Segment struct {
	StartPosition          bigquery.NullInt64
	EndPosition            bigquery.NullInt64
	CountTotalResults      bigquery.NullInt64
	CountUnexpectedResults bigquery.NullInt64
}

func (c *Client) ChangepointAnalysisForTestVariant(ctx context.Context, project string, keys []TestVerdictKey) (map[TestVerdictKey]*ChangepointResult, error) {
	err := validateTestVerdictKeys(keys)
	if err != nil {
		return nil, errors.Annotate(err, "validate keys").Err()
	}
	clauses := make([]string, len(keys))
	for i, key := range keys {
		clauses[i] = fmt.Sprintf("(test_id = %q AND variant_hash = %q AND ref_hash = %q)", key.TestID, key.VariantHash, key.RefHash)
	}
	whereClause := fmt.Sprintf("(%s)", strings.Join(clauses, " OR "))
	query := `
		SELECT
			test_id as TestID,
			variant_hash as VariantHash,
			ref_hash as RefHash,
			(SELECT
					ARRAY_AGG(STRUCT(
					s.start_position as StartPosition,
					s.end_position as EndPosition,
					s.counts.total_results as CountTotalResults,
					s.counts.unexpected_results as CountUnexpectedResults))
				FROM UNNEST(segments) s
			) AS Segments
		FROM test_variant_segments_unexpected_realtime
		WHERE ` + whereClause
	logging.Infof(ctx, "Running query %s", query)
	q := c.client.Query(query)
	q.DefaultDatasetID = project
	q.DefaultProjectID = c.luciAnalysisProjectFunc(project)

	job, err := q.Run(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "running query").Err()
	}
	it, err := job.Read(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "read").Err()
	}
	results := map[TestVerdictKey]*ChangepointResult{}
	for {
		row := &ChangepointResult{}
		err := it.Next(row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, errors.Annotate(err, "obtain next changepoint row").Err()
		}
		key := TestVerdictKey{
			TestID:      row.TestID,
			VariantHash: row.VariantHash,
			RefHash:     row.RefHash,
		}
		results[key] = row
	}
	return results, nil
}

func validateTestVerdictKeys(keys []TestVerdictKey) error {
	for _, key := range keys {
		if err := rdbpbutil.ValidateTestID(key.TestID); err != nil {
			return err
		}
		if err := util.ValidateVariantHash(key.VariantHash); err != nil {
			return err
		}
		if err := util.ValidateRefHash(key.RefHash); err != nil {
			return err
		}
	}
	return nil
}
