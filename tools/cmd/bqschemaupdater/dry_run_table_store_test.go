// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

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
	err := ts.createTable(ctx, dID, tID)
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
	err := ts.createTable(ctx, dID, tID)
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
