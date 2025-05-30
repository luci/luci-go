// Copyright 2024 The LUCI Authors.
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

// Package ensureviews ensures BigQuery views are properly created and maintained.
package ensureviews

import (
	"context"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/bqutil"
	"go.chromium.org/luci/resultdb/pbutil"
)

type makeTableMetadata func(luciProject string) *bigquery.TableMetadata

var luciProjectViewQueries = map[string]makeTableMetadata{
	"text_artifacts": func(luciProject string) *bigquery.TableMetadata {
		return &bigquery.TableMetadata{
			ViewQuery: `SELECT * FROM internal.text_artifacts WHERE ` + projectWhereClause(luciProject),
			Labels:    map[string]string{bq.MetadataVersionKey: "3"},
		}
	},
	"invocations": func(luciProject string) *bigquery.TableMetadata {
		// Revalidate project as safeguard against SQL-Injection.
		if err := pbutil.ValidateProject(luciProject); err != nil {
			panic(err)
		}
		return &bigquery.TableMetadata{
			ViewQuery: `SELECT * FROM internal.invocations WHERE ` + projectWhereClause(luciProject),
			Labels:    map[string]string{bq.MetadataVersionKey: "3"},
		}
	},
}

// CronHandler is then entry-point for the ensure views cron job.
func CronHandler(ctx context.Context, gcpProject string) error {
	client, err := bq.NewClient(ctx, gcpProject)
	if err != nil {
		return errors.Fmt("create bq client: %w", err)
	}
	defer client.Close()

	if err := ensureViews(ctx, client); err != nil {
		return errors.Fmt("ensure view: %w", err)
	}
	return nil
}

func projectWhereClause(luciProject string) string {
	// Special treatment for chromium/chrome project views.
	if luciProject == "chromium" {
		// Enclose in brackets to ensure expression is evaluated to
		// a boolean wherever it is injected. I.E. So that
		// `WHERE something = a AND ` + projectWhereClause(luciProject) is safe.
		return `(project = "chromium" OR STARTS_WITH(project, "chromium-m"))`
	} else if luciProject == "chrome" {
		return `(project IN ("chromium", "chrome") OR STARTS_WITH(project, "chromium-m") OR STARTS_WITH(project, "chrome-m"))`
	} else if luciProject == "fuchsia" {
		return `(project IN ("fuchsia", "turquoise"))`
	}

	// Other LUCI Projects.
	// Revalidate project as safeguard against SQL-Injection.
	if err := pbutil.ValidateProject(luciProject); err != nil {
		panic(err)
	}
	return `(project = "` + luciProject + `")`
}

func ensureViews(ctx context.Context, bqClient *bigquery.Client) error {
	// Get datasets for LUCI projects.
	datasetIDs, err := projectDatasets(ctx, bqClient)
	if err != nil {
		return errors.Fmt("get LUCI project datasets: %w", err)
	}
	// Create views that is common to each LUCI project's dataset.
	for _, projectDatasetID := range datasetIDs {
		if err := createViewsForLUCIDataset(ctx, bqClient, projectDatasetID); err != nil {
			return errors.Fmt("ensure view for LUCI project dataset %s: %w", projectDatasetID, err)
		}
	}
	return nil
}

// createViewsForLUCIDataset creates views with the given tableSpecs under the given datasetID
func createViewsForLUCIDataset(ctx context.Context, bqClient *bigquery.Client, datasetID string) error {
	luciProject, err := bqutil.ProjectForDataset(datasetID)
	if err != nil {
		return errors.Fmt("get LUCI project with dataset name %s: %w", datasetID, err)
	}
	for tableName, specFunc := range luciProjectViewQueries {
		table := bqClient.Dataset(datasetID).Table(tableName)
		spec := specFunc(luciProject)
		if err := bq.EnsureTable(ctx, table, spec, bq.UpdateMetadata(), bq.RefreshViewInterval(time.Hour)); err != nil {
			return errors.Fmt("ensure view %s: %w", tableName, err)
		}
	}
	return nil
}

// projectDatasets returns all project datasets in the GCP Project.
// E.g. "chromium", "chrome", ....
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
		datasets = append(datasets, d.DatasetID)
	}
	return datasets, nil
}
