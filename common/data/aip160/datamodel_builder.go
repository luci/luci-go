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

package aip160

import (
	"go.chromium.org/luci/common/data/aip132"
)

type FieldBuilder struct {
	column Field
}

// NewField starts building a new field.
func NewField() *FieldBuilder {
	return &FieldBuilder{Field{}}
}

// WithFieldPath specifies the field path the field maps to
// in the returned resource. Field paths are described in AIP-161.
//
// For convenience, the field path is described here as a set of
// segments where each segment is joined by the traversal operator (.).
// E.g. the field path "metrics.`some-metric`.value" would be specified
// as ["metrics", "some-metric", "value"].
func (c *FieldBuilder) WithFieldPath(segments ...string) *FieldBuilder {
	c.column.fieldPath = aip132.NewFieldPath(segments...)
	return c
}

// WithBackend specifies the FieldBackend that implements database
// filtering/sorting for this field.
func (c *FieldBuilder) WithBackend(backend FieldBackend) *FieldBuilder {
	c.column.backend = backend
	return c
}

// Sortable specifies this field can be sorted on.
func (c *FieldBuilder) Sortable() *FieldBuilder {
	c.column.sortable = true
	return c
}

// Filterable specifies this field can be filtered on.
func (c *FieldBuilder) Filterable() *FieldBuilder {
	c.column.filterable = true
	return c
}

// FilterableImplicitly specifies this field can be filtered on implicitly.
// This means that AIP-160 filter expressions not referencing any
// particular field will try to search in this field.
func (c *FieldBuilder) FilterableImplicitly() *FieldBuilder {
	c.column.filterable = true
	c.column.implicitFilter = true
	return c
}

// Build returns the built field.
func (c *FieldBuilder) Build() *Field {
	result := &Field{}
	*result = c.column
	return result
}

type TableBuilder struct {
	fields []*Field
}

// NewDatabaseTable starts building a new table.
func NewDatabaseTable() *TableBuilder {
	return &TableBuilder{}
}

// WithFields specifies the fields in the table.
func (t *TableBuilder) WithFields(fields ...*Field) *TableBuilder {
	t.fields = fields
	return t
}

// Build returns the built table.
func (t *TableBuilder) Build() *DatabaseTable {
	fieldByFieldPath := make(map[string]*Field)
	for _, c := range t.fields {
		if _, ok := fieldByFieldPath[c.fieldPath.String()]; ok {
			panic("multiple fields with the same field path: " + c.fieldPath.String())
		}
		fieldByFieldPath[c.fieldPath.String()] = c
	}

	return &DatabaseTable{
		fields:           t.fields,
		fieldByFieldPath: fieldByFieldPath,
	}
}
