// Copyright 2026 The LUCI Authors.
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

package spanutil

import "strings"

// PageTokenElement represents an element of a pagination token.
type PageTokenElement struct {
	// ColumnName in the database.
	ColumnName string
	// AfterValue is the value of the column after which to filter.
	AfterValue string
}

// WhereAfterClause generates a WHERE clause that filters for rows after
// some composite pagination key (A, B, C, ... ). This is normally used
// in pagination queries to fetch rows after a certain pagination key.
//
// E.g. if paramPrefix is "after":
// (A > @afterA)
// OR (A = @afterA AND B > @afterB)
// OR (A = @afterA AND B = @afterB AND C > @afterC)
func WhereAfterClause(pageToken []PageTokenElement, paramPrefix string, params map[string]any) string {
	var result strings.Builder
	result.WriteString("(")
	for i, afterColumn := range pageToken {
		if i > 0 {
			result.WriteString("\n OR ")
		}
		result.WriteString("(")
		keepEqualColumns := pageToken[:i]
		for _, keepEqualColumn := range keepEqualColumns {
			// E.g. "ModuleName = @afterModuleName AND "
			result.WriteString(keepEqualColumn.ColumnName)
			result.WriteString(" = @")
			result.WriteString(paramPrefix)
			result.WriteString(keepEqualColumn.ColumnName)
			result.WriteString(" AND ")
		}
		// E.g. "ModuleScheme > @afterModuleScheme"
		result.WriteString(afterColumn.ColumnName)
		result.WriteString(" > @")
		result.WriteString(paramPrefix)
		result.WriteString(afterColumn.ColumnName)
		result.WriteString(")")
	}
	result.WriteString(")")

	// Bind parameters.
	for _, col := range pageToken {
		params[paramPrefix+col.ColumnName] = col.AfterValue
	}
	return result.String()
}
