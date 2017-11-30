// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"net/http"

	"cloud.google.com/go/bigquery"
	"golang.org/x/net/context"
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
