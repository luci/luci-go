// Copyright 2025 The LUCI Authors.
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
	"fmt"

	"go.chromium.org/luci/common/data/aip132"
)

// FieldBackend generates SQL to filter and sort on a logical field (which can
// be used in an AIP-160 filter clause like `myfield="blah"`), using knowledge about
// its database representation.
//
// While many simple columns are implemented by the same database type as their field
// (e.g. STRING implemented as STRING), for others the external and internal representations
// look different. For example, the test identifier might be exposed as a string but
// stored as many components in the database.
//
// Each backend can choose which AIP-160/AIP-132 features it supports, i.e. which
// filter operators it supports, and if it supports being used in an ORDER BY clause or not.
type FieldBackend interface {
	// RestrictionQuery generates SQL for the given AIP-160 Restriction node acting
	// on the column.
	// The returned SQL should be wrapped in brackets to avoid
	// misinterpretation when included in other SQL expressions, e.g. "(condition)".
	RestrictionQuery(restriction RestrictionContext, g Generator) (string, error)
	// ImplicitRestrictionQuery generates SQL for matching the column to the given argument
	// value. This is used in contexts where neither the column name nor comparator are explicitly
	// specified (i.e. a raw value in the filter).
	// The returned SQL should be wrapped in brackets to avoid
	// misinterpretation when included in other SQL expressions, e.g. "(condition)".
	ImplicitRestrictionQuery(ir ImplicitRestrictionContext, g Generator) (string, error)
	// Generates SQL for ordering by the field.
	// This should be a list of one or more SQL column expressions followed by "ASC" or "DESC".
	// E.g. "Column1 ASC, IF(Column2='blah', 1, 0) DESC".
	OrderBy(desc bool) (string, error)
}

// RestrictionContext provides details of a restriction on a field.
// E.g. `my_field="blah"`
type RestrictionContext struct {
	// The path to the current field.
	FieldPath aip132.FieldPath
	// The field path within this field the filter is attempting to access.
	NestedFields []string
	// The comparator operator, e.g. "=", ":", "!=".
	Comparator string
	// The argument to the restriction.
	Arg *Arg
}

// ImplicitRestrictionContext provides details of an implicit restriction on a field.
// Implicit restrictions are restrictions that don't directly reference a field,
// e.g. `"blah"`
type ImplicitRestrictionContext struct {
	// The raw string value from the filter, it should not be used directly in the
	// generated SQL.
	ArgValueUnsafe string
}

// Generator provides methods to support SQL query generation methods.
type Generator interface {
	// BindString binds a new query parameter with the given string value, and returns
	// the name of the parameter (including '@').
	// The returned string is an injection-safe SQL expression.
	BindString(value string) string
	// ColumnReference returns the fully-qualified name of a column in the database,
	// appending the table alias prefix (if any).
	ColumnReference(columnName string) string
}

// FieldsUnsupportedError is returned when a field does not support sub-fields.
func FieldsUnsupportedError(fieldPath aip132.FieldPath) error {
	return fmt.Errorf("fields are only supported for key-value columns.  Try removing the '.' from after your field named %q", fieldPath.String())
}

// OperatorNotImplementedError is returned when a filter operator is not supported for a field.
func OperatorNotImplementedError(operator string, fieldPath aip132.FieldPath, fieldType string) error {
	return fmt.Errorf("operator %q not implemented for field %q of type %s", operator, fieldPath.String(), fieldType)
}

// ImplicitRestrictionUnsupportedError is returned when an implicit restriction is not supported for a field.
func ImplicitRestrictionUnsupportedError() error {
	return fmt.Errorf("implicit restrictions not supported for this column type")
}
