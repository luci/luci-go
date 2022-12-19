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
	"fmt"
	"strings"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/analysis/internal/aip"
	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/analysis/internal/bqutil"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/trace"
)

var ClusteredFailuresTable = aip.NewTable().WithColumns(
	aip.NewColumn().WithName("test_id").WithDatabaseName("test_id").FilterableImplicitly().Build(),
	aip.NewColumn().WithName("failure_reason").WithDatabaseName("failure_reason.primary_error_message").FilterableImplicitly().Build(),
	aip.NewColumn().WithName("realm").WithDatabaseName("realm").Filterable().Build(),
	aip.NewColumn().WithName("ingested_invocation_id").WithDatabaseName("ingested_invocation_id").Filterable().Build(),
	aip.NewColumn().WithName("cluster_algorithm").WithDatabaseName("cluster_algorithm").Filterable().Build(),
	aip.NewColumn().WithName("cluster_id").WithDatabaseName("cluster_id").Filterable().Build(),
	aip.NewColumn().WithName("variant_hash").WithDatabaseName("variant_hash").Filterable().Build(),
	aip.NewColumn().WithName("test_run_id").WithDatabaseName("test_run_id").Filterable().Build(),
	aip.NewColumn().WithName("variant").WithDatabaseName("variant").KeyValue().Filterable().Build(),
	aip.NewColumn().WithName("tags").WithDatabaseName("tags").KeyValue().Filterable().Build(),
	aip.NewColumn().WithName("is_test_run_blocked").WithDatabaseName("is_test_run_blocked").Bool().Filterable().Build(),
	aip.NewColumn().WithName("is_ingested_invocation_blocked").WithDatabaseName("is_ingested_invocation_blocked").Bool().Filterable().Build(),
).Build()

const MetricValueColumnSuffix = "value"

var ClusterSummariesTable = aip.NewTable().WithColumns(
	aip.NewColumn().WithName("presubmit_rejects").WithDatabaseName(metrics.HumanClsFailedPresubmit.ColumnName(MetricValueColumnSuffix)).Sortable().Build(),
	aip.NewColumn().WithName("critical_failures_exonerated").WithDatabaseName(metrics.CriticalFailuresExonerated.ColumnName(MetricValueColumnSuffix)).Sortable().Build(),
	aip.NewColumn().WithName("failures").WithDatabaseName(metrics.Failures.ColumnName(MetricValueColumnSuffix)).Sortable().Build(),
).Build()

var ClusterSummariesDefaultOrder = []aip.OrderBy{
	{Name: "presubmit_rejects", Descending: true},
	{Name: "critical_failures_exonerated", Descending: true},
	{Name: "failures", Descending: true},
}

type QueryClusterSummariesOptions struct {
	// A filter on the underlying failures to include in the clusters.
	FailureFilter *aip.Filter
	OrderBy       []aip.OrderBy
	Realms        []string
}

// ClusterSummary represents a summary of the cluster's failures
// and their impact.
type ClusterSummary struct {
	ClusterID            clustering.ClusterID
	ExampleFailureReason bigquery.NullString
	ExampleTestID        string
	UniqueTestIDs        int64
	MetricValues         map[metrics.ID]int64
}

