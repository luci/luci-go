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

// ErrWrongTableKind represents a mismatch in BigQuery table type.
var ErrWrongTableKind = errors.New("cannot change a regular table into a view table or vice-versa")

// Table is implemented by *bigquery.Table.
// See its documentation for description of the methods below.
type Table interface {
	FullyQualifiedName() string
	Metadata(ctx context.Context, opts ...bigquery.TableMetadataOption) (md *bigquery.TableMetadata, err error)
	Create(ctx context.Context, md *bigquery.TableMetadata) error
	Update(ctx context.Context, md bigquery.TableMetadataToUpdate, etag string, opts ...bigquery.TableUpdateOption) (*bigquery.TableMetadata, error)
}

// SchemaApplyerCache is used by SchemaApplyer to avoid making redundant BQ
// calls.
//
// Instantiate it with RegisterSchemaApplyerCache(capacity) during init time.
type SchemaApplyerCache struct {
	handle caching.LRUHandle[string, error]
}

// RegisterSchemaApplyerCache allocates a process cached used by SchemaApplier.
//
// The capacity should roughly match expected number of tables the schema
// applier will work on.
//
// Must be called during init time.
func RegisterSchemaApplyerCache(capacity int) SchemaApplyerCache {
	return SchemaApplyerCache{caching.RegisterLRUCache[string, error](capacity)}
}

// SchemaApplyer provides methods to synchronise BigQuery schema
// to match a desired state.
type SchemaApplyer struct {
	cache SchemaApplyerCache
}

// NewSchemaApplyer initialises a new schema applyer, using the given cache
// to cache BQ schema to avoid making duplicate BigQuery calls.
func NewSchemaApplyer(cache SchemaApplyerCache) *SchemaApplyer {
	return &SchemaApplyer{
		cache: cache,
	}
}

// EnsureTable creates a BigQuery table if it doesn't exist and updates its
// schema (or a view query for view tables) if it is stale. Non-schema options,
// like Partitioning and Clustering settings, will be applied if the table is
// being created but will not be synchronized after creation.
//
// Existing fields will not be deleted.
//
// Example usage:
// // At top of file
// var schemaApplyer = bq.NewSchemaApplyer(
//
//	bq.RegisterSchemaApplyerCache(50 ) // depending on how many different
//	                                   // tables will be used.
//
// )
//
// ...
// // In method.
// table := client.Dataset("my_dataset").Table("my_table")
// schema := ... // e.g. from SchemaConverter.
//
//	spec := &bigquery.TableMetadata{
//	   TimePartitioning: &bigquery.TimePartitioning{
//	     Field:      "partition_time",
//	     Expiration: 540 * time.Day,
//	   },
//	   Schema: schema.Relax(), // Ensure no mandatory fields.
//	}
//
// err := schemaApplyer.EnsureBQTable(ctx, table, spec)
//
//	if err != nil {
//	   if transient.Tag.In(err) {
//	      // Handle retriable error.
//	   } else {
//	      // Handle fatal error.
//	   }
//	}
func (s *SchemaApplyer) EnsureTable(ctx context.Context, t Table, spec *bigquery.TableMetadata) error {
	// Note: creating/updating the table inside GetOrCreate ensures that different
	// goroutines do not attempt to create/update the same table concurrently.
	cachedErr, err := s.cache.handle.LRU(ctx).GetOrCreate(ctx, t.FullyQualifiedName(), func() (error, time.Duration, error) {
		if err := EnsureTable(ctx, t, spec); err != nil {
			if !transient.Tag.In(err) {
				// Cache the fatal error for one minute.
				return err, time.Minute, nil
			}
			return nil, 0, err
		}
		// Table is successfully ensured, remember for 5 minutes.
		return nil, 5 * time.Minute, nil
	})
	if err != nil {
		return err
	}
	return cachedErr
}

// ensureTableOpts captures options passed to EnsureTable(...).
type ensureTableOpts struct {
	// Whether all table settings should be enforced, not just schema.
	// This creates the possibility of edit wars as updates may
	// not converge.
	complete bool
}

// EnsureTableOption defines an option passed to EnsureTable(...).
type EnsureTableOption func(opts *ensureTableOpts)

// EnforceAllSettings specifies that EnsureTable should
// not just enforce the schema, but all table specification settings
// permitted by BigQuery and supported by the implementation.
//
// This list of settings is currently:
// - view definition
// - description
// - labels
//
// WARNING: If this option is used and there are multiple version of
// the table specification in production simultaneously (e.g. as canary
// and stable deployments), an edit war may ensue and EnsureTable may get
// stuck in an read-modify-update loop contending with other updates.
//
// Applications using this option must take steps to prevent such
// edit wars themselves, e.g. by only performing such table updates
// on a periodic cron job where edit wars, if they occur, are sufficiently
// slow that they do not cause contention.
func EnforceAllSettings() EnsureTableOption {
	return func(opts *ensureTableOpts) {
		opts.complete = true
	}
}

// EnsureTable creates a BigQuery table if it doesn't exist and updates its
// schema if it is stale.
//
// By default, non-schema fields, like View Definition, Partitioning and
// Clustering settings, will be applied if the table is being created but
// will not be synchronised after creation.
//
// To synchronise more of the table specification, including view definition,
// see EnforceAllSettings.
//
// Existing fields will not be deleted.
func EnsureTable(ctx context.Context, t Table, spec *bigquery.TableMetadata, options ...EnsureTableOption) error {
	var opts ensureTableOpts
	for _, apply := range options {
		apply(&opts)
	}
	return ensureTable(ctx, t, spec, opts)
}

