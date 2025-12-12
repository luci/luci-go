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

// Field represents the schema of a field that may be used in an AIP-160 filter
// or AIP-132 order by clause.
type Field struct {
	// The externally-visible field path of this field.
	// This is the path that may be referenced in AIP-160 filters and AIP-132 order by clauses.
	fieldPath aip132.FieldPath

	// Whether this field can be sorted on.
	sortable bool

	// Whether this field can be filtered on.
	filterable bool

	// ImplicitFilter controls whether this field is searched implicitly
	// in AIP-160 filter expressions.
	implicitFilter bool

	// Backend implements filtering/sorting support for this field in the database.
	backend FieldBackend
}

// DatabaseTable represents the schema of a database table, view or query.
type DatabaseTable struct {
	// The fields in the database table.
	fields []*Field

	// A mapping from externally-visible field path to the field
	// definition. The field name used as a key is in lowercase.
	fieldByFieldPath map[string]*Field
}

// FilterableFieldByFieldPath returns the filterable field
// with the given field path.
func (t *DatabaseTable) FilterableFieldByFieldPath(path aip132.FieldPath) (*Field, error) {
	col := t.fieldByFieldPath[path.String()]
	if col != nil && col.filterable {
		return col, nil
	}

	fieldNames := []string{}
	for _, column := range t.fields {
		if column.filterable {
			fieldNames = append(fieldNames, column.fieldPath.String())
		}
	}
	return nil, fmt.Errorf("no filterable field %q, valid fields are %s", path.String(), strings.Join(fieldNames, ", "))
}

// SortableFieldByFieldPath returns the sortable field
// with the given externally-visible field path.
func (t *DatabaseTable) SortableFieldByFieldPath(path aip132.FieldPath) (*Field, error) {
	col := t.fieldByFieldPath[path.String()]
	if col != nil && col.sortable {
		return col, nil
	}

	fieldNames := []string{}
	for _, column := range t.fields {
		if column.sortable {
			fieldNames = append(fieldNames, column.fieldPath.String())
		}
	}
	return nil, fmt.Errorf("no sortable field named %q, valid fields are %s", path.String(), strings.Join(fieldNames, ", "))
}
