// Copyright 2022 The LUCI Authors.
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

package analysis

import (
	"context"
	"math"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/trace"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/analysis/internal/bqutil"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/rulesalgorithm"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

// Cluster contains detailed information about a cluster, including
// a statistical summary of a cluster's failures, and their impact.
type Cluster struct {
	ClusterID clustering.ClusterID `json:"clusterId"`
	// Distinct user CLs with presubmit rejects.
	PresubmitRejects1d Counts `json:"presubmitRejects1d"`
	PresubmitRejects3d Counts `json:"presubmitRejects3d"`
	PresubmitRejects7d Counts `json:"presubmitRejects7d"`
	// Distinct test runs failed.
	TestRunFails1d Counts `json:"testRunFailures1d"`
	TestRunFails3d Counts `json:"testRunFailures3d"`
	TestRunFails7d Counts `json:"testRunFailures7d"`
	// Total test results with unexpected failures.
	Failures1d Counts `json:"failures1d"`
	Failures3d Counts `json:"failures3d"`
	Failures7d Counts `json:"failures7d"`
	// Test failures exonerated on critical builders, and for an
	// exoneration reason other than NOT_CRITICAL.
	CriticalFailuresExonerated1d Counts `json:"criticalFailuresExonerated1d"`
	CriticalFailuresExonerated3d Counts `json:"criticalFailuresExonerated3d"`
	CriticalFailuresExonerated7d Counts `json:"criticalFailuresExonerated7d"`

	// The realm(s) examples of the cluster are present in.
	Realms               []string
	ExampleFailureReason bigquery.NullString `json:"exampleFailureReason"`
	// Top Test IDs included in the cluster, up to 5. Unless the cluster
	// is empty, will always include at least one Test ID.
	TopTestIDs []TopCount `json:"topTestIds"`
	// Top Monorail Components indicates the top monorail components failures
	// in the cluster are associated with by number of failures, up to 5.
	TopMonorailComponents []TopCount `json:"topMonorailComponents"`
}

// ExampleTestID returns an example Test ID that is part of the cluster, or
// "" if the cluster is empty.
func (s *Cluster) ExampleTestID() string {
	if len(s.TopTestIDs) > 0 {
		return s.TopTestIDs[0].Value
	}
	return ""
}

// Counts captures the values of an integer-valued metric in different
// calculation bases.
type Counts struct {
	// The statistic value after impact has been reduced by exoneration.
	Nominal int64 `json:"nominal"`
	// The statistic value:
	// - excluding impact already counted under other higher-priority clusters
	//   (I.E. bug clusters.)
	// - after impact has been reduced by exoneration.
	Residual int64 `json:"residual"`
}

// TopCount captures the result of the APPROX_TOP_COUNT operator. See:
// https://cloud.google.com/bigquery/docs/reference/standard-sql/approximate_aggregate_functions#approx_top_count
type TopCount struct {
	// Value is the value that was frequently occurring.
	Value string `json:"value"`
	// Count is the frequency with which the value occurred.
	Count int64 `json:"count"`
}

// RebuildAnalysis re-builds the cluster summaries analysis from
// clustered test results.
func (c *Client) RebuildAnalysis(ctx context.Context, luciProject string) error {
	datasetID, err := bqutil.DatasetForProject(luciProject)
	if err != nil {
		return errors.Annotate(err, "getting dataset").Err()
	}
	dataset := c.client.Dataset(datasetID)

	dstTable := dataset.Table("cluster_summaries")

	q := c.client.Query(clusterAnalysis)
	q.DefaultDatasetID = dataset.DatasetID
	q.Dst = dstTable
	q.CreateDisposition = bigquery.CreateIfNeeded
	q.WriteDisposition = bigquery.WriteTruncate
	job, err := q.Run(ctx)
	if err != nil {
		return errors.Annotate(err, "starting cluster summary analysis").Err()
	}

	waitCtx, cancel := context.WithTimeout(ctx, time.Minute*5)
	defer cancel()

	js, err := job.Wait(waitCtx)
	if err != nil {
		return errors.Annotate(err, "waiting for cluster summary analysis to complete").Err()
	}
	if js.Err() != nil {
		return errors.Annotate(err, "cluster summary analysis failed").Err()
	}
	return nil
}

// PurgeStaleRows purges stale clustered failure rows from the table.
// Stale rows are those rows which have been superseded by a new row with a later
// version, or where the latest version of the row has the row not included in a
// cluster.
// This is necessary for:
// - Our QueryClusterSummaries query, which for performance reasons (UI-interactive)
//   does not do filtering to fetch the latest version of rows and instead uses all
//   rows.
// - Keeping the size of the BigQuery table to a minimum.
// We currently only purge the last 7 days to keep purging costs to a minimum and
// as this is as far as QueryClusterSummaries looks back.
func (c *Client) PurgeStaleRows(ctx context.Context, luciProject string) error {
	datasetID, err := bqutil.DatasetForProject(luciProject)
	if err != nil {
		return errors.Annotate(err, "getting dataset").Err()
	}
	dataset := c.client.Dataset(datasetID)

	// If something goes wrong with this statement it deletes everything
	// for some reason, the system can be restored as follows:
	// - Fix the statement.
	// - Bump the algorithm version on all algorithms, to trigger a
	//   re-clustering and re-export of all test results.
	q := c.client.Query(`
		DELETE FROM clustered_failures cf1
		WHERE
			cf1.partition_time > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY) AND
			-- Not in the streaming buffer. Streaming buffer keeps up to
			-- 30 minutes of data. We use 40 minutes here to allow some
			-- margin as our last_updated timestamp is the timestamp
			-- the chunk was committed in Spanner and export to BigQuery
			-- can be delayed from that.
			cf1.last_updated < TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 40 MINUTE) AND
			(
				-- Not the latest (cluster, test result) entry.
				cf1.last_updated < (SELECT MAX(cf2.last_updated)
								FROM clustered_failures cf2
								WHERE cf2.partition_time > TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
									AND cf2.partition_time = cf1.partition_time
									AND cf2.cluster_algorithm = cf1.cluster_algorithm
									AND cf2.cluster_id = cf1.cluster_id
									AND cf2.chunk_id = cf1.chunk_id
									AND cf2.chunk_index = cf1.chunk_index
									)
				-- Or is the latest (cluster, test result) entry, but test result
				-- is no longer in cluster.
				OR NOT cf1.is_included
			)
	`)
	q.DefaultDatasetID = dataset.DatasetID

	job, err := q.Run(ctx)
	if err != nil {
		return errors.Annotate(err, "purge stale rows").Err()
	}

	waitCtx, cancel := context.WithTimeout(ctx, time.Minute*5)
	defer cancel()

	js, err := job.Wait(waitCtx)
	if err != nil {
		// BigQuery specifies that rows are kept in the streaming buffer for
		// 30 minutes, but sometimes exceeds this SLO. We could be less
		// aggressive at deleting rows, but that would make the average-case
		// experience worse. These errors should only occur occasionally,
		// so it is better to ignore them.
		if strings.Contains(err.Error(), "would affect rows in the streaming buffer, which is not supported") {
			logging.Warningf(ctx, "Row purge failed for %v because rows were in the streaming buffer for over 30 minutes. "+
				"If this message occurs more than 25 percent of the time, it should be investigated.", luciProject)
			return nil
		}
		return errors.Annotate(err, "waiting for stale row purge to complete").Err()
	}
	if js.Err() != nil {
		return errors.Annotate(err, "purge stale rows failed").Err()
	}
	return nil
}

// ReadCluster reads information about a list of clusters.
// If the dataset for the LUCI project does not exist, returns ProjectNotExistsErr.
func (c *Client) ReadClusters(ctx context.Context, luciProject string, clusterIDs []clustering.ClusterID) (cs []*Cluster, err error) {
	_, s := trace.StartSpan(ctx, "go.chromium.org/luci/analysis/internal/analysis/ReadClusters")
	s.Attribute("project", luciProject)
	defer func() { s.End(err) }()

	dataset, err := bqutil.DatasetForProject(luciProject)
	if err != nil {
		return nil, errors.Annotate(err, "getting dataset").Err()
	}

	q := c.client.Query(`
		SELECT
			STRUCT(cluster_algorithm AS Algorithm, cluster_id as ID) as ClusterID,` +
		selectCounts("critical_failures_exonerated", "CriticalFailuresExonerated", "1d") +
		selectCounts("critical_failures_exonerated", "CriticalFailuresExonerated", "3d") +
		selectCounts("critical_failures_exonerated", "CriticalFailuresExonerated", "7d") +
		selectCounts("presubmit_rejects", "PresubmitRejects", "1d") +
		selectCounts("presubmit_rejects", "PresubmitRejects", "3d") +
		selectCounts("presubmit_rejects", "PresubmitRejects", "7d") +
		selectCounts("test_run_fails", "TestRunFails", "1d") +
		selectCounts("test_run_fails", "TestRunFails", "3d") +
		selectCounts("test_run_fails", "TestRunFails", "7d") +
		selectCounts("failures", "Failures", "1d") +
		selectCounts("failures", "Failures", "3d") +
		selectCounts("failures", "Failures", "7d") + `
		    realms as Realms,
			example_failure_reason.primary_error_message as ExampleFailureReason,
			top_test_ids as TopTestIDs
		FROM cluster_summaries
		WHERE STRUCT(cluster_algorithm AS Algorithm, cluster_id as ID) IN UNNEST(@clusterIDs)
	`)
	q.DefaultDatasetID = dataset
	q.Parameters = []bigquery.QueryParameter{
		{Name: "clusterIDs", Value: clusterIDs},
	}
	job, err := q.Run(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "querying cluster").Err()
	}
	it, err := job.Read(ctx)
	if err != nil {
		return nil, handleJobReadError(err)
	}
	clusters := []*Cluster{}
	for {
		row := &Cluster{}
		err := it.Next(row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, errors.Annotate(err, "obtain next cluster row").Err()
		}
		clusters = append(clusters, row)
	}
	return clusters, nil
}

