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

	"cloud.google.com/go/bigquery"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/analysis/internal/bqutil"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/tracing"
	"go.chromium.org/luci/common/errors"
)

type ClusterFailure struct {
	Realm              bigquery.NullString
	TestID             bigquery.NullString
	Variant            []*Variant
	PresubmitRunID     *PresubmitRunID
	PresubmitRunOwner  bigquery.NullString
	PresubmitRunMode   bigquery.NullString
	PresubmitRunStatus bigquery.NullString
	Changelists        []*Changelist
	PartitionTime      bigquery.NullTimestamp
	Exonerations       []*Exoneration
	// luci.analysis.v1.BuildStatus, without "BUILD_STATUS_" prefix.
	BuildStatus                 bigquery.NullString
	IsBuildCritical             bigquery.NullBool
	IngestedInvocationID        bigquery.NullString
	IsIngestedInvocationBlocked bigquery.NullBool
	Count                       int32
}

type Exoneration struct {
	// luci.analysis.v1.ExonerationReason value. E.g. "OCCURS_ON_OTHER_CLS".
	Reason bigquery.NullString
}

type Variant struct {
	Key   bigquery.NullString
	Value bigquery.NullString
}

type PresubmitRunID struct {
	System bigquery.NullString
	ID     bigquery.NullString
}

type Changelist struct {
	Host      bigquery.NullString
	Change    bigquery.NullInt64
	Patchset  bigquery.NullInt64
	OwnerKind bigquery.NullString
}

type ReadClusterFailuresOptions struct {
	// The LUCI Project.
	Project   string
	ClusterID clustering.ClusterID
	Realms    []string
	// The metric to show failures related to.
	// If this is empty, all failures can be returned.
	MetricFilter *metrics.Definition
}

// ReadClusterFailures reads the latest 2000 groups of failures for a single cluster for the last 7 days.
// A group of failures are failures that would be grouped together in MILO display, i.e.
// same ingested_invocation_id, test_id and variant.
func (c *Client) ReadClusterFailures(ctx context.Context, opts ReadClusterFailuresOptions) (cfs []*ClusterFailure, err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/analysis.ReadClusterFailures",
		attribute.String("project", opts.Project),
	)
	defer func() { tracing.End(s, err, attribute.Int("outcome", len(cfs))) }()

	var metricFilterSQL string
	metricBaseView := "latest_failures_7d"
	if opts.MetricFilter != nil && opts.MetricFilter.FilterSQL != "" {
		metricFilterSQL = opts.MetricFilter.FilterSQL
		if opts.MetricFilter.RequireAttrs {
			metricBaseView = "latest_failures_with_attrs_7d"
		}
	} else {
		// If the is no filter, include all failures.
		metricFilterSQL = "TRUE"
	}

	q := c.client.Query(`
		WITH latest_failures_7d AS (
			SELECT
				project,
				cluster_algorithm,
				cluster_id,
				test_result_system,
				ingested_invocation_id,
				test_result_id,
				ARRAY_AGG(cf ORDER BY cf.last_updated DESC LIMIT 1)[OFFSET(0)] as f
			FROM clustered_failures cf
			WHERE cf.partition_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
			  AND project = @project
			  AND cluster_algorithm = @clusterAlgorithm
			  AND cluster_id = @clusterID
			  AND realm IN UNNEST(@realms)
			GROUP BY project, cluster_algorithm, cluster_id, test_result_system, ingested_invocation_id, test_result_id
			HAVING f.is_included
		), latest_failures_with_attrs_7d AS (
			SELECT
				lf.project AS project,
				lf.cluster_algorithm AS cluster_algorithm,
				lf.cluster_id AS cluster_id ,
				lf.test_result_system AS test_result_system,
				lf.ingested_invocation_id AS ingested_invocation_id,
				lf.test_result_id AS test_result_id,
				lf.f AS f,
				attrs AS attrs,
			FROM latest_failures_7d AS lf
				LEFT JOIN failure_attributes AS attrs
					ON (
						lf.project = attrs.project
						AND lf.test_result_system = attrs.test_result_system
						AND lf.ingested_invocation_id = attrs.ingested_invocation_id
						AND lf.test_result_id = attrs.test_result_id
					)
		)
		SELECT
			f.realm as Realm,
			f.test_id as TestID,
			ANY_VALUE(f.variant) as Variant,
			ANY_VALUE(f.presubmit_run_id) as PresubmitRunID,
			ANY_VALUE(f.presubmit_run_owner) as PresubmitRunOwner,
			ANY_VALUE(f.presubmit_run_mode) as PresubmitRunMode,
			ANY_VALUE(f.presubmit_run_status) as PresubmitRunStatus,
			ANY_VALUE(f.changelists) as Changelists,
			f.partition_time as PartitionTime,
			ANY_VALUE(f.exonerations) as Exonerations,
			ANY_VALUE(f.build_status) as BuildStatus,
			ANY_VALUE(f.build_critical) as IsBuildCritical,
			f.ingested_invocation_id as IngestedInvocationID,
			ANY_VALUE(f.is_ingested_invocation_blocked) as IsIngestedInvocationBlocked,
			count(*) as Count
		FROM ` + metricBaseView + `
		WHERE (` + metricFilterSQL + `)
		GROUP BY
			f.realm,
			f.ingested_invocation_id,
			f.test_id,
			f.variant_hash,
			f.partition_time
		ORDER BY f.partition_time DESC
		LIMIT 2000
	`)
	q.DefaultDatasetID = bqutil.InternalDatasetID
	q.Parameters = []bigquery.QueryParameter{
		{Name: "clusterAlgorithm", Value: opts.ClusterID.Algorithm},
		{Name: "clusterID", Value: opts.ClusterID.ID},
		{Name: "realms", Value: opts.Realms},
		{Name: "project", Value: opts.Project},
	}
	job, err := q.Run(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "querying cluster failures").Err()
	}
	it, err := job.Read(ctx)
	if err != nil {
		return nil, handleJobReadError(err)
	}
	failures := []*ClusterFailure{}
	for {
		row := &ClusterFailure{}
		err := it.Next(row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, errors.Annotate(err, "obtain next cluster failure row").Err()
		}
		failures = append(failures, row)
	}
	return failures, nil
}
