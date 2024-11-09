// Copyright 2018 The LUCI Authors.
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

package main

import (
	"context"
	"net/http"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/googleapi"
)

type tableStore interface {
	getTableMetadata(ctx context.Context, datasetID, tableID string) (*bigquery.TableMetadata, error)
	createTable(ctx context.Context, datasetID, tableID string, md *bigquery.TableMetadata) error
	updateTable(ctx context.Context, datasetID, tableID string, toUpdate bigquery.TableMetadataToUpdate) error
}

type bqTableStore struct {
	c *bigquery.Client
}

func isNotFound(e error) bool {
	err, ok := e.(*googleapi.Error)
	return ok && err.Code == http.StatusNotFound
}

func (ts bqTableStore) getTableMetadata(ctx context.Context, datasetID, tableID string) (*bigquery.TableMetadata, error) {
	t := ts.c.Dataset(datasetID).Table(tableID)
	return t.Metadata(ctx)
}

func (ts bqTableStore) createTable(ctx context.Context, datasetID, tableID string, md *bigquery.TableMetadata) error {
	t := ts.c.Dataset(datasetID).Table(tableID)
	return t.Create(ctx, md)
}

func (ts bqTableStore) updateTable(ctx context.Context, datasetID, tableID string, toUpdate bigquery.TableMetadataToUpdate) error {
	t := ts.c.Dataset(datasetID).Table(tableID)
	_, err := t.Update(ctx, toUpdate, "")
	return err
}
