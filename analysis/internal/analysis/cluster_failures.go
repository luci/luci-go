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
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/analysis/internal/bqutil"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/trace"
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
}

// ReadClusterFailures reads the latest 2000 groups of failures for a single cluster for the last 7 days.
// A group of failures are failures that would be grouped together in MILO display, i.e.
// same ingested_invocation_id, test_id and variant.
func (c *Client) ReadClusterFailures(ctx context.Context, opts ReadClusterFailuresOptions) (cfs []*ClusterFailure, err error) {
	_, s := trace.StartSpan(ctx, "go.chromium.org/luci/analysis/internal/analysis/ReadClusterFailures")
	s.Attribute("project", opts.Project)
	defer func() { s.End(err) }()

	dataset, err := bqutil.DatasetForProject(opts.Project)
	if err != nil {
		return nil, errors.Annotate(err, "getting dataset").Err()
	}
	q := c.client.Query(`
		WITH latest_failures_7d AS (
			SELECT
				cluster_algorithm,
				cluster_id,
				test_result_system,
				test_result_id,
				ARRAY_AGG(cf ORDER BY cf.last_updated DESC LIMIT 1)[OFFSET(0)] as r
			FROM clustered_failures cf
			WHERE cf.partition_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
			  AND cluster_algorithm = @clusterAlgorithm
			  AND cluster_id = @clusterID
			  AND realm IN UNNEST(@realms)
			GROUP BY cluster_algorithm, cluster_id, test_result_system, test_result_id
			HAVING r.is_included
		)
		SELECT
			r.realm as Realm,
			r.test_id as TestID,
			ANY_VALUE(r.variant) as Variant,
			ANY_VALUE(r.presubmit_run_id) as PresubmitRunID,
			ANY_VALUE(r.presubmit_run_owner) as PresubmitRunOwner,
			ANY_VALUE(r.presubmit_run_mode) as PresubmitRunMode,
			ANY_VALUE(r.presubmit_run_status) as PresubmitRunStatus,
			ANY_VALUE(r.changelists) as Changelists,
			r.partition_time as PartitionTime,
			ANY_VALUE(r.exonerations) as Exonerations,
			ANY_VALUE(r.build_status) as BuildStatus,
			ANY_VALUE(r.build_critical) as IsBuildCritical,
			r.ingested_invocation_id as IngestedInvocationID,
			ANY_VALUE(r.is_ingested_invocation_blocked) as IsIngestedInvocationBlocked,
			count(*) as Count
		FROM latest_failures_7d
		GROUP BY
			r.realm,
			r.ingested_invocation_id,
			r.test_id,
			r.variant_hash,
			r.partition_time
		ORDER BY r.partition_time DESC
		LIMIT 2000
	`)
	q.DefaultDatasetID = dataset
	q.Parameters = []bigquery.QueryParameter{
		{Name: "clusterAlgorithm", Value: opts.ClusterID.Algorithm},
		{Name: "clusterID", Value: opts.ClusterID.ID},
		{Name: "realms", Value: opts.Realms},
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
