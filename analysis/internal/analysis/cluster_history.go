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
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/analysis/internal/aip"
	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/trace"
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
	_, s := trace.StartSpan(ctx, "go.chromium.org/luci/analysis/internal/analysis/ReadClusterHistory")
	s.Attribute("project", options.Project)
	defer func() { s.End(err) }()

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

	// Works out to ~ 0.4 cents for 7 days on 1 cluster as measured 2022-11-29
	// TODO: should we make this use only the latest versions in the table for greater accuracy?
	sql := `
	WITH items AS (
		SELECT
			project,
			cluster_algorithm,
			cluster_id,
			partition_time,
			test_run_id,
			CONCAT(chunk_id, '/', COALESCE(chunk_index, 0)) as unique_test_result_id,
			` + strings.Join(precomputeList, "\n") + `
	  FROM
		clustered_failures f
	  WHERE
	    is_included_with_high_priority
		AND partition_time > TIMESTAMP_TRUNC(TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL @days DAY), DAY)
		AND project = @project
		AND ` + whereClause + `
		AND realm IN UNNEST(@realms)
	  )
	  SELECT
		TIMESTAMP(day) as Date,
		` + strings.Join(metricSelectList, "\n") + `
	  FROM UNNEST(GENERATE_DATE_ARRAY(DATE_SUB(CURRENT_DATE(), INTERVAL @days - 1 DAY),CURRENT_DATE())) AS day LEFT OUTER JOIN items i ON date(i.partition_time) = day
	  GROUP BY 1
	  ORDER BY 1 ASC
	`
	q := c.client.Query(sql)
	q.DefaultDatasetID = "internal"
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
		row := &ReadClusterHistoryDay{Date: rowVals["Date"].(time.Time)}
		row.MetricValues = map[metrics.ID]int32{}
		for _, metric := range options.Metrics {
			row.MetricValues[metric.ID] = int32(rowVals[metric.ColumnName(MetricValueColumnSuffix)].(int64))
		}
		days = append(days, row)
	}
	return days, nil
}
