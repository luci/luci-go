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
	"reflect"
	"testing"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/googleapi"
)

type testSchemaA struct {
	testField string
}

type testSchemaB struct {
	testField int
}

func newTestSchema(t *testing.T) bigquery.Schema {
	s, err := bigquery.InferSchema(testSchemaA{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCreateTable(t *testing.T) {
	ctx := context.Background()
	datasetID := "test_dataset"
	tableID := "test_table"
	s := newTestSchema(t)
	md := &bigquery.TableMetadata{Schema: s}
	t.Run("SimpleCreate", func(t *testing.T) {
		ts := localTableStore{}
		if err := ts.createTable(ctx, datasetID, tableID, md); err != nil {
			t.Fatal(err)
		}
		want := &bigquery.TableMetadata{Schema: s}
		got, err := ts.getTableMetadata(ctx, datasetID, tableID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got: %v; want: %v", got, want)
		}
	})
	t.Run("AlreadyCreated", func(t *testing.T) {
		ts := localTableStore{}
		if err := ts.createTable(ctx, datasetID, tableID, md); err != nil {
			t.Fatal(err)
		}
		want := &googleapi.Error{Code: http.StatusConflict}
		got := ts.createTable(ctx, datasetID, tableID, md)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got: %v; want: %v", got, want)
		}
	})
}

func TestGetTableMetadata(t *testing.T) {
	ctx := context.Background()
	datasetID := "test_dataset"
	tableID := "test_table"
	t.Run("TableDoesNotExist", func(t *testing.T) {
		ts := localTableStore{}
		want := &googleapi.Error{Code: http.StatusNotFound}
		_, got := ts.getTableMetadata(ctx, datasetID, tableID)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got: %v; want: %v", got, want)
		}
	})
	t.Run("TableExists", func(t *testing.T) {
		ts := localTableStore{}
		want := newTestSchema(t)
		md := &bigquery.TableMetadata{Schema: want}
		if err := ts.createTable(ctx, datasetID, tableID, md); err != nil {
			t.Fatal(err)
		}
		md, err := ts.getTableMetadata(ctx, datasetID, tableID)
		got := md.Schema
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got: %v; want: %v", got, want)
		}
	})
}

func TestUpdateTable(t *testing.T) {
	ctx := context.Background()
	ts := localTableStore{}
	datasetID := "test_dataset"
	s := newTestSchema(t)
	initialMD := &bigquery.TableMetadata{Schema: s}
	otherS, err := bigquery.InferSchema(testSchemaB{})
	if err != nil {
		t.Fatal(err)
	}
	type updateTestCase struct {
		tableID     string
		toUpdate    bigquery.TableMetadataToUpdate
		want        *bigquery.TableMetadata
		wantErr     error
		createTable bool
	}
	cases := []updateTestCase{
		{
			tableID:     "table_dne",
			toUpdate:    bigquery.TableMetadataToUpdate{},
			want:        nil,
			wantErr:     &googleapi.Error{Code: http.StatusNotFound},
			createTable: false,
		},
		{
			tableID:     "table_noop",
			toUpdate:    bigquery.TableMetadataToUpdate{},
			want:        &bigquery.TableMetadata{Schema: s},
			createTable: true,
		},
		{
			tableID: "table_change_one_thing",
			toUpdate: bigquery.TableMetadataToUpdate{
				Name: "test_name",
			},
			want: &bigquery.TableMetadata{
				Schema: s,
				Name:   "test_name",
			},
			createTable: true,
		},
		{
			tableID: "table_change_everything",
			toUpdate: bigquery.TableMetadataToUpdate{
				Name:        "test_name",
				Description: "test_desc",
				Schema:      otherS,
			},
			want: &bigquery.TableMetadata{
				Name:        "test_name",
				Description: "test_desc",
				Schema:      otherS,
			},
			createTable: true,
		},
	}
	for _, tc := range cases {
		if tc.createTable {
			if err := ts.createTable(ctx, datasetID, tc.tableID, initialMD); err != nil {
				t.Fatal(err)
			}
		}
		err = ts.updateTable(ctx, datasetID, tc.tableID, tc.toUpdate)
		if got := err; !reflect.DeepEqual(got, tc.wantErr) {
			t.Errorf("unexpected error: got: %v; want: %v", got, tc.wantErr)
		}
		md, err := ts.getTableMetadata(ctx, datasetID, tc.tableID)
		if got := err; !reflect.DeepEqual(got, tc.wantErr) {
			t.Errorf("unexpected error: got: %v; want: %v", got, tc.wantErr)
		}
		if got := md; !reflect.DeepEqual(got, tc.want) {
			t.Errorf("update failed for: %v; got: %v; want: %v", tc.tableID, got, tc.want)
		}
	}
}
