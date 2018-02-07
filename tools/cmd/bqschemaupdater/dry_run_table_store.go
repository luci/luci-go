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

	"cloud.google.com/go/bigquery"
	"golang.org/x/net/context"
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
	printSchema(ts.w, toUpdate.Schema, "\t")
	return nil
}

func printSchema(w io.Writer, s bigquery.Schema, prefix string) {
	for _, f := range s {
		fmt.Fprintf(w, "%sName: %v\n", prefix, f.Name)
		fmt.Fprintf(w, "%sDescription: %v\n", prefix, f.Description)
		fmt.Fprintf(w, "%sRepeated: %v\n", prefix, f.Repeated)
		fmt.Fprintf(w, "%sRequired: %v\n", prefix, f.Required)
		fmt.Fprintf(w, "%sType: %v\n", prefix, f.Type)
		if f.Type == bigquery.RecordFieldType {
			printSchema(w, f.Schema, prefix+"\t")
		}
		fmt.Fprint(w, "\n")
	}
}
