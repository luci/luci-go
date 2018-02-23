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
	"fmt"
	"io"

	"golang.org/x/net/context"

	"cloud.google.com/go/bigquery"

	"go.chromium.org/luci/common/data/text/indented"
)

type dryRunTableStore struct {
	ts tableStore
	w  io.Writer
}

func (ts dryRunTableStore) getTableMetadata(ctx context.Context, datasetID, tableID string) (*bigquery.TableMetadata, error) {
	fmt.Fprintf(ts.w, "Running getTableMetadata for datasetID %v tableID %v\n", datasetID, tableID)
	md, err := ts.ts.getTableMetadata(ctx, datasetID, tableID)
	if err != nil {
		fmt.Fprintln(ts.w, err)
		return nil, err
	}
	fmt.Fprintf(ts.w, "Got TableMetadata: %+v\n", md)
	return md, nil
}

func (ts dryRunTableStore) createTable(ctx context.Context, datasetID, tableID string, md *bigquery.TableMetadata) error {
	fmt.Fprintf(ts.w, "Would run createTable with datasetID %v, tableID %v, metadata %+v\n", datasetID, tableID, md)
	return nil
}

func (ts dryRunTableStore) updateTable(ctx context.Context, datasetID, tableID string, toUpdate bigquery.TableMetadataToUpdate) error {
	fmt.Fprintf(ts.w, "Would run updateTable with datasetID %v, tableID %v...\n", datasetID, tableID)
	fmt.Fprintf(ts.w, "...using TableMetadataToUpdate{Name: %v, Description: %v}\n", toUpdate.Name, toUpdate.Description)
	fmt.Fprintln(ts.w, "...and schema:")
	printSchema(&indented.Writer{Writer: ts.w}, toUpdate.Schema)
	return nil
}
