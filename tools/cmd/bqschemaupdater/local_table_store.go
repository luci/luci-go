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

func (ts localTableStore) createTable(ctx context.Context, datasetID, tableID string, md *bigquery.TableMetadata) error {
	key := tableKey{datasetID, tableID}
	if _, ok := ts[key]; ok {
		return &googleapi.Error{Code: http.StatusConflict}
	}
	var cpy bigquery.TableMetadata
	if md != nil {
		cpy = *md
	}
	ts[key] = &cpy
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
