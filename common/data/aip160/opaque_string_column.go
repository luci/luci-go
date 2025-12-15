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

import "fmt"

// OpaqueStringColumn represents a string field which is implemented by a STRING database column,
// but values are modified/encoded in some way prior to being stored.
// This type of column supports only the exact match filter operators.
type OpaqueStringColumn struct {
	SimpleColumn
	// encodeFunction converts a user-visible value to its database representation.
	encodeFunction func(value string) string
}

// RestrictionQuery implements FieldBackend.
func (s *OpaqueStringColumn) RestrictionQuery(restriction RestrictionContext, g Generator) (string, error) {
	if len(restriction.NestedFields) > 0 {
		return "", FieldsUnsupportedError(restriction.FieldPath)
	}
	argValueUnsafe, err := CoerceArgToStringConstant(restriction.Arg)
	if err != nil {
		return "", fmt.Errorf("argument for field %q: %w", restriction.FieldPath.String(), err)
	}
	dbValueUnsafe := s.encodeFunction(argValueUnsafe)
	switch restriction.Comparator {
	case "=":
		return fmt.Sprintf("(%s = %s)", g.ColumnReference(s.databaseName), g.BindString(dbValueUnsafe)), nil
	case "!=":
		return fmt.Sprintf("(%s <> %s)", g.ColumnReference(s.databaseName), g.BindString(dbValueUnsafe)), nil
	default:
		return "", OperatorNotImplementedError(restriction.Comparator, restriction.FieldPath, "OPAQUE STRING")
	}
}

// ImplicitRestrictionQuery implements FieldBackend.
func (s *OpaqueStringColumn) ImplicitRestrictionQuery(ir ImplicitRestrictionContext, g Generator) (string, error) {
	return "", ImplicitRestrictionUnsupportedError()
}

// OrderBy implements FieldBackend.
func (s *OpaqueStringColumn) OrderBy(desc bool) (string, error) {
	// Ordering on opaque string columns is not supported, as it gives the database order and not
	// the logical field order.
	// We could support this if the user specified a SQL function to decode the column value.
	return "", fmt.Errorf("OrderBy not supported for opaque string columns")
}

type OpaqueStringColumnBuilder struct {
	column *OpaqueStringColumn
}

// NewOpaqueStringColumn returns a new builder for constructing an OpaqueStringColumn.
// databaseName MUST be a constant; it may not come from user input.
func NewOpaqueStringColumn(databaseName string) *OpaqueStringColumnBuilder {
	return &OpaqueStringColumnBuilder{
		column: &OpaqueStringColumn{
			SimpleColumn: SimpleColumn{
				databaseName: databaseName,
			},
		},
	}
}

// WithEncodeFunction sets the encode function for the column.
func (b *OpaqueStringColumnBuilder) WithEncodeFunction(encodeFunction func(value string) string) *OpaqueStringColumnBuilder {
	b.column.encodeFunction = encodeFunction
	return b
}

// Build returns the built column.
func (b *OpaqueStringColumnBuilder) Build() *OpaqueStringColumn {
	return b.column
}