// ensureTable creates a BigQuery table if it doesn't exist and updates its
// schema if it is stale.
func ensureTable(ctx context.Context, t Table, spec *bigquery.TableMetadata, opts ensureTableOpts) error {
	md, err := t.Metadata(ctx)
	apiErr, ok := err.(*googleapi.Error)
	switch {
	case ok && apiErr.Code == http.StatusNotFound:
		// Table doesn't exist. Create it now.
		if err = createBQTable(ctx, t, spec); err != nil {
			return errors.Annotate(err, "create bq table").Err()
		}
		return nil
	case ok && apiErr.Code == http.StatusForbidden:
		// No read table permission.
		return err
	case err != nil:
		return transient.Tag.Apply(err)
	}

	// Table exists and is accessible.
	// Ensure its specification is up to date.
	if md.Type == bigquery.ViewTable && len(spec.Schema) > 0 ||
		md.Type != bigquery.ViewTable && spec.ViewQuery != "" {
		// View without view query or non-View table with View query.
		return ErrWrongTableKind
	}
	if err = ensureBQTable(ctx, t, spec, opts); err != nil {
		return errors.Annotate(err, "ensure bq table").Err()
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

// ensureBQTable updates the BigQuery table t to match specification spec.
func ensureBQTable(ctx context.Context, t Table, spec *bigquery.TableMetadata, opts ensureTableOpts) error {
	err := retry.Retry(ctx, transient.Only(retry.Default), func() error {
		// We should retrieve Metadata in a retry loop because of the ETag check
		// below.
		md, err := t.Metadata(ctx)
		if err != nil {
			return err
		}

		var update bigquery.TableMetadataToUpdate
		mutated := false

		// Only consider Schema updates for tables that are not views.
		if md.Type != bigquery.ViewTable {
			combinedSchema, updated := appendMissing(md.Schema, spec.Schema)
			if updated {
				mutated = true
				update.Schema = combinedSchema
			}
		}
		// The following fields are subject of a possible edit war
		// if there are multiple versions of the table specification
		// deployed simultaneously (e.g. to canary and stable).
		// Only perform them if the application has asked us to enforce
		// the full table specification and it has a solution to
		// manage the edit war.
		if opts.complete {
			if md.Type == bigquery.ViewTable && spec.ViewQuery != md.ViewQuery {
				// Apply view updates to views.
				update.ViewQuery = spec.ViewQuery
				mutated = true
			}
			if md.Description != spec.Description {
				// Update description.
				update.Description = spec.Description
				mutated = true
			}
			setLabels, deleteLabels := diffLabels(md.Labels, spec.Labels)
			if len(setLabels) > 0 || len(deleteLabels) > 0 {
				// Update labels.
				for k, v := range setLabels {
					update.SetLabel(k, v)
				}
				for k := range deleteLabels {
					update.DeleteLabel(k)
				}
				mutated = true
			}
		}

		if !mutated {
			// Nothing to update.
			return nil
		}

		_, err = t.Update(ctx, update, md.ETag)
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

// appendMissing merges BigQuery schemas, adding to schema
// those fields that only exist in newFields and returning
// the result.
// Schema merging happens recursively, e.g. so that missing
// fields are added to struct-valued fields as well.
func appendMissing(schema, newFields bigquery.Schema) (combinedSchema bigquery.Schema, updated bool) {
	// Shallow copy schema to avoid changes propogating backwards
	// to the caller via arguments.
	combinedSchema = copySchema(schema)

	updated = false
	indexed := make(map[string]*bigquery.FieldSchema, len(combinedSchema))
	for _, c := range combinedSchema {
		indexed[c.Name] = c
	}

	for _, newField := range newFields {
		if existingField := indexed[newField.Name]; existingField == nil {
			// The field is missing.
			combinedSchema = append(combinedSchema, newField)
			updated = true
		} else {
			var didUpdate bool
			existingField.Schema, didUpdate = appendMissing(existingField.Schema, newField.Schema)
			if didUpdate {
				updated = true
			}
		}
	}
	return combinedSchema, updated
}

// copySchema creates a shallow copy of the existing schema.
func copySchema(schema bigquery.Schema) bigquery.Schema {
	if schema == nil {
		// Preserve existing 'nil' schemas as nil instead of
		// converting them to zero-length slices.
		return nil
	}
	copy := make(bigquery.Schema, 0, len(schema))
	for _, fieldSchema := range schema {
		fieldCopy := *fieldSchema
		copy = append(copy, &fieldCopy)
	}
	return copy
}

// diffLabels returns the difference between two set of BigQuery
// labels, in terms of the updates that need to be applied to
// currentLabels to reach newLabels:
// - the set of new/updated labels
// - the set of labels that should be deleted
func diffLabels(currentLabels, newLabels map[string]string) (setLabels map[string]string, deleteLabels map[string]struct{}) {
	setLabels = make(map[string]string)
	deleteLabels = make(map[string]struct{})

	// Identify new and updated labels in newLabels.
	for k, v := range newLabels {
		existingValue, ok := currentLabels[k]
		if !(ok && existingValue == v) {
			setLabels[k] = v
		}
	}
	// Identify labels that were deleted (in currentLabels but not in newLabels).
	for k := range currentLabels {
		if _, ok := newLabels[k]; !ok {
			deleteLabels[k] = struct{}{}
		}
	}
	return setLabels, deleteLabels
}
