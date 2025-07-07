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
	"math"
	"sort"
	"strings"

	"cloud.google.com/go/bigquery"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/common/data/aip132"
	"go.chromium.org/luci/common/data/aip160"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/analysis/internal/bqutil"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/rulesalgorithm"
	"go.chromium.org/luci/analysis/internal/tracing"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

var ClusteredFailuresTable = aip160.NewSqlTable().WithColumns(
	aip160.NewSqlColumn().WithFieldPath("test_id").WithDatabaseName("test_id").FilterableImplicitly().Build(),
	aip160.NewSqlColumn().WithFieldPath("failure_reason").WithDatabaseName("failure_reason.primary_error_message").FilterableImplicitly().Build(),
	aip160.NewSqlColumn().WithFieldPath("realm").WithDatabaseName("realm").Filterable().Build(),
	aip160.NewSqlColumn().WithFieldPath("ingested_invocation_id").WithDatabaseName("ingested_invocation_id").Filterable().Build(),
	aip160.NewSqlColumn().WithFieldPath("cluster_algorithm").WithDatabaseName("cluster_algorithm").Filterable().WithArgumentSubstitutor(resolveAlgorithm).Build(),
	aip160.NewSqlColumn().WithFieldPath("cluster_id").WithDatabaseName("cluster_id").Filterable().Build(),
	aip160.NewSqlColumn().WithFieldPath("variant_hash").WithDatabaseName("variant_hash").Filterable().Build(),
	aip160.NewSqlColumn().WithFieldPath("test_run_id").WithDatabaseName("test_run_id").Filterable().Build(),
	aip160.NewSqlColumn().WithFieldPath("variant").WithDatabaseName("variant").KeyValue().Filterable().Build(),
	aip160.NewSqlColumn().WithFieldPath("tags").WithDatabaseName("tags").KeyValue().Filterable().Build(),
	aip160.NewSqlColumn().WithFieldPath("is_test_run_blocked").WithDatabaseName("is_test_run_blocked").Bool().Filterable().Build(),
	aip160.NewSqlColumn().WithFieldPath("is_ingested_invocation_blocked").WithDatabaseName("is_ingested_invocation_blocked").Bool().Filterable().Build(),
	aip160.NewSqlColumn().WithFieldPath("build_gardener_rotations").WithDatabaseName("build_gardener_rotations").Array().Filterable().Build(),
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
const metricByDayColumnSuffix = "by_day"

type QueryClusterSummariesOptions struct {
	// A filter on the underlying failures to include in the clusters.
	FailureFilter *aip160.Filter
	OrderBy       []aip132.OrderBy
	Realms        []string
	// Metrics is the set of metrics to query. If a metric is referenced
	// in the OrderBy clause, it must also be included here.
	Metrics   []metrics.Definition
	TimeRange *pb.TimeRange
	// Whether the daily breakdown should be included in the cluster summaries'
	// metric values.
	IncludeMetricBreakdown bool
}

type MetricValue struct {
	// The residual value of the cluster metric.
	// For bug clusters, the residual metric value is the metric value
	// calculated using all of the failures in the cluster.
	// For suggested clusters, the residual metric value is calculated
	// using the failures in the cluster which are not also part of a
	// bug cluster. In this way, measures attributed to bug clusters
	// are not counted again against suggested clusters.
	Value int64
	// The value of the cluster metric over time, grouped by 24-hour periods
	// in the queried time range, in reverse chronological order
	// i.e. the first entry is the metric value for the 24-hour period
	// immediately preceding the time range's latest time.
	DailyBreakdown []int64
}

// ClusterSummary represents a summary of the cluster's failures
// and its metrics.
type ClusterSummary struct {
	ClusterID            clustering.ClusterID
	ExampleFailureReason bigquery.NullString
	ExampleTestID        string
	UniqueTestIDs        int64
	MetricValues         map[metrics.ID]*MetricValue
}

type QueryClusterMetricBreakdownsOptions struct {
	// A filter on the underlying failures to include in the clusters.
	FailureFilter *aip160.Filter
	OrderBy       []aip132.OrderBy
	Realms        []string
	// Metrics is the set of metrics to query. If a metric is referenced
	// in the OrderBy clause, it must also be included here.
	Metrics   []metrics.Definition
	TimeRange *pb.TimeRange
}

// ClusterMetricBreakdown is the breakdown of metrics over time
// for a cluster's failures.
type ClusterMetricBreakdown struct {
	ClusterID        clustering.ClusterID
	MetricBreakdowns map[metrics.ID]*MetricBreakdown
}

// MetricBreakdown is the breakdown of values over time for a single
// metric.
type MetricBreakdown struct {
	DailyValues []int64
}

func defaultOrder(ms []metrics.Definition) []aip132.OrderBy {
	sortedMetrics := make([]metrics.Definition, len(ms))
	copy(sortedMetrics, ms)

	// Sort by SortPriority descending, then by ID ascending.
	sort.Slice(sortedMetrics, func(i, j int) bool {
		if sortedMetrics[i].Config.SortPriority > sortedMetrics[j].Config.SortPriority {
			return true
		}
		if (sortedMetrics[i].Config.SortPriority == sortedMetrics[j].Config.SortPriority) &&
			(sortedMetrics[i].ID < sortedMetrics[j].ID) {
			return true
		}
		return false
	})

	var result []aip132.OrderBy
	for _, m := range sortedMetrics {
		result = append(result, aip132.OrderBy{
			FieldPath:  aip132.NewFieldPath("metrics", string(m.ID), "value"),
			Descending: true,
		})
	}
	return result
}

// ClusterSummariesTable returns the schema of the table returned by
// the cluster summaries query. This can be used to generate and
// validate the order by clause.
func ClusterSummariesTable(queriedMetrics []metrics.Definition) *aip160.SqlTable {
	var columns []*aip160.SqlColumn
	for _, metric := range queriedMetrics {
		metricColumnName := metric.ColumnName(MetricValueColumnSuffix)
		columns = append(columns, aip160.NewSqlColumn().WithFieldPath("metrics", string(metric.ID), "value").WithDatabaseName(metricColumnName).Sortable().Build())
	}
	return aip160.NewSqlTable().WithColumns(columns...).Build()
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
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/analysis.QueryClusterSummaries",
		attribute.String("project", luciProject),
	)
	defer func() { tracing.End(s, err, attribute.Int("outcome", len(cs))) }()

	if err := pbutil.ValidateTimeRange(ctx, options.TimeRange); err != nil {
		return nil, InvalidArgumentTag.Apply(errors.Fmt("time_range: %w", err))
	}

	// Note that the content of the filter and order_by clause is untrusted
	// user input and is validated as part of the Where/OrderBy clause
	// generation here.
	const parameterPrefix = "w_"
	whereClause, parameters, err := ClusteredFailuresTable.WhereClause(options.FailureFilter, parameterPrefix)
	if err != nil {
		return nil, InvalidArgumentTag.Apply(errors.Fmt("failure_filter: %w", err))
	}

	resultTable := ClusterSummariesTable(options.Metrics)

	order := aip160.MergeWithDefaultOrder(defaultOrder(options.Metrics), options.OrderBy)
	orderByClause, err := resultTable.OrderByClause(order)
	if err != nil {
		return nil, InvalidArgumentTag.Apply(errors.Fmt("order_by: %w", err))
	}

	sql := constructQueryString(options.IncludeMetricBreakdown, options.Metrics, whereClause, orderByClause)
	q := c.client.Query(sql)
	q.DefaultDatasetID = bqutil.InternalDatasetID
	q.Parameters = toBigQueryParameters(parameters)
	q.Parameters = append(q.Parameters,
		bigquery.QueryParameter{Name: "realms", Value: options.Realms},
		bigquery.QueryParameter{Name: "project", Value: luciProject},
		bigquery.QueryParameter{Name: "earliest", Value: options.TimeRange.Earliest.AsTime()},
		bigquery.QueryParameter{Name: "latest", Value: options.TimeRange.Latest.AsTime()})

	it, err := q.Read(ctx)
	if err != nil {
		return nil, errors.Fmt("querying cluster summaries: %w", err)
	}

	// Calculate the array length for daily breakdowns for metrics.
	duration := options.TimeRange.Latest.AsTime().Sub(options.TimeRange.Earliest.AsTime())
	daysSpanningDuration := int64(math.Ceil(duration.Hours() / 24))

	clusters := []*ClusterSummary{}
	for {
		var rowVals rowLoader
		err := it.Next(&rowVals)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, errors.Fmt("obtain next cluster summary row: %w", err)
		}

		row := &ClusterSummary{}
		row.ClusterID = clustering.ClusterID{
			Algorithm: rowVals.String("cluster_algorithm"),
			ID:        rowVals.String("cluster_id"),
		}
		row.ExampleFailureReason = rowVals.NullString("example_failure_reason")
		row.ExampleTestID = rowVals.String("example_test_id")
		row.UniqueTestIDs = rowVals.Int64("unique_test_ids")
		row.MetricValues = make(map[metrics.ID]*MetricValue, len(options.Metrics))

		// The day indexes to use when constructing daily breakdowns for metrics.
		var dayIndexes []int64
		if options.IncludeMetricBreakdown {
			dayIndexes = rowVals.Int64s("day_indexes")
		}

		for _, metric := range options.Metrics {
			metricValue := &MetricValue{
				Value: rowVals.Int64(metric.ColumnName(MetricValueColumnSuffix)),
			}
			if options.IncludeMetricBreakdown {
				metricByDayResults := rowVals.Int64s(metric.ColumnName(metricByDayColumnSuffix))
				dailyBreakdown := make([]int64, daysSpanningDuration)
				for i, dayIndex := range dayIndexes {
					dailyBreakdown[dayIndex] = metricByDayResults[i]
				}
				metricValue.DailyBreakdown = dailyBreakdown
			}
			row.MetricValues[metric.ID] = metricValue
		}
		if err := rowVals.Error(); err != nil {
			return nil, errors.Fmt("marshal cluster summary row: %w", err)
		}
		clusters = append(clusters, row)
	}
	return clusters, nil
}

