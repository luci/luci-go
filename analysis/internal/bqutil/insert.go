// Copyright 2022 The LUCI Authors.
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

package bqutil

import (
	"context"
	"net/http"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
)

// Inserter provides methods to insert rows into a BigQuery table.
type Inserter struct {
	table     *bigquery.Table
	batchSize int
}

// NewInserter initialises a new inserter.
func NewInserter(table *bigquery.Table, batchSize int) *Inserter {
	return &Inserter{
		table:     table,
		batchSize: batchSize,
	}
}

// Put inserts the given rows into BigQuery.
func (i *Inserter) Put(ctx context.Context, rows []*bq.Row) error {
	inserter := i.table.Inserter()
	for i, batch := range i.batch(rows) {
		if err := inserter.Put(ctx, batch); err != nil {
			return errors.Fmt("putting batch %v: %w", i, err)
		}
	}
	return nil
}

// batch divides the rows to be inserted into batches of at most batchSize.
func (i *Inserter) batch(rows []*bq.Row) [][]*bq.Row {
	var result [][]*bq.Row
	pages := (len(rows) + (i.batchSize - 1)) / i.batchSize
	for p := 0; p < pages; p++ {
		start := p * i.batchSize
		end := start + i.batchSize
		if end > len(rows) {
			end = len(rows)
		}
		page := rows[start:end]
		result = append(result, page)
	}
	return result
}

func hasReason(apiErr *googleapi.Error, reason string) bool {
	for _, e := range apiErr.Errors {
		if e.Reason == reason {
			return true
		}
	}
	return false
}

// PutWithRetries puts rows into BigQuery.
// Retries on transient errors.
func (i *Inserter) PutWithRetries(ctx context.Context, rows []*bq.Row) error {
	return retry.Retry(ctx, transient.Only(retry.Default), func() error {
		err := i.Put(ctx, rows)

		switch e := err.(type) {
		case *googleapi.Error:
			if e.Code == http.StatusForbidden && hasReason(e, "quotaExceeded") {
				err = transient.Tag.Apply(err)
			}
		}

		return err
	}, retry.LogCallback(ctx, "bigquery_put"))
}

// FatalError returns true if the error is a known fatal error.
func FatalError(err error) bool {
	if apiErr, ok := err.(*googleapi.Error); ok && apiErr.Code == http.StatusForbidden && hasReason(apiErr, "accessDenied") {
		return true
	}
	return false
}
