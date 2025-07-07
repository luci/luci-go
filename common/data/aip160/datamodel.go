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

// Package aip contains utilities used to comply with API Improvement
// Proposals (AIPs) from https://google.aip.dev/. This includes
// an AIP-160 filter parser and SQL generator and AIP-132 order by
// clause parser and SQL generator.
package aip160

import (
	"fmt"
	"strings"

	"go.chromium.org/luci/common/data/aip132"
)

const (
	// SqlColumnTypeString is a column of type string.
	SqlColumnTypeString SqlColumnType = iota
	// SqlColumnTypeBool is a column of type boolean.  NULL values are mapped to FALSE.
	SqlColumnTypeBool = iota
)

// SqlColumnType is an enum for the type of a column.  Valid values are in the const block above.
type SqlColumnType int32

func (t SqlColumnType) String() string {
	switch t {
	case SqlColumnTypeString:
		return "STRING"
	case SqlColumnTypeBool:
		return "BOOL"
	default:
		return "UNKNOWN"
	}
}

// SqlColumn represents the schema of a Database column.
type SqlColumn struct {
	// The externally-visible field path this column maps to.
	// This path may be referenced in AIP-160 filters and AIP-132 order by clauses.
	fieldPath aip132.FieldPath

	// The database name of the column.
	// Important: Only assign assign safe constants to this field.
	// User input MUST NOT flow to this field, as it will be used directly
	// in SQL statements and would allow the user to perform SQL injection
	// attacks.
	databaseName string

	// Whether this column can be sorted on.
	sortable bool

	// Whether this column can be filtered on.
	filterable bool

	// ImplicitFilter controls whether this field is searched implicitly
	// in AIP-160 filter expressions.
	implicitFilter bool

	// Whether this column is an array of structs with two string members: key and value.
	// Note that repeated keys are not supported and may lead to undefined filtering behaviour if present.
	// If you use this option, columnType represents the type of the values associated with each key and must be ColumnType_STRING.
	keyValue bool

	// Whether this column is an array of "key:value" strings.
	// Note that repeated keys are not supported and may lead to undefined filtering behaviour if present.
	// If you use this option, columnType represents the type of the values associated with each key and must be ColumnType_STRING.
	stringArrayKeyValue bool

	// Whether this column is an array.
	array bool

	// The type of the column, defaults to ColumnType_STRING.
	columnType SqlColumnType

	// The function which is applied to the filter arguments.
	argSubstitute func(sub string) string
}

// SqlTable represents the schema of a Database table, view or query.
type SqlTable struct {
	// The columns in the database table.
	columns []*SqlColumn

	// A mapping from externally-visible field path to the column
	// definition. The column name used as a key is in lowercase.
	columnByFieldPath map[string]*SqlColumn
}

// FilterableColumnByFieldPath returns the database name of the filterable column
// with the given field path.
func (t *SqlTable) FilterableColumnByFieldPath(path aip132.FieldPath) (*SqlColumn, error) {
	col := t.columnByFieldPath[path.String()]
	if col != nil && col.filterable {
		return col, nil
	}

	columnNames := []string{}
	for _, column := range t.columns {
		if column.filterable {
			columnNames = append(columnNames, column.fieldPath.String())
		}
	}
	return nil, fmt.Errorf("no filterable field %q, valid fields are %s", path.String(), strings.Join(columnNames, ", "))
}

// SortableColumnByFieldPath returns the sortable database column
// with the given externally-visible field path.
func (t *SqlTable) SortableColumnByFieldPath(path aip132.FieldPath) (*SqlColumn, error) {
	col := t.columnByFieldPath[path.String()]
	if col != nil && col.sortable {
		return col, nil
	}

	columnNames := []string{}
	for _, column := range t.columns {
		if column.sortable {
			columnNames = append(columnNames, column.fieldPath.String())
		}
	}
	return nil, fmt.Errorf("no sortable field named %q, valid fields are %s", path.String(), strings.Join(columnNames, ", "))
}
