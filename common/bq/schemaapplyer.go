// Copyright 2021 The LUCI Authors.
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

package bq

import (
	"context"
	"net/http"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/caching"
)

// Table is implemented by *bigquery.Table.
// See its documentation for description of the methods below.
type Table interface {
	FullyQualifiedName() string
	Metadata(ctx context.Context) (md *bigquery.TableMetadata, err error)
	Create(ctx context.Context, md *bigquery.TableMetadata) error
	Update(ctx context.Context, md bigquery.TableMetadataToUpdate, etag string) (*bigquery.TableMetadata, error)
}

// SchemaApplyer provides methods to synchronise BigQuery schema
// to match a desired state.
type SchemaApplyer struct {
	cache caching.LRUHandle
}

// NewSchemaApplyer initialises a new schema applyer, using the given cache
// to cache BQ schema to avoid making duplicate BigQuery calls.
func NewSchemaApplyer(cache caching.LRUHandle) *SchemaApplyer {
	return &SchemaApplyer{
		cache: cache,
	}
}

// EnsureTable creates a BigQuery table if it doesn't exist and updates its
// schema if it is stale. Non-schema options, like Partitioning and Clustering
// settings, will be applied if the table is being created but will not be
// synchronised after creation.
//
// Existing fields will not be deleted.
//
// Example usage:
// // At top of file
// var schemaApplyer = bq.NewSchemaApplyer(
//	 caching.RegisterLRUCache(50) // depending on how many different
//                                // tables will be used.
// )
//
// ...
// // In method.
// table := client.Dataset("my_dataset").Table("my_table")
// schema := ... // e.g. from SchemaConverter.
// spec := &bigquery.TableMetadata{
//		TimePartitioning: &bigquery.TimePartitioning{
//			Field:      "partition_time",
//			Expiration: 540 * time.Day,
//		},
//		Schema: schema.Relax(), // Ensure no mandatory fields.
// }
// err := schemaApplyer.EnsureBQTable(ctx, table, spec)
// if err != nil {
//    if transient.Tag.In(err) {
//       // Handle retriable error.
//    } else {
//	     // Handle fatal error.
//    }
// }
func (s *SchemaApplyer) EnsureTable(ctx context.Context, t Table, spec *bigquery.TableMetadata) error {
	// Note: creating/updating the table inside GetOrCreate ensures that different
	// goroutines do not attempt to create/update the same table concurrently.
	cachedErr, err := s.cache.LRU(ctx).GetOrCreate(ctx, t.FullyQualifiedName(), func() (interface{}, time.Duration, error) {
		_, err := t.Metadata(ctx)
		apiErr, ok := err.(*googleapi.Error)
		switch {
		case ok && apiErr.Code == http.StatusNotFound:
			// Table doesn't exist. Create it and cache its existence for 5 minutes.
			err = createBQTable(ctx, t, spec)
			if err != nil {
				err = errors.Annotate(err, "create bq table").Err()
				if !transient.Tag.In(err) {
					// Cache the fatal error for one minute.
					return err, time.Minute, nil
				}
				return nil, 0, err
			}
			return nil, 5 * time.Minute, nil

		case ok && apiErr.Code == http.StatusForbidden:
			// No read table permission.
			return err, time.Minute, nil

		case err != nil:
			return nil, 0, transient.Tag.Apply(err)
		}

		// Table exists and is accessible.
		// Ensure its schema is up to date and remember that for 5 minutes.
		err = ensureBQTableFields(ctx, t, spec.Schema)
		return nil, 5 * time.Minute, errors.Annotate(err, "ensure bq table fields").Err()
	})
	if err != nil {
		return err
	}
	if cachedErr != nil {
		return cachedErr.(error)
	}
	return nil
}

func createBQTable(ctx context.Context, t Table, spec *bigquery.TableMetadata) error {
	err := t.Create(ctx, spec)
	apiErr, ok := err.(*googleapi.Error)
	switch {
	case ok && apiErr.Code == http.StatusConflict:
		// Table just got created. This is fine.
		return nil
	case ok && apiErr.Code == http.StatusForbidden:
		// No create table permission.
		return err
	case err != nil:
		return transient.Tag.Apply(err)
	default:
		logging.Infof(ctx, "Created BigQuery table %s", t.FullyQualifiedName())
		return nil
	}
}

// ensureBQTableFields adds missing fields to t.
func ensureBQTableFields(ctx context.Context, t Table, newSchema bigquery.Schema) error {
	err := retry.Retry(ctx, transient.Only(retry.Default), func() error {
		// We should retrieve Metadata in a retry loop because of the ETag check
		// below.
		md, err := t.Metadata(ctx)
		if err != nil {
			return err
		}

		combinedSchema := md.Schema

		// Append fields missing in the actual schema.
		mutated := false
		var appendMissing func(schema, newSchema bigquery.Schema) bigquery.Schema
		appendMissing = func(schema, newFields bigquery.Schema) bigquery.Schema {
			indexed := make(map[string]*bigquery.FieldSchema, len(schema))
			for _, c := range schema {
				indexed[c.Name] = c
			}

			for _, newField := range newFields {
				if existingField := indexed[newField.Name]; existingField == nil {
					// The field is missing.
					schema = append(schema, newField)
					mutated = true
				} else {
					existingField.Schema = appendMissing(existingField.Schema, newField.Schema)
				}
			}
			return schema
		}

		// Relax the new fields because we cannot add new required fields.
		combinedSchema = appendMissing(combinedSchema, newSchema)
		if !mutated {
			// Nothing to update.
			return nil
		}

		_, err = t.Update(ctx, bigquery.TableMetadataToUpdate{Schema: combinedSchema}, md.ETag)
		apiErr, ok := err.(*googleapi.Error)
		switch {
		case ok && apiErr.Code == http.StatusConflict:
			// ETag became stale since we requested it. Try again.
			return transient.Tag.Apply(err)

		case err != nil:
			return err

		default:
			logging.Infof(ctx, "Updated BigQuery table %s", t.FullyQualifiedName())
			return nil
		}
	}, nil)

	apiErr, ok := err.(*googleapi.Error)
	switch {
	case ok && apiErr.Code == http.StatusForbidden:
		// No read or modify table permission.
		return err
	case err != nil:
		return transient.Tag.Apply(err)
	}
	return nil
}