// Queries a summary of clusters in the project.
// The subset of failures included in the clustering may be filtered.
// If the dataset for the LUCI project does not exist, returns
// ProjectNotExistsErr.
// If options.FailuresFilter or options.OrderBy is invalid with respect to the
// query schema, returns an error tagged with InvalidArgumentTag so that the
// appropriate gRPC error can be returned to the client (if applicable).
func (c *Client) QueryClusterSummaries(ctx context.Context, luciProject string, options *QueryClusterSummariesOptions) (cs []*ClusterSummary, err error) {
	_, s := trace.StartSpan(ctx, "go.chromium.org/luci/analysis/internal/analysis/QueryClusterSummaries")
	s.Attribute("project", luciProject)
	defer func() { s.End(err) }()

	// Note that the content of the filter and order_by clause is untrusted
	// user input and is validated as part of the Where/OrderBy clause
	// generation here.
	const parameterPrefix = "w_"
	whereClause, parameters, err := ClusteredFailuresTable.WhereClause(options.FailureFilter, parameterPrefix)
	if err != nil {
		return nil, errors.Annotate(err, "failure_filter").Tag(InvalidArgumentTag).Err()
	}

	order := aip.MergeWithDefaultOrder(ClusterSummariesDefaultOrder, options.OrderBy)
	orderByClause, err := ClusterSummariesTable.OrderByClause(order)
	if err != nil {
		return nil, errors.Annotate(err, "order_by").Tag(InvalidArgumentTag).Err()
	}

	dataset, err := bqutil.DatasetForProject(luciProject)
	if err != nil {
		return nil, errors.Annotate(err, "getting dataset").Err()
	}

	// The following query does not take into account removals of test failures
	// from clusters as this dramatically slows down the query. Instead, we
	// rely upon a periodic job to purge these results from the table.
	// We avoid double-counting the test failures (e.g. in case of addition
	// deletion, re-addition) by using APPROX_COUNT_DISTINCT to count the
	// number of distinct failures / other items in the cluster.
	var precomputeList []string
	var metricSelectList []string
	for _, metric := range metrics.DefaultMetrics {
		itemIdentifier := "unique_test_result_id"
		if metric.CountSQL != "" {
			itemIdentifier = metric.ColumnName("item")
			precomputeList = append(precomputeList, fmt.Sprintf("%s AS %s,", metric.CountSQL, itemIdentifier))
		}
		filterIdentifier := "TRUE"
		if metric.FilterSQL != "" {
			filterIdentifier = metric.ColumnName("filter")
			precomputeList = append(precomputeList, fmt.Sprintf("%s AS %s,", metric.FilterSQL, filterIdentifier))
		}
		metricIdentifier := metric.ColumnName(MetricValueColumnSuffix)
		metricSelect := fmt.Sprintf("APPROX_COUNT_DISTINCT(IF(%s,%s,NULL)) AS %s,", filterIdentifier, itemIdentifier, metricIdentifier)
		metricSelectList = append(metricSelectList, metricSelect)
	}

	sql := `
		WITH clustered_failure_precompute AS (
			SELECT
				cluster_algorithm,
				cluster_id,
				test_id,
				failure_reason,
				CONCAT(chunk_id, '/', COALESCE(chunk_index, 0)) as unique_test_result_id,
				` + strings.Join(precomputeList, "\n") + `
			FROM clustered_failures f
			WHERE
				is_included_with_high_priority
				AND partition_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
				AND ` + whereClause + `
				AND realm IN UNNEST(@realms)
		)
		SELECT
			cluster_algorithm,
			cluster_id,
			ANY_VALUE(failure_reason.primary_error_message) AS example_failure_reason,
			MIN(test_id) AS example_test_id,
			APPROX_COUNT_DISTINCT(test_id) AS unique_test_ids,
		    ` + strings.Join(metricSelectList, "\n") + `
		FROM clustered_failure_precompute
		GROUP BY
			cluster_algorithm,
			cluster_id
		` + orderByClause + `
		LIMIT 1000
	`

	q := c.client.Query(sql)
	q.DefaultDatasetID = dataset
	q.Parameters = toBigQueryParameters(parameters)
	q.Parameters = append(q.Parameters, bigquery.QueryParameter{
		Name:  "realms",
		Value: options.Realms,
	})

	job, err := q.Run(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "querying cluster summaries").Err()
	}
	it, err := job.Read(ctx)
	if err != nil {
		return nil, handleJobReadError(err)
	}
	clusters := []*ClusterSummary{}
	for {
		var rowVals rowLoader
		err := it.Next(&rowVals)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, errors.Annotate(err, "obtain next cluster summary row").Err()
		}

		row := &ClusterSummary{}
		row.ClusterID = clustering.ClusterID{
			Algorithm: rowVals.String("cluster_algorithm"),
			ID:        rowVals.String("cluster_id"),
		}
		row.ExampleFailureReason = rowVals.NullString("example_failure_reason")
		row.ExampleTestID = rowVals.String("example_test_id")
		row.UniqueTestIDs = rowVals.Int64("unique_test_ids")
		row.MetricValues = make(map[metrics.ID]int64, len(metrics.DefaultMetrics))
		for _, metric := range metrics.DefaultMetrics {
			row.MetricValues[metric.ID] = rowVals.Int64(metric.ColumnName(MetricValueColumnSuffix))
		}
		if err := rowVals.Error(); err != nil {
			return nil, errors.Annotate(err, "marshal cluster summary row").Err()
		}
		clusters = append(clusters, row)
	}
	return clusters, nil
}

func toBigQueryParameters(pars []aip.QueryParameter) []bigquery.QueryParameter {
	result := make([]bigquery.QueryParameter, 0, len(pars))
	for _, p := range pars {
		result = append(result, bigquery.QueryParameter{
			Name:  p.Name,
			Value: p.Value,
		})
	}
	return result
}
