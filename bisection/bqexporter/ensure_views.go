// Copyright 2023 The LUCI Authors.
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

package bqexporter

import (
	"context"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/info"

	"go.chromium.org/luci/bisection/util/bqutil"
)

type makeTableMetadata func(luciProject string) *bigquery.TableMetadata

var luciProjectViewQueries = map[string]makeTableMetadata{
	"test_failure_analyses": func(luciProject string) *bigquery.TableMetadata {
		return &bigquery.TableMetadata{
			ViewQuery: `SELECT * FROM internal.test_failure_analyses WHERE project = "` + luciProject + `"`,
			Labels:    map[string]string{bq.MetadataVersionKey: "1"},
		}
	},
}

func EnsureViews(ctx context.Context) error {
	if !isEnabled(ctx) {
		logging.Warningf(ctx, "ensure view is not enabled")
	}

	client, err := bq.NewClient(ctx, info.AppID(ctx))
	if err != nil {
		return errors.Annotate(err, "create bq client").Err()
	}
	defer client.Close()

	if err := ensureViews(ctx, client); err != nil {
		return errors.Annotate(err, "ensure view").Err()
	}
	return nil
}

func ensureViews(ctx context.Context, bqClient *bigquery.Client) error {
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
