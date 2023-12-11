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
	"time"

	"cloud.google.com/go/bigquery"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/analysis/internal/aip"
	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/analysis/internal/bqutil"
	"go.chromium.org/luci/analysis/internal/tracing"
	"go.chromium.org/luci/common/errors"
)

type ReadClusterHistoryOptions struct {
	Project       string
	FailureFilter *aip.Filter
	Days          int32
	Metrics       []metrics.Definition
	Realms        []string
}

type ReadClusterHistoryDay struct {
	Date         time.Time
	MetricValues map[metrics.ID]int32
	Realms       []string
}

// ReadCluster reads information about a list of clusters.
// If the dataset for the LUCI project does not exist, returns ProjectNotExistsErr.
func (c *Client) ReadClusterHistory(ctx context.Context, options ReadClusterHistoryOptions) (ret []*ReadClusterHistoryDay, err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/analysis.ReadClusterHistory",
		attribute.String("project", options.Project),
	)
	defer func() { tracing.End(s, err, attribute.Int("outcome", len(ret))) }()

	// Note that the content of the filter and order_by clause is untrusted
	// user input and is validated as part of the Where/OrderBy clause
	// generation here.
	const parameterPrefix = "w_"
	whereClause, parameters, err := ClusteredFailuresTable.WhereClause(options.FailureFilter, parameterPrefix)
	if err != nil {
		return nil, errors.Annotate(err, "failure_filter").Tag(InvalidArgumentTag).Err()
	}

	var precomputeList []string
	var metricSelectList []string
	metricsBaseView := "latest_failures"
	for _, metric := range options.Metrics {
		filterIdentifier := metric.ColumnName("filter")
		filterSQL := "TRUE"
		if metric.FilterSQL != "" {
			filterSQL = metric.FilterSQL
		}
		precomputeList = append(precomputeList, fmt.Sprintf("%s AS %s,", filterSQL, filterIdentifier))

		var itemIdentifier string
		if metric.CountSQL != "" {
			itemIdentifier = metric.ColumnName("item")
			precomputeList = append(precomputeList, fmt.Sprintf("%s AS %s,", metric.CountSQL, itemIdentifier))
		}

		metricColumnName := metric.ColumnName(MetricValueColumnSuffix)

		var metricExpr string
		if metric.CountSQL != "" {
			metricExpr = fmt.Sprintf("COUNT(DISTINCT IF(%s, %s, NULL))", filterIdentifier, itemIdentifier)
		} else {
			metricExpr = fmt.Sprintf("COUNTIF(%s)", filterIdentifier)
		}

		if metric.RequireAttrs {
			metricsBaseView = "latest_failures_with_attrs"
		}

		metricSelect := fmt.Sprintf("%s AS %s,", metricExpr, metricColumnName)
		metricSelectList = append(metricSelectList, metricSelect)
	}

	// Works out to 1.5GB procssed (~0.8 cents) for 7 days on 1 cluster as measured 2023-03-13.
	sql := `
		WITH latest_failures AS (
			SELECT
				DATE(cf.partition_time) as partition_day,
				project,
				cluster_algorithm,
				cluster_id,
				test_result_system,
				ingested_invocation_id,
				test_result_id,
				ARRAY_AGG(cf ORDER BY cf.last_updated DESC LIMIT 1)[OFFSET(0)] as f
			FROM clustered_failures cf
			WHERE cf.partition_time >= TIMESTAMP_SUB(TIMESTAMP_TRUNC(CURRENT_TIMESTAMP(), DAY), INTERVAL @days DAY)
				AND project = @project
				AND (` + whereClause + `)
				AND realm IN UNNEST(@realms)
			GROUP BY partition_day, project, cluster_algorithm, cluster_id, test_result_system, ingested_invocation_id, test_result_id
			HAVING f.is_included
		), latest_failures_with_attrs AS (
			SELECT
				lf.partition_day AS partition_day,
				lf.project AS project,
				lf.cluster_algorithm AS cluster_algorithm,
				lf.cluster_id AS cluster_id ,
				lf.test_result_system AS test_result_system,
				lf.test_result_id AS test_result_system,
				lf.f AS f,
				attrs AS attrs,
			FROM latest_failures AS lf
				LEFT JOIN failure_attributes AS attrs
					ON (
						lf.project = attrs.project
						AND lf.test_result_system = attrs.test_result_system
						AND lf.ingested_invocation_id = attrs.ingested_invocation_id
						AND lf.test_result_id = attrs.test_result_id
					)
		), failures_precompute AS (
			SELECT
				partition_day,
				cluster_algorithm,
				cluster_id,
				` + strings.Join(precomputeList, "\n") + `
			FROM ` + metricsBaseView + `
		)
		SELECT
			TIMESTAMP(day) as day,
			` + strings.Join(metricSelectList, "\n") + `
		FROM UNNEST(
			GENERATE_DATE_ARRAY(
				DATE_SUB(CURRENT_DATE(), INTERVAL @days - 1 DAY),
				CURRENT_DATE()
			)
		) day
		LEFT JOIN failures_precompute ON day = partition_day
		GROUP BY day
		ORDER BY day ASC
	`
	q := c.client.Query(sql)
	q.DefaultDatasetID = bqutil.InternalDatasetID
	q.Parameters = toBigQueryParameters(parameters)
	q.Parameters = append(q.Parameters,
		bigquery.QueryParameter{Name: "project", Value: options.Project},
		bigquery.QueryParameter{Name: "realms", Value: options.Realms},
		bigquery.QueryParameter{Name: "days", Value: options.Days})
	job, err := q.Run(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "querying cluster history").Err()
	}
	it, err := job.Read(ctx)
	if err != nil {
		return nil, handleJobReadError(err)
	}
	days := []*ReadClusterHistoryDay{}
	for {
		rowVals := map[string]bigquery.Value{}
		err := it.Next(&rowVals)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, errors.Annotate(err, "obtain next cluster history day row").Err()
		}
		row := &ReadClusterHistoryDay{Date: rowVals["day"].(time.Time)}
		row.MetricValues = map[metrics.ID]int32{}
		for _, metric := range options.Metrics {
			row.MetricValues[metric.ID] = int32(rowVals[metric.ColumnName(MetricValueColumnSuffix)].(int64))
		}
		days = append(days, row)
	}
	return days, nil
}
