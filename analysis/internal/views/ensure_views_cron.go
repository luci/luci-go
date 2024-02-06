// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package views contains methods to interact with BigQuery views.
package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/analysis/internal/bqutil"
	"go.chromium.org/luci/analysis/pbutil"
)

const rulesViewBaseQuery = `
	WITH items AS (
		SELECT
		project,
		rule_id,
		ARRAY_AGG(rh1 ORDER BY rh1.last_update_time DESC LIMIT 1)[OFFSET(0)] as row
		FROM internal.failure_association_rules_history rh1
		GROUP BY rh1.project, rh1.rule_id
	)
	SELECT
		project,
		rule_id,
		row.* EXCEPT(project, rule_id)
	FROM items`

const segmentsUnexpectedRealtimeQuery = `
	WITH merged_table AS(
		SELECT *
		FROM internal.test_variant_segment_updates
		WHERE has_recent_unexpected_results = 1
		UNION ALL
		SELECT *
		FROM internal.test_variant_segments
		WHERE has_recent_unexpected_results = 1
	), merged_table_grouped AS(
		SELECT
			project, test_id, variant_hash, ref_hash,
			ARRAY_AGG(m ORDER BY version DESC LIMIT 1)[OFFSET(0)] as row
		FROM merged_table m
		GROUP BY project, test_id, variant_hash, ref_hash
	)
	SELECT
		project, test_id, variant_hash, ref_hash,
		row.variant AS variant,
		row.ref AS ref,
		-- Omit has_recent_unexpected_results here as all rows have unexpected results.
		row.segments AS segments,
		row.version AS version
	FROM merged_table_grouped`

// TODO(beining@): update this to query from the internal.test_variant_segments_unexpected_realtime after its been created.
const segmentsUnexpectedRealtimePerProjectQuery = `
	WITH merged_table AS(
		SELECT *
		FROM internal.test_variant_segment_updates
		WHERE project = "%[1]s" AND has_recent_unexpected_results = 1
		UNION ALL
		SELECT *
		FROM internal.test_variant_segments
		WHERE project = "%[1]s" AND has_recent_unexpected_results = 1
	), merged_table_grouped AS(
		SELECT
			project, test_id, variant_hash, ref_hash,
			ARRAY_AGG(m ORDER BY version DESC LIMIT 1)[OFFSET(0)] as row
		FROM merged_table m
		GROUP BY project, test_id, variant_hash, ref_hash
	)
	SELECT
		project, test_id, variant_hash, ref_hash,
		row.variant AS variant,
		row.ref AS ref,
		-- Omit has_recent_unexpected_results here as all rows have unexpected results.
		row.segments AS segments,
		row.version AS version
	FROM merged_table_grouped`

var datasetViewQueries = map[string]map[string]*bigquery.TableMetadata{
	"internal": {
		"failure_association_rules": &bigquery.TableMetadata{
			ViewQuery: rulesViewBaseQuery,
			Labels:    map[string]string{bq.MetadataVersionKey: "2"},
		},
		"test_variant_segments_unexpected_realtime": &bigquery.TableMetadata{
			Description: "Contains test variant histories segmented by change point analysis, limited to test variants with unexpected" +
				" results in postsubmit in the last 90 days. See go/luci-test-variant-analysis-design.",
			ViewQuery: segmentsUnexpectedRealtimeQuery,
			Labels:    map[string]string{bq.MetadataVersionKey: "1"},
		}},
}

type makeTableMetadata func(luciProject string) *bigquery.TableMetadata