// ImpactfulClusterReadOptions specifies options for ReadImpactfulClusters().
type ImpactfulClusterReadOptions struct {
	// Project is the LUCI Project for which analysis is being performed.
	Project string
	// Thresholds is the set of thresholds, which if any are met
	// or exceeded, should result in the cluster being returned.
	// Thresholds are applied based on the residual actual
	// cluster impact.
	Thresholds *configpb.ImpactThreshold
	// AlwaysIncludeBugClusters controls whether to include analysis for all
	// bug clusters.
	AlwaysIncludeBugClusters bool
}

// ReadImpactfulClusters reads clusters exceeding specified impact metrics, or are otherwise
// nominated to be read.
func (c *Client) ReadImpactfulClusters(ctx context.Context, opts ImpactfulClusterReadOptions) (cs []*Cluster, err error) {
	_, s := trace.StartSpan(ctx, "go.chromium.org/luci/analysis/internal/analysis/ReadImpactfulClusters")
	s.Attribute("project", opts.Project)
	defer func() { s.End(err) }()

	if opts.Thresholds == nil {
		return nil, errors.New("thresholds must be specified")
	}

	dataset, err := bqutil.DatasetForProject(opts.Project)
	if err != nil {
		return nil, errors.Annotate(err, "getting dataset").Err()
	}

	whereCriticalFailuresExonerated, cfeParams := whereThresholdsMet("critical_failures_exonerated", opts.Thresholds.CriticalFailuresExonerated)
	whereFailures, failuresParams := whereThresholdsMet("failures", opts.Thresholds.TestResultsFailed)
	whereTestRuns, testRunsParams := whereThresholdsMet("test_run_fails", opts.Thresholds.TestRunsFailed)
	wherePresubmits, presubmitParams := whereThresholdsMet("presubmit_rejects", opts.Thresholds.PresubmitRunsFailed)

	q := c.client.Query(`
		SELECT
			STRUCT(cluster_algorithm AS Algorithm, cluster_id as ID) as ClusterID,` +
		selectCounts("critical_failures_exonerated", "CriticalFailuresExonerated", "1d") +
		selectCounts("critical_failures_exonerated", "CriticalFailuresExonerated", "3d") +
		selectCounts("critical_failures_exonerated", "CriticalFailuresExonerated", "7d") +
		selectCounts("presubmit_rejects", "PresubmitRejects", "1d") +
		selectCounts("presubmit_rejects", "PresubmitRejects", "3d") +
		selectCounts("presubmit_rejects", "PresubmitRejects", "7d") +
		selectCounts("test_run_fails", "TestRunFails", "1d") +
		selectCounts("test_run_fails", "TestRunFails", "3d") +
		selectCounts("test_run_fails", "TestRunFails", "7d") +
		selectCounts("failures", "Failures", "1d") +
		selectCounts("failures", "Failures", "3d") +
		selectCounts("failures", "Failures", "7d") + `
			example_failure_reason.primary_error_message as ExampleFailureReason,
			top_test_ids as TopTestIDs,
			ARRAY(
				SELECT AS STRUCT value, count
				FROM UNNEST(top_monorail_components)
				WHERE value IS NOT NULL
			) as TopMonorailComponents
		FROM cluster_summaries
		WHERE (` + whereCriticalFailuresExonerated + `) OR (` + whereFailures + `)
		    OR (` + whereTestRuns + `) OR (` + wherePresubmits + `)
		    OR (@alwaysIncludeBugClusters AND cluster_algorithm = @ruleAlgorithmName)
		ORDER BY
			presubmit_rejects_residual_1d DESC,
			critical_failures_exonerated_residual_1d DESC,
			test_run_fails_residual_1d DESC,
			failures_residual_1d DESC
	`)
	q.DefaultDatasetID = dataset

	params := []bigquery.QueryParameter{
		{
			Name:  "ruleAlgorithmName",
			Value: rulesalgorithm.AlgorithmName,
		},
		{
			Name:  "alwaysIncludeBugClusters",
			Value: opts.AlwaysIncludeBugClusters,
		},
	}
	params = append(params, cfeParams...)
	params = append(params, failuresParams...)
	params = append(params, testRunsParams...)
	params = append(params, presubmitParams...)
	q.Parameters = params

	job, err := q.Run(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "querying clusters").Err()
	}
	it, err := job.Read(ctx)
	if err != nil {
		return nil, handleJobReadError(err)
	}
	clusters := []*Cluster{}
	for {
		row := &Cluster{}
		err := it.Next(row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, errors.Annotate(err, "obtain next cluster row").Err()
		}
		clusters = append(clusters, row)
	}
	return clusters, nil
}

