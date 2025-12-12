// Copyright 2022 The LUCI Authors.
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
	"strings"

	"go.chromium.org/luci/common/data/aip132"
)

// MergeWithDefaultOrder merges the specified order with the given
// defaultOrder. The merge occurs as follows:
//   - Ordering specified in `order` takes precedence.
//   - For columns not specified in the `order` that appear in `defaultOrder`,
//     ordering is applied in the order they apply in defaultOrder.
func MergeWithDefaultOrder(defaultOrder []aip132.OrderBy, order []aip132.OrderBy) []aip132.OrderBy {
	result := make([]aip132.OrderBy, 0, len(order)+len(defaultOrder))
	seenColumns := make(map[string]struct{})
	for _, o := range order {
		result = append(result, o)
		seenColumns[o.FieldPath.String()] = struct{}{}
	}
	for _, o := range defaultOrder {
		if _, ok := seenColumns[o.FieldPath.String()]; !ok {
			result = append(result, o)
		}
	}
	return result
}

// OrderByClause returns a Standard SQL Order by clause, including
// "ORDER BY" and trailing new line (if an order is specified).
// If no order is specified, returns "".
//
// The returned order clause is safe against SQL injection; only
// strings appearing from Table appear in the output.
func (t *DatabaseTable) OrderByClause(order []aip132.OrderBy) (string, error) {
	if len(order) == 0 {
		return "", nil
	}
	seenColumns := make(map[string]struct{})
	var result strings.Builder
	result.WriteString("ORDER BY ")
	for i, o := range order {
		if i > 0 {
			result.WriteString(", ")
		}
		column, err := t.SortableFieldByFieldPath(o.FieldPath)
		if err != nil {
			return "", err
		}
		if _, ok := seenColumns[o.FieldPath.String()]; ok {
			return "", fmt.Errorf("field appears in order_by multiple times: %q", o.FieldPath.String())
		}
		seenColumns[o.FieldPath.String()] = struct{}{}
		orderBy, err := column.backend.OrderBy(o.Descending)
		if err != nil {
			return "", fmt.Errorf("field %q: %w", o.FieldPath.String(), err)
		}
		result.WriteString(orderBy)
	}
	result.WriteString("\n")
	return result.String(), nil
}
