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

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/bqutil"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/tracing"
)

type ExoneratedTestVariant struct {
	TestID                     bigquery.NullString
	Variant                    []*Variant
	CriticalFailuresExonerated int32
	LastExoneration            bigquery.NullTimestamp
}

type ReadClusterExoneratedTestVariantsOptions struct {
	// The LUCI Project.
	Project   string
	ClusterID clustering.ClusterID
	Realms    []string
}

// ReadClusterExoneratedTestVariants reads the latest 100 test variants
// which have presubmit-blocking failures exonerated in the last 7 days.
func (c *Client) ReadClusterExoneratedTestVariants(ctx context.Context, opts ReadClusterExoneratedTestVariantsOptions) (cfs []*ExoneratedTestVariant, err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/analysis.ReadClusterExoneratedTestVariants",
		attribute.String("project", opts.Project),
	)
	defer func() { tracing.End(s, err, attribute.Int("outcome", len(cfs))) }()
	q := c.client.Query(`
		WITH latest_failures_7d AS (
			SELECT
				project,
				cluster_algorithm,
				cluster_id,
				test_result_system,
				test_result_id,
				ARRAY_AGG(cf ORDER BY cf.last_updated DESC LIMIT 1)[OFFSET(0)] as r
			FROM clustered_failures cf
			WHERE cf.partition_time >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
			  AND project = @project
			  AND cluster_algorithm = @clusterAlgorithm
			  AND cluster_id = @clusterID
			  AND realm IN UNNEST(@realms)
			GROUP BY project, cluster_algorithm, cluster_id, test_result_system, ingested_invocation_id, test_result_id
			HAVING r.is_included
		)
		SELECT
			r.test_id as TestID,
			ANY_VALUE(r.variant) as Variant,
			COUNT(*) as CriticalFailuresExonerated,
			MAX(r.partition_time) as LastExoneration,
		FROM latest_failures_7d
		WHERE
			-- Presubmit run and tryjob is critical, and
			(r.build_critical AND
				-- Exonerated for a reason other than NOT_CRITICAL or UNEXPECTED_PASS.
				-- Passes are not ingested by LUCI Analysis, but if a test has both an unexpected pass
				-- and an unexpected failure, it will be exonerated for the unexpected pass.
				(EXISTS
					(SELECT TRUE FROM UNNEST(r.exonerations) e
					-- TODO(b/250541091): Temporarily exclude OCCURS_ON_MAINLINE.
					WHERE e.Reason = 'OCCURS_ON_OTHER_CLS')
				)
			)
		GROUP BY
			r.test_id,
			r.variant_hash
		ORDER BY LastExoneration DESC
		LIMIT 100
	`)
	q.DefaultDatasetID = bqutil.InternalDatasetID
	q.Parameters = []bigquery.QueryParameter{
		{Name: "clusterAlgorithm", Value: opts.ClusterID.Algorithm},
		{Name: "clusterID", Value: opts.ClusterID.ID},
		{Name: "realms", Value: opts.Realms},
		{Name: "project", Value: opts.Project},
	}
	it, err := q.Read(ctx)
	if err != nil {
		return nil, errors.Fmt("querying cluster exonerated test variants: %w", err)
	}
	tvs := []*ExoneratedTestVariant{}
	for {
		row := &ExoneratedTestVariant{}
		err := it.Next(row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, errors.Fmt("obtain next cluster exonerated test variant row: %w", err)
		}
		tvs = append(tvs, row)
	}
	return tvs, nil
}
