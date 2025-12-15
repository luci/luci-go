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

// KeyValueColumn implements FieldBackend for key-value fields.
type KeyValueColumn struct {
	SimpleColumn

	// If the underlying database type is a STRING ARRAY, where each element is "key:value".
	IsUsingStringArray bool

	// If the underlying database type is a repeated struct<key STRING, value STRING>.
	IsUsingRepeatedStruct bool
}

// RestrictionQuery implements FieldBackend.
func (k *KeyValueColumn) RestrictionQuery(restriction RestrictionContext, g Generator) (string, error) {
	if len(restriction.NestedFields) == 0 {
		// TODO: AIP-160 specifies the has operator on maps will check for the presence of a key.
		return "", fmt.Errorf("key value columns must specify the key to search on.  Instead of '%s%s' try '%s.key%s'", restriction.FieldPath.String(), restriction.Comparator, restriction.FieldPath.String(), restriction.Comparator)
	}

	if len(restriction.NestedFields) > 1 {
		return "", fmt.Errorf("expected only a single '.' after key-value column named %q", restriction.FieldPath.String())
	}
	columnDatabaseName := g.ColumnReference(k.databaseName)
	keyUnsafe := restriction.NestedFields[0]
	// argValueUnsafe is user provided input and can only be used in bind parameters, never in the raw SQL string.
	argValueUnsafe, err := CoerceArgToStringConstant(restriction.Arg)
	if err != nil {
		segments := append([]string{}, restriction.FieldPath.GetSegments()...)
		segments = append(segments, keyUnsafe)
		return "", fmt.Errorf("argument for field %q: %w", aip132.NewFieldPath(segments...).String(), err)
	}

	if k.IsUsingStringArray {
		if restriction.Comparator == ":" {
			boundVal := g.BindString(keyUnsafe + ":" + "%" + quoteLike(argValueUnsafe) + "%")
			return fmt.Sprintf("(EXISTS (SELECT 1 FROM UNNEST(%s) as _v WHERE _v LIKE %s))", columnDatabaseName, boundVal), nil
		}

		boundVal := g.BindString(keyUnsafe + ":" + argValueUnsafe)
		switch restriction.Comparator {
		case "=":
			return fmt.Sprintf("(%s IN UNNEST(%s))", boundVal, columnDatabaseName), nil
		case "!=":
			boundKey := g.BindString(keyUnsafe + ":")
			return fmt.Sprintf("(EXISTS (SELECT 1 FROM UNNEST(%s) as _v WHERE STARTS_WITH(_v, %s) AND _v <> %s))", columnDatabaseName, boundKey, boundVal), nil
		default:
			return "", OperatorNotImplementedError(restriction.Comparator, restriction.FieldPath, "KEY-VALUE")
		}
	} else if k.IsUsingRepeatedStruct {
		key := g.BindString(keyUnsafe)
		if restriction.Comparator == ":" {
			boundVal := g.BindString("%" + quoteLike(argValueUnsafe) + "%")
			return fmt.Sprintf("(EXISTS (SELECT _v.key, _v.value FROM UNNEST(%s) as _v WHERE _v.key = %s AND _v.value LIKE %s))", columnDatabaseName, key, boundVal), nil
		}
		boundVal := g.BindString(argValueUnsafe)
		switch restriction.Comparator {
		case "=":
			return fmt.Sprintf("(EXISTS (SELECT _v.key, _v.value FROM UNNEST(%s) as _v WHERE _v.key = %s AND _v.value = %s))", columnDatabaseName, key, boundVal), nil
		case "!=":
			return fmt.Sprintf("(EXISTS (SELECT _v.key, _v.value FROM UNNEST(%s) as _v WHERE _v.key = %s AND _v.value <> %s))", columnDatabaseName, key, boundVal), nil
		default:
			return "", OperatorNotImplementedError(restriction.Comparator, restriction.FieldPath, "KEY-VALUE")
		}
	} else {
		panic("logic error: KeyValueColumn has an invalid configuration")
	}
}

// ImplicitRestrictionQuery implements FieldBackend.
func (k *KeyValueColumn) ImplicitRestrictionQuery(ir ImplicitRestrictionContext, g Generator) (string, error) {
	return "", ImplicitRestrictionUnsupportedError()
}

// OrderBy implements FieldBackend.
func (k *KeyValueColumn) OrderBy(desc bool) (string, error) {
	return "", fmt.Errorf("OrderBy not supported for key-value columns")
}

// KeyValueColumnBuilder implements a builder pattern for KeyValueColumn.
type KeyValueColumnBuilder struct {
	column *KeyValueColumn
}

// NewKeyValueColumn returns a new builder for constructing a KeyValueColumn.
// databaseName MUST be a constant; it may not come from user input.
func NewKeyValueColumn(databaseName string) *KeyValueColumnBuilder {
	return &KeyValueColumnBuilder{
		column: &KeyValueColumn{
			SimpleColumn: SimpleColumn{
				databaseName: databaseName,
			},
		},
	}
}

// WithStringArray sets that the underlying database type is an ARRAY<STRING>,
// where each key-value pair is encoded as `<key>:<value>`.
func (b *KeyValueColumnBuilder) WithStringArray() *KeyValueColumnBuilder {
	b.column.IsUsingStringArray = true
	return b
}

// WithRepeatedStruct sets that the underlying database type is
// ARRAY<STRUCT<key string, value string>>.
func (b *KeyValueColumnBuilder) WithRepeatedStruct() *KeyValueColumnBuilder {
	b.column.IsUsingRepeatedStruct = true
	return b
}

// Build returns the built KeyValueColumn.
func (b *KeyValueColumnBuilder) Build() *KeyValueColumn {
	if b.column.IsUsingRepeatedStruct == b.column.IsUsingStringArray {
		panic("exactly one of IsUnderlyingRepeatedStruct and IsUnderlyingStringArray must be set")
	}
	return b.column
}
