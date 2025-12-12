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

// SimpleColumn represents a field implemented by one database column.
// As this column is not aware of its database type, it has no built-in filtering support.
// It assumes there will be an ordering provided by the database.
type SimpleColumn struct {
	// The database name of the column.
	// Important: Only assign assign safe constants to this field.
	// User input MUST NOT flow to this field, as it will be used directly
	// in SQL statements and would allow the user to perform SQL injection
	// attacks.
	databaseName string
}

// RestrictionQuery implements FieldBackend.
func (s *SimpleColumn) RestrictionQuery(restriction RestrictionContext, g Generator) (string, error) {
	return "", fmt.Errorf("simple columns do not support filtering")
}

// ImplicitRestrictionQuery implements FieldBackend.
func (s *SimpleColumn) ImplicitRestrictionQuery(ir ImplicitRestrictionContext, g Generator) (string, error) {
	return "", fmt.Errorf("simple columns do not support filtering")
}

// OrderBy implements FieldBackend.
func (s *SimpleColumn) OrderBy(desc bool) (string, error) {
	if desc {
		return fmt.Sprintf("%s DESC", s.databaseName), nil
	}
	return fmt.Sprintf("%s", s.databaseName), nil
}

// NewSimpleColumn returns a new SimpleColumn.
// databaseName MUST be a constant; it may not come from user input.
func NewSimpleColumn(databaseName string) *SimpleColumn {
	return &SimpleColumn{
		databaseName: databaseName,
	}
}
