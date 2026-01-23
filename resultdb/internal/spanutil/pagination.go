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
	AfterValue any
	// Whether this column is already at its maximum value, and there
	// is no point looking for values which are greater in respect of
	// this column. In some cases, this hint can be leveraged by the
	// database to improve the query plan.
	//
	// It is always safe to leave this value unset, but setting it
	// (correctly) can improve performance.
	AtLimit bool
}

// WhereAfterClause generates a WHERE clause that filters for rows *after*
// some composite pagination key (A, B, C, ... ). This is normally used
// in pagination queries to fetch rows after a certain pagination key.
//
// E.g. if paramPrefix is "after", it generates SQL like:
// (A > @afterA)
// OR (A = @afterA AND B > @afterB)
// OR (A = @afterA AND B = @afterB AND C > @afterC)
func WhereAfterClause(pageToken []PageTokenElement, paramPrefix string, params map[string]any) string {
	atLimitPrefix, remainingColumns := splitAtLimitPrefix(pageToken)
	if len(remainingColumns) == 0 {
		// The page token is the ID of the last row we saw. If all columns are at their limit,
		// there are no more rows.
		return "(FALSE)"
	}

	// We can factor out the prefix of columns that are at their limit.
	// This can help the database to improve the query plan.
	var result strings.Builder
	if len(atLimitPrefix) > 0 {
		result.WriteString("(")
	}
	for _, col := range atLimitPrefix {
		result.WriteString(col.ColumnName)
		result.WriteString(" = @")
		result.WriteString(paramPrefix)
		result.WriteString(col.ColumnName)
		result.WriteString(" AND ")
	}
	result.WriteString("(")
	for i, afterColumn := range remainingColumns {
		if i > 0 {
			result.WriteString("\n OR ")
		}
		result.WriteString("(")
		keepEqualColumns := remainingColumns[:i]
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
	if len(atLimitPrefix) > 0 {
		result.WriteString(")")
	}
	// Bind parameters.
	for _, col := range pageToken {
		params[paramPrefix+col.ColumnName] = col.AfterValue
	}
	return result.String()
}

func splitAtLimitPrefix(pageToken []PageTokenElement) (atLimitColumns []PageTokenElement, remainingColumns []PageTokenElement) {
	for i, afterColumn := range pageToken {
		if !afterColumn.AtLimit {
			return pageToken[:i], pageToken[i:]
		}
	}
	// All columns are at their limits.
	return pageToken, nil
}
