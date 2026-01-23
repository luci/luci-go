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
//
// If no ordering is specified, the implementation will revert to sorting
// by primary key, which is the most efficient order for performance.
type Ordering struct {
	// If multiple orders are combined, they are applied in the order they appear
	// below. I.E. ByUIPriority sort takes priority over ByStructuredTestID sort.

	// Whether to order by UI priority descending (most important first).
	// This is means the order will be:
	// - Failed
	// - Execution Error
	// - Precluded
	// - Flaky
	// - Exonerated
	// - Passed and Skipped (treated equivalently).
	ByUIPriority bool
	// Whether to order by structured test ID ascending ('a's before 'z's).
	ByStructuredTestID bool
}

// ParseOrderBy parses the order_by string.
func ParseOrderBy(orderBy string) (Ordering, error) {
	parts, err := aip132.ParseOrderBy(orderBy)
	if err != nil {
		return Ordering{}, err
	}
	var byUIPriority bool
	var byStructuredTestID bool
	index := 0
	if index < len(parts) && parts[index].FieldPath.Equals(uiPriorityFieldPath) {
		if parts[index].Descending {
			return Ordering{}, errors.Fmt("can only sort by %q in ascending order", uiPriorityFieldPath.String())
		}
		byUIPriority = true
		index++
	}
	if index < len(parts) && parts[index].FieldPath.Equals(idFieldPath) {
		if parts[index].Descending {
			return Ordering{}, errors.Fmt("can only sort by %q in ascending order", idFieldPath.String())
		}
		byStructuredTestID = true
		index++
	}
	if index < len(parts) {
		return Ordering{}, errors.Fmt(`unsupported order by clause: %q; supported orders are "test_id_structured" or "ui_priority,test_id_structured", "ui_priority" or "" (system-preferred order)`, orderBy)
	}
	return Ordering{
		ByUIPriority:       byUIPriority,
		ByStructuredTestID: byStructuredTestID,
	}, nil
}