var luciProjectViewQueries = map[string]makeTableMetadata{
	"failure_association_rules": func(luciProject string) *bigquery.TableMetadata {
		// Revalidate project as safeguard against SQL-Injection.
		if err := pbutil.ValidateProject(luciProject); err != nil {
			panic(err)
		}

		return &bigquery.TableMetadata{
			Description: "Failure association rules for " + luciProject + ". See go/luci-analysis-concepts#failure-association-rules.",
			ViewQuery:   `SELECT * FROM internal.failure_association_rules WHERE project = "` + luciProject + `"`,
			Labels:      map[string]string{bq.MetadataVersionKey: "1"},
		}
	},
	"clustered_failures": func(luciProject string) *bigquery.TableMetadata {
		// Revalidate project as safeguard against SQL-Injection.
		if err := pbutil.ValidateProject(luciProject); err != nil {
			panic(err)
		}
		return &bigquery.TableMetadata{
			Description: "Clustered test failures for " + luciProject + ". Each failure is repeated for each cluster it is contained in.",
			ViewQuery:   `SELECT * FROM internal.clustered_failures WHERE project = "` + luciProject + `"`,
			Labels:      map[string]string{bq.MetadataVersionKey: "1"},
		}
	},
	"cluster_summaries": func(luciProject string) *bigquery.TableMetadata {
		// Revalidate project as safeguard against SQL-Injection.
		if err := pbutil.ValidateProject(luciProject); err != nil {
			panic(err)
		}
		return &bigquery.TableMetadata{
			Description: "Test failure clusters for " + luciProject + " with cluster metrics. Periodically updated from clustered_failures table with ~15 minute staleness.",
			ViewQuery:   `SELECT * FROM internal.cluster_summaries WHERE project = "` + luciProject + `"`,
			Labels:      map[string]string{bq.MetadataVersionKey: "1"},
		}
	},
	"test_verdicts": func(luciProject string) *bigquery.TableMetadata {
		// Revalidate project as safeguard against SQL-Injection.
		if err := pbutil.ValidateProject(luciProject); err != nil {
			panic(err)
		}
		return &bigquery.TableMetadata{
			Description: "Contains all test verdicts produced by " + luciProject + ". See go/luci-analysis-verdict-export-proposal.",
			ViewQuery:   `SELECT * FROM internal.test_verdicts WHERE project = "` + luciProject + `"`,
			Labels:      map[string]string{bq.MetadataVersionKey: "1"},
		}
	},
	"test_variant_segments": func(luciProject string) *bigquery.TableMetadata {
		// Revalidate project as safeguard against SQL-Injection.
		if err := pbutil.ValidateProject(luciProject); err != nil {
			panic(err)
		}
		return &bigquery.TableMetadata{
			Description: "Contains test variant histories segmented by change point analysis. See go/luci-test-variant-analysis-design.",
			ViewQuery:   `SELECT * FROM internal.test_variant_segments WHERE project = "` + luciProject + `"`,
			Labels:      map[string]string{bq.MetadataVersionKey: "1"},
		}
	},
	"test_variant_segments_unexpected_realtime": func(luciProject string) *bigquery.TableMetadata {
		// Revalidate project as safeguard against SQL-Injection.
		if err := pbutil.ValidateProject(luciProject); err != nil {
			panic(err)
		}
		viewQuery := fmt.Sprintf(segmentsUnexpectedRealtimePerProjectQuery, luciProject)
		return &bigquery.TableMetadata{
			Description: "Contains test variant histories segmented by change point analysis, limited to test variants with unexpected" +
				" results in postsubmit in the last 90 days. See go/luci-test-variant-analysis-design.",
			ViewQuery: viewQuery,
			Labels:    map[string]string{bq.MetadataVersionKey: "2"},
		}
	},
}

// CronHandler is then entry-point for the ensure views cron job.
func CronHandler(ctx context.Context, gcpProject string) (retErr error) {
	client, err := bqutil.Client(ctx, gcpProject)
	if err != nil {
		return errors.Annotate(err, "create bq client").Err()
	}
	defer func() {
		if err := client.Close(); err != nil && retErr == nil {
			retErr = errors.Annotate(err, "closing bq client").Err()
		}
	}()
	if err := ensureViews(ctx, client); err != nil {
		logging.Errorf(ctx, "ensure views: %s", err)
		return err
	}
	return nil
}

func ensureViews(ctx context.Context, bqClient *bigquery.Client) error {
	// Create views for individual datasets.
	for datasetID, tableSpecs := range datasetViewQueries {
		for tableName, spec := range tableSpecs {
			table := bqClient.Dataset(datasetID).Table(tableName)
			if err := bq.EnsureTable(ctx, table, spec, bq.UpdateMetadata(), bq.RefreshViewInterval(time.Hour)); err != nil {
				return errors.Annotate(err, "ensure view %s", tableName).Err()
			}
		}
	}
	// Get datasets for LUCI projects.
	datasetIDs, err := projectDatasets(ctx, bqClient)
	if err != nil {
		return errors.Annotate(err, "get LUCI project datasets").Err()
	}
	// Create views that is common to each LUCI project's dataset.
	for _, projectDatasetID := range datasetIDs {
		if err := createViewsForLUCIDataset(ctx, bqClient, projectDatasetID); err != nil {
			return errors.Annotate(err, "ensure view for LUCI project dataset %s", projectDatasetID).Err()
		}
	}
	return nil
}

// createViewsForLUCIDataset creates views with the given tableSpecs under the given datasetID
func createViewsForLUCIDataset(ctx context.Context, bqClient *bigquery.Client, datasetID string) error {
	luciProject, err := bqutil.ProjectForDataset(datasetID)
	if err != nil {
		return errors.Annotate(err, "get LUCI project with dataset name %s", datasetID).Err()
	}
	for tableName, specFunc := range luciProjectViewQueries {
		table := bqClient.Dataset(datasetID).Table(tableName)
		spec := specFunc(luciProject)
		if err := bq.EnsureTable(ctx, table, spec, bq.UpdateMetadata(), bq.RefreshViewInterval(time.Hour)); err != nil {
			return errors.Annotate(err, "ensure view %s", tableName).Err()
		}
	}
	return nil
}

// projectDatasets returns all project datasets in the GCP Project.
// E.g. "chromium", "chromeos", ....
func projectDatasets(ctx context.Context, bqClient *bigquery.Client) ([]string, error) {
	var datasets []string
	di := bqClient.Datasets(ctx)
	for {
		d, err := di.Next()
		if err == iterator.Done {
			break
		} else if err != nil {
			return nil, err
		}
		// The internal dataset is a special dataset that does
		// not belong to a LUCI project.
		if strings.EqualFold(d.DatasetID, bqutil.InternalDatasetID) {
			continue
		}
		// Same for the experiments dataset.
		if strings.EqualFold(d.DatasetID, "experiments") {
			continue
		}
		datasets = append(datasets, d.DatasetID)
	}
	return datasets, nil
}
