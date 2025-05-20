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
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/caching"
)

// ErrWrongTableKind represents a mismatch in BigQuery table type.
var ErrWrongTableKind = errors.New("cannot change a regular table into a view table or vice-versa")

var errMetadataVersionLabelMissing = errors.New("table definition is missing MetadataVersionKey label or label value is not a positive integer")

var errViewRefreshEnabledOnNonView = errors.New("RefreshViewInterval option cannot be used on a table without a ViewQuery")

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
//		spec := &bigquery.TableMetadata{
//		   TimePartitioning: &bigquery.TimePartitioning{
//		     Field:      "partition_time",
//		     Expiration: 540 * time.Day,
//		   },
//	    // Ensure no mandatory fields.
//		   Schema: schema.Relax(), // or bq.RelaxSchema(schema) if schema contain default values.
//		}
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
func (s *SchemaApplyer) EnsureTable(ctx context.Context, t Table, spec *bigquery.TableMetadata, options ...EnsureTableOption) error {
	// Note: creating/updating the table inside GetOrCreate ensures that different
	// goroutines do not attempt to create/update the same table concurrently.
	cachedErr, err := s.cache.handle.LRU(ctx).GetOrCreate(ctx, t.FullyQualifiedName(), func() (error, time.Duration, error) {
		if err := EnsureTable(ctx, t, spec, options...); err != nil {
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
	// Whether metadata versioning is enabled and metadata
	// updates can be rolled out, not just schema.
	metadataVersioned bool

	// Whether view definitions should be periodically refreshed
	// so that indirect schema updates can be propogated to schema.
	viewRefreshEnabled bool
	// The view refresh interval.
	viewRefreshInterval time.Duration
}

// EnsureTableOption defines an option passed to EnsureTable(...).
type EnsureTableOption func(opts *ensureTableOpts)

// RefreshViewInterval ensures the BigQuery view definition is
// updated if it has not been updated for duration d. This is
// to ensure indirect schema changes are propogated.
//
// Scenario:
// You have a view defined with the SQL:
//
//	`SELECT * FROM base_table WHERE project = 'chromium'`.
//
// By default, schema changes to base_table will not be
// reflected in the schema for the view (e.g. as seen in BigQuery UI).
// This is a usability issue for users of the view.
//
// To cause indirect schema changes to propogate, when this
// option is set, the view definition will be prefixed with a
// one line comment like:
// -- Indirect schema version: 2023-05-01T12:34:56Z
//
// The view (including comment) will be periodically refreshed
// if duration d has elapsed, triggering BigQuery to refresh the
// view schema.
//
// If this option is set but the table definition is not for
// a view, an error will be returned by EnsureTable(...).
func RefreshViewInterval(d time.Duration) EnsureTableOption {
	return func(opts *ensureTableOpts) {
		opts.viewRefreshEnabled = true
		opts.viewRefreshInterval = d
	}
}

// MetadataVersionKey is the label key used to version table
// metadata. Increment the integer assigned to this label
// to push updated table metadata.
//
// This label must be used in conjunction with the UpdateMetadata()
// EnsureTable(...) option to have effect.
//
// The value assigned to this label must be a positive integer,
// like "1" or "9127".
//
// See UpdateMetadata option for usage.
const MetadataVersionKey = "metadata_version"

// UpdateMetadata allows the non-schema metadata to be updated in
// EnsureTable(...), namely the view definition, clustering settings,
// description and labels.
//
// This option requires the caller to use the `MetadataVersionKey`
// label to control metadata update rollouts.
//
// Usage:
//
// The table definition passed to EnsureTable(...) must define
// a label with the key `MetadataVersionKey`. The value of
// the label must be a positive integer. Incrementing the integer
// will trigger an update of table metadata.
//
//	table := client.Dataset("my_dataset").Table("my_table")
//	spec :=	&bigquery.TableMetadata{
//	   ...
//	   Labels: map[string]string {
//	      // Increment to update table metadata.
//	      MetadataVersionKey: "2",
//	   }
//	}
//	err := EnsureTable(ctx, table, spec, UpdateMetadata())
//
// Rationale:
// Without a system to control rollouts, if there are multiple
// versions of the table metadata in production simultaneously
// (e.g. in canary and stable deployments), an edit war may
// ensue.
//
// Such an edit war scenario is not an issue when we update
// schema only as columns are only added, never removed, so
// schema will always converge to the union of all columns.
func UpdateMetadata() EnsureTableOption {
	return func(opts *ensureTableOpts) {
		opts.metadataVersioned = true
	}
}

// EnsureTable creates a BigQuery table if it doesn't exist and updates its
// schema if it is stale.
//
// By default, non-schema metadata, like View Definition, Partitioning and
// Clustering settings, will be applied if the table is being created but
// will not be synchronised after creation.
//
// To synchronise more of the table metadata, including view definition,
// description and labels, see the MetadataVersioned option.
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
	// If metadata versioning is enabled, confirm the caller has
	// specified a version.
	if opts.metadataVersioned && metadataVersion(spec.Labels) <= 0 {
		return errMetadataVersionLabelMissing
	}
	if spec.ViewQuery == "" && opts.viewRefreshEnabled {
		return errViewRefreshEnabledOnNonView
	}

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
	case ok && apiErr.Code == http.StatusBadRequest:
		// Bad table name, etc.
		return err
	case err != nil:
		return transient.Tag.Apply(err)
	}

	// Table exists and is accessible.
	// Ensure its specification is up to date.
	if (md.Type == bigquery.ViewTable) != (spec.ViewQuery != "") {
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
	case ok && (apiErr.Code == http.StatusNotFound || apiErr.Code == http.StatusBadRequest):
		// Dataset or project not found, bad table name, etc.
		return err
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

		if md.Type != bigquery.ViewTable {
			// Only consider Schema updates for tables that are not views.
			combinedSchema, updated := appendMissing(md.Schema, spec.Schema)
			if updated {
				mutated = true
				update.Schema = combinedSchema
			}
		}

		// The following fields are subject of a possible edit war
		// if there are multiple versions of the table specification
		// deployed simultaneously (e.g. to canary and stable).
		// Only perform them if there is a versioning scheme
		// in place to prevent an edit war.
		pushMetadata := opts.metadataVersioned && metadataVersion(spec.Labels) > metadataVersion(md.Labels)

		if md.Type == bigquery.ViewTable {
			// Update view query, if necessary.

			existingQuery := parseViewQuery(md.ViewQuery)

			now := clock.Now(ctx)
			updateQueryContent := pushMetadata && spec.ViewQuery != existingQuery.query
			isViewStale := opts.viewRefreshEnabled && now.Sub(existingQuery.lastIndirectSchemaUpdate) > opts.viewRefreshInterval

			if updateQueryContent || isViewStale {
				var newQuery viewQuery
				if updateQueryContent {
					newQuery.query = spec.ViewQuery
				} else {
					// Only update the indirect schema version header
					// without changing the rest of the query. New
					// query contents should only be rolled out with
					// an uprev of the schema version.
					newQuery.query = existingQuery.query
				}

				if opts.viewRefreshEnabled {
					newQuery.lastIndirectSchemaUpdate = now
				} else {
					newQuery.lastIndirectSchemaUpdate = time.Time{}
				}

				update.ViewQuery = newQuery.SQL()
				mutated = true
			}
		}
		if pushMetadata && isClusteringDifferent(md.Clustering, spec.Clustering) {
			// Update clustering.
			update.Clustering = spec.Clustering
			mutated = true
		}
		if pushMetadata && md.Description != spec.Description {
			// Update description.
			update.Description = spec.Description
			mutated = true
		}
		setLabels, deleteLabels := diffLabels(md.Labels, spec.Labels)
		if pushMetadata && (len(setLabels) > 0 || len(deleteLabels) > 0) {
			// Update labels.
			for k, v := range setLabels {
				update.SetLabel(k, v)
			}
			for k := range deleteLabels {
				update.DeleteLabel(k)
			}
			mutated = true
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

// metadataVersion attempts to extract the metadata version from the
// given table labels, and returns it if it is found.
// If it cannot be found, or is invalid, zero (0) is returned.
func metadataVersion(labels map[string]string) int64 {
	if labels == nil {
		// No labels. Invalid.
		return 0
	}
	versionString := labels[MetadataVersionKey]
	i, err := strconv.ParseInt(versionString, 10, 64)
	if err != nil || i < 0 {
		// No version string or negative version. Invalid.
		return 0
	}
	if versionString != fmt.Sprintf("%v", i) {
		// Integer is not in its canoncial string representation,
		// e.g. "+1" instead of "1". Invalid.
		return 0
	}
	return i
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

// isClusteringDifferent returns whether clusterings settings a and b
// are semantically different.
func isClusteringDifferent(a, b *bigquery.Clustering) bool {
	aLength := 0
	if a != nil {
		aLength = len(a.Fields)
	}
	bLength := 0
	if b != nil {
		bLength = len(b.Fields)
	}
	if aLength != bLength {
		return true
	}
	for i := 0; i < aLength; i++ {
		if !strings.EqualFold(a.Fields[i], b.Fields[i]) {
			return true
		}
	}
	return false
}

var indirectSchemaVersionRE = regexp.MustCompile(`^-- Indirect schema version: ([0-9\-:TZ]+)$`)

// viewQuery is the logical representation of a SQL query
// in a BigQuery view, separating the indirect schema version
// header comment (if any) from the residual query content.
type viewQuery struct {
	// lastIndirectSchemaUpdate is the timestamp in the Indirect schema version
	// header that may appear before the query, if any.
	// If there is no such header, this is the zero time (time.Time{}).
	lastIndirectSchemaUpdate time.Time
	// query is the residual query content.
	query string
}

// parseViewQuery parses the SQL of a view query, separating
// the indirect schema version header from the rest of the
// query content.
func parseViewQuery(sql string) viewQuery {
	lines := strings.Split(sql, "\n")

	// Try to find the header in the first line of the SQL.
	matches := indirectSchemaVersionRE.FindStringSubmatch(lines[0])
	if len(matches) == 0 {
		// No indirect schema version header found.
		return viewQuery{
			lastIndirectSchemaUpdate: time.Time{}, // Use zero time.
			query:                    sql,
		}
	}

	// Indirect schema version header is present.
	timestampString := matches[1]
	residualSQL := strings.Join(lines[1:], "\n")

	lastUpdateTime, err := time.Parse(time.RFC3339, timestampString)
	if err != nil {
		// Invalid timestamp.
		return viewQuery{
			lastIndirectSchemaUpdate: time.Time{}, // Use zero time.
			query:                    residualSQL,
		}
	}
	return viewQuery{
		lastIndirectSchemaUpdate: lastUpdateTime,
		query:                    residualSQL,
	}
}

// SQL returns the raw SQL representation of the view query.
func (v viewQuery) SQL() string {
	var b strings.Builder
	if (v.lastIndirectSchemaUpdate != time.Time{}) {
		b.WriteString(fmt.Sprintf("-- Indirect schema version: %s\n", v.lastIndirectSchemaUpdate.Format(time.RFC3339)))
	}
	b.WriteString(v.query)
	return b.String()
}
