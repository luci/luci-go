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

// RepeatedStringColumn implements FieldBackend for repeated string fields which are stored
// in the database as ARRAY<STRING>.
type RepeatedStringColumn struct {
	SimpleColumn
}

// RestrictionQuery implements FieldBackend.
func (r *RepeatedStringColumn) RestrictionQuery(restriction RestrictionContext, g Generator) (string, error) {
	if len(restriction.NestedFields) > 0 {
		return "", FieldsUnsupportedError(restriction.FieldPath)
	}
	argValueUnsafe, err := CoerceArgToStringConstant(restriction.Arg)
	if err != nil {
		return "", fmt.Errorf("argument for field %q: %w", restriction.FieldPath.String(), err)
	}

	if restriction.Comparator == ":" {
		return fmt.Sprintf("(EXISTS (SELECT value FROM UNNEST(%s) as value WHERE value LIKE %s))",
			g.ColumnReference(r.databaseName), g.BindString("%"+quoteLike(argValueUnsafe)+"%")), nil
	} else {
		return "", OperatorNotImplementedError(restriction.Comparator, restriction.FieldPath, "REPEATED STRING")
	}
}

// ImplicitRestrictionQuery implements FieldBackend.
func (r *RepeatedStringColumn) ImplicitRestrictionQuery(ir ImplicitRestrictionContext, g Generator) (string, error) {
	return "", ImplicitRestrictionUnsupportedError()
}

// OrderBy implements FieldBackend.
func (r *RepeatedStringColumn) OrderBy(desc bool) (string, error) {
	return "", fmt.Errorf("OrderBy not supported for repeated string columns")
}

// NewRepeatedStringColumn returns a new RepeatedStringColumn.
// databaseName MUST be a constant; it may not come from user input.
func NewRepeatedStringColumn(databaseName string) *RepeatedStringColumn {
	return &RepeatedStringColumn{
		SimpleColumn: SimpleColumn{
			databaseName: databaseName,
		},
	}
}
