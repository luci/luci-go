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

// IntegerColumn implements FieldBackend for fields that represent proto enums.
// The underlying database column is assumed to be of type INT64.
type IntegerColumn struct {
	SimpleColumn
}

// RestrictionQuery implements FieldBackend.
func (e *IntegerColumn) RestrictionQuery(restriction RestrictionContext, g Generator) (string, error) {
	if len(restriction.NestedFields) > 0 {
		return "", FieldsUnsupportedError(restriction.FieldPath)
	}
	argValue, err := CoarceArgToIntegerConstant(restriction.Arg)
	if err != nil {
		return "", fmt.Errorf("argument for field %q: %w", restriction.FieldPath.String(), err)
	}

	// Integers are SQL injection safe.
	value := strconv.FormatInt(int64(argValue), 10)
	switch restriction.Comparator {
	case "=", ">", "<", ">=", "<=":
		return fmt.Sprintf("(%s %s %s)", g.ColumnReference(e.databaseName), restriction.Comparator, value), nil
	case "!=":
		// Prefer to use the ISO SQL not-equal syntax.
		return fmt.Sprintf("(%s <> %s)", g.ColumnReference(e.databaseName), value), nil
	default:
		return "", OperatorNotImplementedError(restriction.Comparator, restriction.FieldPath, "INTEGER")
	}
}

// ImplicitRestrictionQuery implements FieldBackend.
func (e *IntegerColumn) ImplicitRestrictionQuery(ir ImplicitRestrictionContext, g Generator) (string, error) {
	return "", ImplicitRestrictionUnsupportedError()
}

// NewIntegerColumn returns a new builder for constructing an IntegerColumn.
// databaseName MUST be a constant; it may not come from user input.
func NewIntegerColumn(databaseName string) *IntegerColumn {
	return &IntegerColumn{
		SimpleColumn: SimpleColumn{
			databaseName: databaseName,
		},
	}
}
