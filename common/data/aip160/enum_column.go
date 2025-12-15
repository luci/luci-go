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
	"strconv"
)

// EnumColumn implements FieldBackend for fields that represent proto enums.
// The underlying database column is assumed to be of type INT64.
type EnumColumn struct {
	SimpleColumn

	enumDef *EnumDefinition
}

// RestrictionQuery implements FieldBackend.
func (e *EnumColumn) RestrictionQuery(restriction RestrictionContext, g Generator) (string, error) {
	if len(restriction.NestedFields) > 0 {
		return "", FieldsUnsupportedError(restriction.FieldPath)
	}
	argValue, err := CoarceArgToEnumConstant(restriction.Arg, e.enumDef)
	if err != nil {
		return "", fmt.Errorf("argument for field %q: %w", restriction.FieldPath.String(), err)
	}

	// Integers are SQL injection safe.
	value := strconv.FormatInt(int64(argValue), 10)
	switch restriction.Comparator {
	case "=":
		return fmt.Sprintf("(%s = %s)", g.ColumnReference(e.databaseName), value), nil
	case "!=":
		return fmt.Sprintf("(%s <> %s)", g.ColumnReference(e.databaseName), value), nil
	default:
		return "", OperatorNotImplementedError(restriction.Comparator, restriction.FieldPath, e.enumDef.typeName)
	}
}

// ImplicitRestrictionQuery implements FieldBackend.
func (e *EnumColumn) ImplicitRestrictionQuery(ir ImplicitRestrictionContext, g Generator) (string, error) {
	return "", ImplicitRestrictionUnsupportedError()
}

// EnumColumnBuilder implements a builder pattern for EnumColumn.
type EnumColumnBuilder struct {
	column *EnumColumn
}

// NewEnumColumn returns a new builder for constructing an EnumColumn.
// databaseName MUST be a constant; it may not come from user input.
func NewEnumColumn(databaseName string) *EnumColumnBuilder {
	return &EnumColumnBuilder{
		column: &EnumColumn{
			SimpleColumn: SimpleColumn{
				databaseName: databaseName,
			},
		},
	}
}

// WithDefinition sets the enum definition.
func (b *EnumColumnBuilder) WithDefinition(def *EnumDefinition) *EnumColumnBuilder {
	b.column.enumDef = def
	return b
}

// Build returns the built EnumColumn.
func (b *EnumColumnBuilder) Build() *EnumColumn {
	return b.column
}