func valueOrDefault(value *int64, defaultValue int64) int64 {
	if value != nil {
		return *value
	}
	return defaultValue
}

// selectCounts generates SQL to select a set of Counts.
func selectCounts(sqlPrefix, fieldPrefix, suffix string) string {
	return `STRUCT(` +
		sqlPrefix + `_` + suffix + ` AS Nominal,` +
		sqlPrefix + `_residual_` + suffix + ` AS Residual` +
		`) AS ` + fieldPrefix + suffix + `,`
}

// whereThresholdsMet generates a SQL Where clause to query
// where a particular metric meets a given threshold.
func whereThresholdsMet(sqlPrefix string, threshold *configpb.MetricThreshold) (string, []bigquery.QueryParameter) {
	if threshold == nil {
		threshold = &configpb.MetricThreshold{}
	}
	sql := sqlPrefix + "_residual_1d >= @" + sqlPrefix + "_1d OR " +
		sqlPrefix + "_residual_3d >= @" + sqlPrefix + "_3d OR " +
		sqlPrefix + "_residual_7d >= @" + sqlPrefix + "_7d"
	parameters := []bigquery.QueryParameter{
		{
			Name:  sqlPrefix + "_1d",
			Value: valueOrDefault(threshold.OneDay, math.MaxInt64),
		},
		{
			Name:  sqlPrefix + "_3d",
			Value: valueOrDefault(threshold.ThreeDay, math.MaxInt64),
		},
		{
			Name:  sqlPrefix + "_7d",
			Value: valueOrDefault(threshold.SevenDay, math.MaxInt64),
		},
	}
	return sql, parameters
}
