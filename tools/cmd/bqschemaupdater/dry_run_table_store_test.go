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
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"

	"cloud.google.com/go/bigquery"
	"golang.org/x/net/context"
	"google.golang.org/api/googleapi"
)

func TestCreateTableDoesNotMutate(t *testing.T) {
	ctx := context.Background()
	ts := dryRunTableStore{ts: localTableStore{}, w: ioutil.Discard}
	dID := "test_dataset"
	tID := "test_table"
	_, got := ts.getTableMetadata(ctx, dID, tID)
	want := &googleapi.Error{Code: http.StatusNotFound}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got: %v; want: %v", got, want)
	}
	err := ts.createTable(ctx, dID, tID, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, got = ts.getTableMetadata(ctx, dID, tID)
	want = &googleapi.Error{Code: http.StatusNotFound}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got: %v; want: %v", got, want)
	}
}

func TestUpdateTableDoesNotMutate(t *testing.T) {
	ctx := context.Background()
	ts := localTableStore{}
	dID := "test_dataset"
	tID := "test_table"
	err := ts.createTable(ctx, dID, tID, nil)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ts.getTableMetadata(ctx, dID, tID)
	if err != nil {
		t.Fatal(err)
	}
	drts := dryRunTableStore{ts: ts, w: ioutil.Discard}
	toUpdate := bigquery.TableMetadataToUpdate{Name: "a name"}
	drts.updateTable(ctx, dID, tID, toUpdate)
	got, err := drts.getTableMetadata(ctx, dID, tID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got: %v; want: %v", got, want)
	}
}
