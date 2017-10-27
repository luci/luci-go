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

type tableKey struct {
	datasetID, tableID string
}

type localTableStore map[tableKey]*bigquery.TableMetadata

func (ts localTableStore) getTableMetadata(ctx context.Context, datasetID, tableID string) (*bigquery.TableMetadata, error) {
	if md, ok := ts[tableKey{datasetID, tableID}]; ok {
		return md, nil
	}
	return nil, &googleapi.Error{Code: http.StatusNotFound}
}

func (ts localTableStore) createTable(ctx context.Context, datasetID, tableID string, option ...bigquery.CreateTableOption) error {
	md := &bigquery.TableMetadata{}
	for _, o := range option {
		if s, ok := o.(bigquery.Schema); ok {
			md.Schema = s
			break
		}
	}
	key := tableKey{datasetID, tableID}
	if _, ok := ts[key]; ok {
		return &googleapi.Error{Code: http.StatusConflict}
	}
	ts[key] = md
	return nil
}

func (ts localTableStore) updateTable(ctx context.Context, datasetID, tableID string, toUpdate bigquery.TableMetadataToUpdate) error {
	md, ok := ts[tableKey{datasetID, tableID}]
	if !ok {
		return &googleapi.Error{Code: http.StatusNotFound}
	}
	if toUpdate.Description != nil {
		md.Description = toUpdate.Description.(string)
	}
	if toUpdate.Name != nil {
		md.Name = toUpdate.Name.(string)
	}
	if toUpdate.Schema != nil {
		md.Schema = toUpdate.Schema
	}
	return nil
}
