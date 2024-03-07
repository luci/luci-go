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

// Package testfailuredetection analyses recent test failures with
// the changepoint analysis from LUCI analysis, and select test failures to bisect.
package lucianalysis

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	tpb "go.chromium.org/luci/bisection/task/proto"
)

func TestUpdateAnalysisStatus(t *testing.T) {
	t.Parallel()

	Convey("generate test failures query", t, func() {
		task := &tpb.TestFailureDetectionTask{
			Project: "chromium",
		}
		dimensionExcludeFilter := "(NOT (SELECT LOGICAL_OR((SELECT count(*) > 0 FROM UNNEST(task_dimensions) WHERE KEY = kv.key and value = kv.value)) FROM UNNEST(@dimensionExcludes) kv))"
		Convey("no excluded pools", func() {
			q, err := generateTestFailuresQuery(task, dimensionExcludeFilter, []string{})
			So(err, ShouldBeNil)
			So(q, ShouldEqual, `WITH
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
      v.buildbucket_build.builder.bucket,
      v.buildbucket_build.builder.builder,
      ANY_VALUE(g) AS regression_group,
      ANY_VALUE(v.buildbucket_build.id HAVING MAX v.partition_time) AS build_id,
      ANY_VALUE(REGEXP_EXTRACT(v.results[0].parent.id, r'^task-chromium-swarm.appspot.com-([0-9a-f]+)$') HAVING MAX v.partition_time) AS swarming_run_id,
      ANY_VALUE(COALESCE(b2.infra.swarming.task_dimensions, b2.infra.backend.task_dimensions, b.infra.swarming.task_dimensions, b.infra.backend.task_dimensions) HAVING MAX v.partition_time) AS task_dimensions,
      ANY_VALUE(JSON_VALUE_ARRAY(b.input.properties, "$.sheriff_rotations") HAVING MAX v.partition_time) AS SheriffRotations,
      ANY_VALUE(JSON_VALUE(b.input.properties, "$.builder_group") HAVING MAX v.partition_time) AS BuilderGroup,
    FROM builder_regression_groups g
    -- Join with test_verdict table to get the build id of the lastest build for a test variant.
    LEFT JOIN test_verdicts v
    ON g.testVariants[0].TestId = v.test_id
      AND g.testVariants[0].VariantHash = v.variant_hash
      AND g.RefHash = v.source_ref_hash
    -- Join with buildbucket builds table to get the buildbucket related information for tests.
    LEFT JOIN (select * from cr-buildbucket.chromium.builds where create_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 3 DAY)) b
    ON v.buildbucket_build.id  = b.id
    -- JOIN with buildbucket builds table again to get task dimensions of parent builds.
    LEFT JOIN (select * from cr-buildbucket.chromium.builds where create_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 3 DAY)) b2
    ON JSON_VALUE(b.input.properties, "$.parent_build_id") = CAST(b2.id AS string)
    -- Filter by test_verdict.partition_time to only return test failures that have test verdict recently.
    -- 3 days is chosen as we expect tests run at least once every 3 days if they are not disabled.
    -- If this is found to be too restricted, we can increase it later.
    WHERE v.partition_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 3 DAY)
    GROUP BY v.buildbucket_build.builder.bucket, v.buildbucket_build.builder.builder, g.testVariants[0].TestId,  g.testVariants[0].VariantHash, g.RefHash
  )
SELECT regression_group.*,
  bucket,
  builder,
  -- use empty array instead of null so we can read into []NullString.
  IFNULL(SheriffRotations, []) as SheriffRotations
FROM builder_regression_groups_with_latest_build
WHERE (NOT (SELECT LOGICAL_OR((SELECT count(*) > 0 FROM UNNEST(task_dimensions) WHERE KEY = kv.key and value = kv.value)) FROM UNNEST(@dimensionExcludes) kv)) AND (bucket NOT IN UNNEST(@excludedBuckets))
  -- We need to compare ARRAY_LENGTH with null because of unexpected Bigquery behaviour b/138262091.
  AND ((BuilderGroup IN UNNEST(@allowedBuilderGroups)) OR ARRAY_LENGTH(@allowedBuilderGroups) = 0 OR ARRAY_LENGTH(@allowedBuilderGroups) IS NULL)
  AND (BuilderGroup NOT IN UNNEST(@excludedBuilderGroups))
ORDER BY regression_group.RegressionEndPosition DESC
LIMIT 5000`)
		})

		Convey("have excluded pools", func() {
			q, err := generateTestFailuresQuery(task, dimensionExcludeFilter, []string{"chromium.tests.gpu"})
			So(err, ShouldBeNil)
			So(q, ShouldEqual, `WITH
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
      v.buildbucket_build.builder.bucket,
      v.buildbucket_build.builder.builder,
      ANY_VALUE(g) AS regression_group,
      ANY_VALUE(v.buildbucket_build.id HAVING MAX v.partition_time) AS build_id,
      ANY_VALUE(REGEXP_EXTRACT(v.results[0].parent.id, r'^task-chromium-swarm.appspot.com-([0-9a-f]+)$') HAVING MAX v.partition_time) AS swarming_run_id,
      ANY_VALUE(COALESCE(b2.infra.swarming.task_dimensions, b2.infra.backend.task_dimensions, b.infra.swarming.task_dimensions, b.infra.backend.task_dimensions) HAVING MAX v.partition_time) AS task_dimensions,
      ANY_VALUE(JSON_VALUE_ARRAY(b.input.properties, "$.sheriff_rotations") HAVING MAX v.partition_time) AS SheriffRotations,
      ANY_VALUE(JSON_VALUE(b.input.properties, "$.builder_group") HAVING MAX v.partition_time) AS BuilderGroup,
    FROM builder_regression_groups g
    -- Join with test_verdict table to get the build id of the lastest build for a test variant.
    LEFT JOIN test_verdicts v
    ON g.testVariants[0].TestId = v.test_id
      AND g.testVariants[0].VariantHash = v.variant_hash
      AND g.RefHash = v.source_ref_hash
    -- Join with buildbucket builds table to get the buildbucket related information for tests.
    LEFT JOIN (select * from cr-buildbucket.chromium.builds where create_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 3 DAY)) b
    ON v.buildbucket_build.id  = b.id
    -- JOIN with buildbucket builds table again to get task dimensions of parent builds.
    LEFT JOIN (select * from cr-buildbucket.chromium.builds where create_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 3 DAY)) b2
    ON JSON_VALUE(b.input.properties, "$.parent_build_id") = CAST(b2.id AS string)
    -- Filter by test_verdict.partition_time to only return test failures that have test verdict recently.
    -- 3 days is chosen as we expect tests run at least once every 3 days if they are not disabled.
    -- If this is found to be too restricted, we can increase it later.
    WHERE v.partition_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 3 DAY)
    GROUP BY v.buildbucket_build.builder.bucket, v.buildbucket_build.builder.builder, g.testVariants[0].TestId,  g.testVariants[0].VariantHash, g.RefHash
  )
SELECT regression_group.*,
  bucket,
  builder,
  -- use empty array instead of null so we can read into []NullString.
  IFNULL(SheriffRotations, []) as SheriffRotations
FROM builder_regression_groups_with_latest_build g
LEFT JOIN chromium-swarm.swarming.task_results_run s
ON g.swarming_run_id = s.run_id
WHERE s.end_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 3 DAY)
  AND (NOT (SELECT LOGICAL_OR((SELECT count(*) > 0 FROM UNNEST(task_dimensions) WHERE KEY = kv.key and value = kv.value)) FROM UNNEST(@dimensionExcludes) kv)) AND (bucket NOT IN UNNEST(@excludedBuckets))
  AND (s.bot.pools[0] NOT IN UNNEST(@excludedPools))
  -- We need to compare ARRAY_LENGTH with null because of unexpected Bigquery behaviour b/138262091.
  AND ((BuilderGroup IN UNNEST(@allowedBuilderGroups)) OR ARRAY_LENGTH(@allowedBuilderGroups) = 0 OR ARRAY_LENGTH(@allowedBuilderGroups) IS NULL)
  AND (BuilderGroup NOT IN UNNEST(@excludedBuilderGroups))
ORDER BY regression_group.RegressionEndPosition DESC
LIMIT 5000`)
		})
	})

}
