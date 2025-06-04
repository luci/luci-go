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

// Package views ensures BigQuery views are properly created and maintained.
package views

import (
	"context"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/tree_status/bqutil"
)

type makeTableMetadata func(treeName string) *bigquery.TableMetadata

var luciTreeViewQueries = map[string]makeTableMetadata{
	"statuses": func(treeName string) *bigquery.TableMetadata {
		return &bigquery.TableMetadata{
			ViewQuery: `SELECT * FROM internal.statuses WHERE tree_name = "` + treeName + `"`,
			Labels:    map[string]string{bq.MetadataVersionKey: "1"},
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

func ensureViews(ctx context.Context, bqClient *bigquery.Client) error {
	// Get datasets for tree names.
	datasetIDs, err := treeNameDatasets(ctx, bqClient)
	if err != nil {
		return errors.Fmt("get tree name datasets: %w", err)
	}
	// Create views that is common to each tree name dataset.
	for _, datasetID := range datasetIDs {
		if err := createViewsForTreeNameDataset(ctx, bqClient, datasetID); err != nil {
			return errors.Fmt("ensure view for LUCI tree name dataset %s: %w", datasetID, err)
		}
	}
	return nil
}

// createViewsForLUCIDataset creates views with the given tableSpecs under the given datasetID
func createViewsForTreeNameDataset(ctx context.Context, bqClient *bigquery.Client, datasetID string) error {
	treeName, err := bqutil.TreeNameForDataset(datasetID)
	if err != nil {
		return errors.Fmt("get tree name with dataset name %s: %w", datasetID, err)
	}
	for tableName, specFunc := range luciTreeViewQueries {
		table := bqClient.Dataset(datasetID).Table(tableName)
		spec := specFunc(treeName)
		if err := bq.EnsureTable(ctx, table, spec, bq.UpdateMetadata(), bq.RefreshViewInterval(time.Hour)); err != nil {
			return errors.Fmt("ensure view %s: %w", tableName, err)
		}
	}
	return nil
}

// treeNameDatasets returns all tree name datasets in the GCP Project.
// E.g. "chromium", "fuchsia-stem", ....
func treeNameDatasets(ctx context.Context, bqClient *bigquery.Client) ([]string, error) {
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
		// not belong any tree.
		if strings.EqualFold(d.DatasetID, bqutil.InternalDatasetID) {
			continue
		}
		datasets = append(datasets, d.DatasetID)
	}
	return datasets, nil
}
