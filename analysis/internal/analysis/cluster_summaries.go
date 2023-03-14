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
	"sort"
	"strings"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/analysis/internal/aip"
	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/rulesalgorithm"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/trace"
)

var ClusteredFailuresTable = aip.NewTable().WithColumns(
	aip.NewColumn().WithFieldPath("test_id").WithDatabaseName("test_id").FilterableImplicitly().Build(),
	aip.NewColumn().WithFieldPath("failure_reason").WithDatabaseName("failure_reason.primary_error_message").FilterableImplicitly().Build(),
	aip.NewColumn().WithFieldPath("realm").WithDatabaseName("realm").Filterable().Build(),
	aip.NewColumn().WithFieldPath("ingested_invocation_id").WithDatabaseName("ingested_invocation_id").Filterable().Build(),
	aip.NewColumn().WithFieldPath("cluster_algorithm").WithDatabaseName("cluster_algorithm").Filterable().WithArgumentSubstitutor(resolveAlgorithm).Build(),
	aip.NewColumn().WithFieldPath("cluster_id").WithDatabaseName("cluster_id").Filterable().Build(),
	aip.NewColumn().WithFieldPath("variant_hash").WithDatabaseName("variant_hash").Filterable().Build(),
	aip.NewColumn().WithFieldPath("test_run_id").WithDatabaseName("test_run_id").Filterable().Build(),
	aip.NewColumn().WithFieldPath("variant").WithDatabaseName("variant").KeyValue().Filterable().Build(),
	aip.NewColumn().WithFieldPath("tags").WithDatabaseName("tags").KeyValue().Filterable().Build(),
	aip.NewColumn().WithFieldPath("is_test_run_blocked").WithDatabaseName("is_test_run_blocked").Bool().Filterable().Build(),
	aip.NewColumn().WithFieldPath("is_ingested_invocation_blocked").WithDatabaseName("is_ingested_invocation_blocked").Bool().Filterable().Build(),
).Build()

func resolveAlgorithm(algorithm string) string {
	// Resolve an alias to the rules algorithm to the concrete
	// implementation, e.g. "rules" -> "rules-v2".
	if algorithm == "rules" {
		return rulesalgorithm.AlgorithmName
	}
	return algorithm
}

const MetricValueColumnSuffix = "value"