func toBigQueryParameters(pars []aip160.SqlQueryParameter) []bigquery.QueryParameter {
	result := make([]bigquery.QueryParameter, 0, len(pars))
	for _, p := range pars {
		result = append(result, bigquery.QueryParameter{
			Name:  p.Name,
			Value: p.Value,
		})
	}
	return result
}

func constructQueryString(includeMetricBreakdown bool, queryMetrics []metrics.Definition,
	whereClause string, orderByClause string) string {
	// The following query does not take into account removals of test failures
	// from clusters as this dramatically slows down the query. Instead, we
	// rely upon a periodic job to purge these results from the table.
	// We avoid double-counting the test failures (e.g. in case of addition
	// deletion, re-addition) by using APPROX_COUNT_DISTINCT to count the
	// number of distinct failures / other items in the cluster.
	var precomputeList []string
	var metricSelectList []string
	joinSQL := ""
	for _, metric := range queryMetrics {
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

		if metric.RequireAttrs {
			joinSQL = `
				LEFT JOIN failure_attributes attrs
					USING (project, test_result_system, ingested_invocation_id, test_result_id)
			`
		}
	}

	clusteredFailurePrecomputeSQL := `
		SELECT
			cluster_algorithm,
			cluster_id,
			f.partition_time AS partition_time,
			test_id,
			failure_reason,
			CONCAT(chunk_id, '/', COALESCE(chunk_index, 0)) AS unique_test_result_id,
			` + strings.Join(precomputeList, "\n") + `
		FROM clustered_failures f ` + joinSQL + `
		WHERE
			is_included_with_high_priority
			AND f.partition_time >= @earliest
			AND f.partition_time < @latest
			AND project = @project
			AND (` + whereClause + `)
			AND realm IN UNNEST(@realms)
	`
	clustersSQL := `
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
		LIMIT 200
	`

	if includeMetricBreakdown {
		var metricValueColumnList []string
		var metricByDayAggList []string
		var metricByDayColumnList []string
		for _, metric := range queryMetrics {
			metricColumnName := metric.ColumnName(MetricValueColumnSuffix)
			metricValueColumnList = append(metricValueColumnList, fmt.Sprintf("%s,", metricColumnName))

			metricByDayColumnName := metric.ColumnName(metricByDayColumnSuffix)
			metricByDayAggSelect := fmt.Sprintf("ARRAY_AGG(%s ORDER BY day_index) AS %s,", metricColumnName, metricByDayColumnName)
			metricByDayAggList = append(metricByDayAggList, metricByDayAggSelect)
			metricByDayColumnList = append(metricByDayColumnList, fmt.Sprintf("%s,", metricByDayColumnName))
		}

		return `
			WITH clustered_failure_precompute AS (
				` + clusteredFailurePrecomputeSQL + `
			),

			clusters AS (
				` + clustersSQL + `
			),

			daily_clusters AS (
				SELECT
					cluster_algorithm,
					cluster_id,
					DIV(EXTRACT(HOUR from @latest - partition_time), 24) AS day_index,
					` + strings.Join(metricSelectList, "\n") + `
				FROM clustered_failure_precompute
				GROUP BY
					cluster_algorithm,
					cluster_id,
					day_index
			),

			daily_clusters_agg AS (
				SELECT
					cluster_algorithm,
					cluster_id,
					ARRAY_AGG(day_index ORDER BY day_index) AS day_indexes,
					` + strings.Join(metricByDayAggList, "\n") + `
				FROM daily_clusters
				GROUP BY
					cluster_algorithm,
					cluster_id
			)

			SELECT
				c.cluster_algorithm,
				c.cluster_id,
				example_failure_reason,
				example_test_id,
				unique_test_ids,
				` + strings.Join(metricValueColumnList, "\n") + `
				day_indexes,
				` + strings.Join(metricByDayColumnList, "\n") + `
			FROM
				clusters c
			JOIN
				daily_clusters_agg d
			ON
				c.cluster_algorithm = d.cluster_algorithm
				AND c.cluster_id = d.cluster_id
			` + orderByClause
	}

	return `
		WITH clustered_failure_precompute AS (
		` + clusteredFailurePrecomputeSQL + `
		)
		` + clustersSQL
}
