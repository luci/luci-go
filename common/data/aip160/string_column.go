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
)

// StringColumn implements FieldBackend for string fields stored as a STRING database column.
type StringColumn struct {
	SimpleColumn
}

// RestrictionQuery implements FieldBackend.
func (s *StringColumn) RestrictionQuery(restriction RestrictionContext, g Generator) (string, error) {
	if len(restriction.NestedFields) > 0 {
		return "", FieldsUnsupportedError(restriction.FieldPath)
	}
	argValueUnsafe, err := CoarceArgToConstant(restriction.Arg)
	if err != nil {
		return "", fmt.Errorf("argument for field %q: %w", restriction.FieldPath.String(), err)
	}
	switch restriction.Comparator {
	case "=":
		return fmt.Sprintf("(%s = %s)", g.ColumnReference(s.databaseName), g.BindString(argValueUnsafe)), nil
	case "!=":
		return fmt.Sprintf("(%s <> %s)", g.ColumnReference(s.databaseName), g.BindString(argValueUnsafe)), nil
	case ":":
		return fmt.Sprintf("(%s LIKE %s)", g.ColumnReference(s.databaseName), g.BindString("%"+quoteLike(argValueUnsafe)+"%")), nil
	default:
		return "", OperatorNotImplementedError(restriction.Comparator, restriction.FieldPath, "STRING")
	}
}

// ImplicitRestrictionQuery implements FieldBackend.
func (s *StringColumn) ImplicitRestrictionQuery(ir ImplicitRestrictionContext, g Generator) (string, error) {
	return fmt.Sprintf("(%s LIKE %s)", g.ColumnReference(s.databaseName), g.BindString("%"+quoteLike(ir.ArgValueUnsafe)+"%")), nil
}

// NewStringColumn returns a new StringColumn.
// databaseName MUST be a constant; it may not come from user input.
func NewStringColumn(databaseName string) *StringColumn {
	return &StringColumn{
		SimpleColumn: SimpleColumn{
			databaseName: databaseName,
		},
	}
}
