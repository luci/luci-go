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

package testverdictsv2

import (
	"go.chromium.org/luci/common/data/aip132"
	"go.chromium.org/luci/common/errors"
)

// idFieldPath describes the structured test ID of the verdict, which is stored in the test_id_structured field.
var idFieldPath = aip132.NewFieldPath("test_id_structured")

// uiPriorityFieldPath describes the UI priority of the aggregation, which is stored in the ui_priority field.
var uiPriorityFieldPath = aip132.NewFieldPath("ui_priority")

// Ordering represents the sort order of test verdicts.
type Ordering int

const (
	// Verdicts should be sorted by (structured) test identifier.
	// This order is the best for RPC performance as it follows the natural table ordering.
	OrderingByID Ordering = iota
	// Verdicts should be sorted by UI priority.
	// This is means the order will be:
	// - Failed
	// - Execution Error
	// - Precluded
	// - Flaky
	// - Exonerated
	// - Passed and Skipped (treated equivalently).
	OrderingByUIPriority
)

// ParseOrderBy parses the order_by string.
func ParseOrderBy(orderBy string) (Ordering, error) {
	parts, err := aip132.ParseOrderBy(orderBy)
	if err != nil {
		return 0, err
	}
	if len(parts) == 0 {
		// Use default order: order by ID asc.
		return OrderingByID, nil
	}
	var byUIPriority bool
	index := 0
	if parts[index].FieldPath.Equals(uiPriorityFieldPath) {
		if !parts[index].Descending {
			return 0, errors.Fmt("can only sort by %q in descending order", uiPriorityFieldPath.String())
		}
		byUIPriority = true
		index++
	}
	if index < len(parts) && parts[index].FieldPath.Equals(idFieldPath) {
		if parts[index].Descending {
			return 0, errors.Fmt("can only sort by %q in ascending order", idFieldPath.String())
		}
		// The default (and residual) order is always test ID ascending, so no need to set anything.
		index++
	}
	if index < len(parts) {
		return 0, errors.Fmt(`unsupported order by clause: %q; supported orders are "test_id_structured" or "ui_priority desc,test_id_structured"`, orderBy)
	}
	if byUIPriority {
		return OrderingByUIPriority, nil
	}
	return OrderingByID, nil
}
