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

	"cloud.google.com/go/bigquery"
	"go.chromium.org/luci/analysis/internal/bqutil"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"google.golang.org/api/iterator"
)

var schemaApplyer = bq.NewSchemaApplyer(bq.RegisterSchemaApplyerCache(50))

const rulesViewBaseQuery = `
	WITH items AS (
		SELECT
		ARRAY_AGG(rh1 ORDER BY rh1.last_updated DESC LIMIT 1)[OFFSET(0)] as row
		FROM internal.failure_association_rules_history rh1
		GROUP BY rh1.project, rh1.rule_id
	)
	SELECT
	row.*
	FROM items`

var datasetViewQueries = map[string]map[string]*bigquery.TableMetadata{
	"internal": {"failure_association_rules": &bigquery.TableMetadata{ViewQuery: rulesViewBaseQuery}},
}

type makeTableMetadata func(luciProject string) *bigquery.TableMetadata

var luciProjectViewQueries = map[string]makeTableMetadata{
	"failure_association_rules": func(luciProject string) *bigquery.TableMetadata {
		if !config.ProjectRe.MatchString(luciProject) {
			panic("invalid LUCI Project")
		}
		return &bigquery.TableMetadata{
			ViewQuery: `SELECT *
			FROM internal.failure_association_rules
			WHERE project = "` + luciProject + `"
		`}
	},
	"clustered_failures": func(luciProject string) *bigquery.TableMetadata {
		if !config.ProjectRe.MatchString(luciProject) {
			panic("invalid LUCI Project")
		}
		return &bigquery.TableMetadata{
			ViewQuery: `SELECT * FROM internal.clustered_failures WHERE project = "` + luciProject + `"
		`}
	},
	"cluster_summaries": func(luciProject string) *bigquery.TableMetadata {
		if !config.ProjectRe.MatchString(luciProject) {
			panic("invalid LUCI Project")
		}
		return &bigquery.TableMetadata{
			ViewQuery: `SELECT * FROM internal.cluster_summaries WHERE project = "` + luciProject + `"`}
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
			if err := schemaApplyer.EnsureTable(ctx, table, spec); err != nil {
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
		if err := schemaApplyer.EnsureTable(ctx, table, spec); err != nil {
			// TODO: Stop skipping this error when migration to centralised cluster table is done.
			if errors.Is(err, bq.ErrWrongTableKind) {
				continue
			}
			return errors.Annotate(err, "ensure view %s", tableName).Err()
		}
	}
	return nil
}

// projectDatasets returnes existing datasets that is for LUCI project.
func projectDatasets(ctx context.Context, bqClient *bigquery.Client) ([]string, error) {
	projectCfg, err := config.Projects(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "get project configs").Err()
	}
	datasetIDs, err := listDatasetIDs(ctx, bqClient)
	if err != nil {
		return nil, errors.Annotate(err, "list gcp datasets").Err()
	}
	projectDatasetIDs := []string{}
	for _, project := range projectCfg.Keys() {
		datasetID, err := bqutil.DatasetForProject(project)
		if err != nil {
			return nil, errors.Annotate(err, "get dataset name for project %s", project).Err()
		}
		if _, ok := datasetIDs[datasetID]; !ok {
			// No dataset provisioned for this LUCI project.
			continue
		}
		projectDatasetIDs = append(projectDatasetIDs, datasetID)
	}
	return projectDatasetIDs, nil
}

// listDatasetIDs returns all dataset ids as keys in a map for a gcp project.
func listDatasetIDs(ctx context.Context, bqClient *bigquery.Client) (map[string]bool, error) {
	datasets := map[string]bool{}
	di := bqClient.Datasets(ctx)
	for {
		d, err := di.Next()
		if err == iterator.Done {
			break
		} else if err != nil {
			return nil, err
		}
		datasets[d.DatasetID] = true
	}
	return datasets, nil
}