type QueryClusterSummariesOptions struct {
	// A filter on the underlying failures to include in the clusters.
	FailureFilter *aip.Filter
	OrderBy       []aip.OrderBy
	Realms        []string
	// Metrics is the set of metrics to query. If a metric is referenced
	// in the OrderBy clause, it must also be included here.
	Metrics   []metrics.Definition
	TimeRange *pb.TimeRange
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

func defaultOrder(ms []metrics.Definition) []aip.OrderBy {
	sortedMetrics := make([]metrics.Definition, len(ms))
	copy(sortedMetrics, ms)

	// Sort by SortPriority descending.
	sort.Slice(sortedMetrics, func(i, j int) bool {
		return sortedMetrics[i].SortPriority > sortedMetrics[j].SortPriority
	})

	var result []aip.OrderBy
	for _, m := range sortedMetrics {
		result = append(result, aip.OrderBy{
			FieldPath:  aip.NewFieldPath("metrics", string(m.ID), "value"),
			Descending: true,
		})
	}
	return result
}

// ClusterSummariesTable returns the schema of the table returned by
// the cluster summaries query. This can be used to generate and
// validate the order by clause.
func ClusterSummariesTable(queriedMetrics []metrics.Definition) *aip.Table {
	var columns []*aip.Column
	for _, metric := range queriedMetrics {
		metricColumnName := metric.ColumnName(MetricValueColumnSuffix)
		columns = append(columns, aip.NewColumn().WithFieldPath("metrics", string(metric.ID), "value").WithDatabaseName(metricColumnName).Sortable().Build())
	}
	return aip.NewTable().WithColumns(columns...).Build()
}

// QueryClusterSummaries queries a summary of clusters in the project.
// The subset of failures included in the clustering may be filtered.
// If the dataset for the LUCI project does not exist, returns
// ProjectNotExistsErr.
// If options.TimeRange is invalid, returns an error tagged with
// InvalidArgumentTag so that the appropriate gRPC error can be returned to the
// client (if applicable).
// If options.FailuresFilter or options.OrderBy is invalid with respect to the
// query schema, returns an error tagged with InvalidArgumentTag so that the
// appropriate gRPC error can be returned to the client (if applicable).
func (c *Client) QueryClusterSummaries(ctx context.Context, luciProject string, options *QueryClusterSummariesOptions) (cs []*ClusterSummary, err error) {
	_, s := trace.StartSpan(ctx, "go.chromium.org/luci/analysis/internal/analysis/QueryClusterSummaries")
	s.Attribute("project", luciProject)
	defer func() { s.End(err) }()

	if err := pbutil.ValidateTimeRange(ctx, options.TimeRange); err != nil {
		return nil, errors.Annotate(err, "time_range").Tag(InvalidArgumentTag).Err()
	}

	// Note that the content of the filter and order_by clause is untrusted
	// user input and is validated as part of the Where/OrderBy clause
	// generation here.
	const parameterPrefix = "w_"
	whereClause, parameters, err := ClusteredFailuresTable.WhereClause(options.FailureFilter, parameterPrefix)
	if err != nil {
		return nil, errors.Annotate(err, "failure_filter").Tag(InvalidArgumentTag).Err()
	}

	resultTable := ClusterSummariesTable(options.Metrics)

	order := aip.MergeWithDefaultOrder(defaultOrder(options.Metrics), options.OrderBy)
	orderByClause, err := resultTable.OrderByClause(order)
	if err != nil {
		return nil, errors.Annotate(err, "order_by").Tag(InvalidArgumentTag).Err()
	}

	// The following query does not take into account removals of test failures
	// from clusters as this dramatically slows down the query. Instead, we
	// rely upon a periodic job to purge these results from the table.
	// We avoid double-counting the test failures (e.g. in case of addition
	// deletion, re-addition) by using APPROX_COUNT_DISTINCT to count the
	// number of distinct failures / other items in the cluster.
	var precomputeList []string
	var metricSelectList []string
	for _, metric := range options.Metrics {
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
		metricColumnName := metric.ColumnName(MetricValueColumnSuffix)
		metricSelect := fmt.Sprintf("APPROX_COUNT_DISTINCT(IF(%s,%s,NULL)) AS %s,", filterIdentifier, itemIdentifier, metricColumnName)
		metricSelectList = append(metricSelectList, metricSelect)
	}

	sql := `
		WITH clustered_failure_precompute AS (
			SELECT
				project,
				cluster_algorithm,
				cluster_id,
				test_id,
				failure_reason,
				CONCAT(chunk_id, '/', COALESCE(chunk_index, 0)) as unique_test_result_id,
				` + strings.Join(precomputeList, "\n") + `
			FROM clustered_failures f
			WHERE
				is_included_with_high_priority
				AND partition_time >= @earliest
				AND partition_time < @latest
				AND project = @project
				AND (` + whereClause + `)
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
	q.DefaultDatasetID = "internal"
	q.Parameters = toBigQueryParameters(parameters)
	q.Parameters = append(q.Parameters,
		bigquery.QueryParameter{Name: "realms", Value: options.Realms},
		bigquery.QueryParameter{Name: "project", Value: luciProject},
		bigquery.QueryParameter{Name: "earliest", Value: options.TimeRange.Earliest.AsTime()},
		bigquery.QueryParameter{Name: "latest", Value: options.TimeRange.Latest.AsTime()})

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
		row.MetricValues = make(map[metrics.ID]int64, len(options.Metrics))
		for _, metric := range options.Metrics {
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
