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

type SqlColumnBuilder struct {
	column SqlColumn
}

// NewSqlColumn starts building a new column.
func NewSqlColumn() *SqlColumnBuilder {
	return &SqlColumnBuilder{SqlColumn{columnType: SqlColumnTypeString}}
}

// WithFieldPath specifies the field path the column maps to
// in the returned resource. Field paths are described in AIP-161.
//
// For convenience, the field path is described here as a set of
// segments where each segment is joined by the traversal operator (.).
// E.g. the field path "metrics.`some-metric`.value" would be specified
// as ["metrics", "some-metric", "value"].
func (c *SqlColumnBuilder) WithFieldPath(segments ...string) *SqlColumnBuilder {
	c.column.fieldPath = aip132.NewFieldPath(segments...)
	return c
}

// WithDatabaseName specifies the database name of the column.
// Important: Only pass safe values (e.g. compile-time constants) to this
// field.
// User input MUST NOT flow to this field, as it will be used directly
// in SQL statements and would allow the user to perform SQL injection
// attacks.
func (c *SqlColumnBuilder) WithDatabaseName(name string) *SqlColumnBuilder {
	c.column.databaseName = name
	return c
}

// KeyValue specifies this column is an array of structs with two string members: key and value.
// The key is exposed as a field on the column name, the value can be queried with :, = and !=
// Example query: tag.key=value
// Note that repeated keys are not supported and may lead to undefined filtering behaviour if present.
// If you use this option, columnType represents the type of the values associated with each key and must be ColumnType_STRING.
func (c *SqlColumnBuilder) KeyValue() *SqlColumnBuilder {
	c.column.keyValue = true
	return c
}

// StringArrayKeyValue specifies this column is an array of "key:value" strings.
// The key is exposed as a field on the column name, the value can be queried with :, = and !=
// Example query: variant.os=Mac-13
// Note that repeated keys are not supported and may lead to undefined filtering behaviour if present.
// If you use this option, columnType represents the type of the values associated with each key and must be ColumnType_STRING.
func (c *SqlColumnBuilder) StringArrayKeyValue() *SqlColumnBuilder {
	c.column.stringArrayKeyValue = true
	return c
}

// Array specifies this column is an array.
// The value can be queried with ':'.  The operator matches if any element of the array matches.
// Example query: column:value
func (c *SqlColumnBuilder) Array() *SqlColumnBuilder {
	c.column.array = true
	return c
}

// Bool specifies this column has bool type in the database.
func (c *SqlColumnBuilder) Bool() *SqlColumnBuilder {
	c.column.columnType = SqlColumnTypeBool
	return c
}

// Sortable specifies this column can be sorted on.
func (c *SqlColumnBuilder) Sortable() *SqlColumnBuilder {
	c.column.sortable = true
	return c
}

// Filterable specifies this column can be filtered on.
func (c *SqlColumnBuilder) Filterable() *SqlColumnBuilder {
	c.column.filterable = true
	return c
}

// FilterableImplicitly specifies this column can be filtered on implicitly.
// This means that AIP-160 filter expressions not referencing any
// particular field will try to search in this column.
func (c *SqlColumnBuilder) FilterableImplicitly() *SqlColumnBuilder {
	c.column.filterable = true
	c.column.implicitFilter = true
	return c
}

// WithArgumentSubstitutor specifies a substitution that should happen to the user-specified
// filter argument before it is matched against the database value. If this option is enabled,
// the filter operators permitted will be limited to = (equals) and != (not equals).
func (c *SqlColumnBuilder) WithArgumentSubstitutor(f func(sub string) string) *SqlColumnBuilder {
	c.column.argSubstitute = f
	return c
}

// Build returns the built column.
func (c *SqlColumnBuilder) Build() *SqlColumn {
	result := &SqlColumn{}
	*result = c.column
	return result
}

type SqlTableBuilder struct {
	columns []*SqlColumn
}

// NewSqlTable starts building a new table.
func NewSqlTable() *SqlTableBuilder {
	return &SqlTableBuilder{}
}

// WithColumns specifies the columns in the table.
func (t *SqlTableBuilder) WithColumns(columns ...*SqlColumn) *SqlTableBuilder {
	t.columns = columns
	return t
}

// Build returns the built table.
func (t *SqlTableBuilder) Build() *SqlTable {
	columnByFieldPath := make(map[string]*SqlColumn)
	for _, c := range t.columns {
		if _, ok := columnByFieldPath[c.fieldPath.String()]; ok {
			panic("multiple columns with the same field path: " + c.fieldPath.String())
		}
		if c.keyValue && c.columnType != SqlColumnTypeString {
			panic("KeyValue columns must be of type string: " + c.fieldPath.String())
		}
		if c.stringArrayKeyValue && c.columnType != SqlColumnTypeString {
			panic("StringArrayKeyValue columns must be of type string: " + c.fieldPath.String())
		}
		columnByFieldPath[c.fieldPath.String()] = c
	}

	return &SqlTable{
		columns:           t.columns,
		columnByFieldPath: columnByFieldPath,
	}
}
