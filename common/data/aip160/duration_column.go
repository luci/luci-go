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

// DurationColumn implements FieldBackend for fields that represent a duration.
// The underlying database column is assumed to be of type INT64 (storing nanoseconds).
type DurationColumn struct {
	SimpleColumn
}

// RestrictionQuery implements FieldBackend.
func (d *DurationColumn) RestrictionQuery(restriction RestrictionContext, g Generator) (string, error) {
	if len(restriction.NestedFields) > 0 {
		return "", FieldsUnsupportedError(restriction.FieldPath)
	}
	duration, err := CoerceArgToDurationConstant(restriction.Arg)
	if err != nil {
		return "", fmt.Errorf("argument for field %q: %w", restriction.FieldPath.String(), err)
	}

	// Database representation is an INT64 storing nanoseconds.
	// Integers are SQL injection safe.
	value := strconv.FormatInt(duration.Nanoseconds(), 10)
	switch restriction.Comparator {
	case "=", "!=", "<", ">", "<=", ">=":
		return fmt.Sprintf("(%s %s %s)", g.ColumnReference(d.databaseName), restriction.Comparator, value), nil
	default:
		return "", OperatorNotImplementedError(restriction.Comparator, restriction.FieldPath, "DURATION")
	}
}

// ImplicitRestrictionQuery implements FieldBackend.
func (d *DurationColumn) ImplicitRestrictionQuery(ir ImplicitRestrictionContext, g Generator) (string, error) {
	return "", ImplicitRestrictionUnsupportedError()
}

// DurationColumnBuilder implements a builder pattern for DurationColumn.
type DurationColumnBuilder struct {
	column *DurationColumn
}

// NewDurationColumn returns a new builder for constructing a DurationColumn.
// databaseName MUST be a constant; it may not come from user input.
func NewDurationColumn(databaseName string) *DurationColumnBuilder {
	return &DurationColumnBuilder{
		column: &DurationColumn{
			SimpleColumn: SimpleColumn{
				databaseName: databaseName,
			},
		},
	}
}

// Build returns the built DurationColumn.
func (b *DurationColumnBuilder) Build() *DurationColumn {
	return b.column
}
