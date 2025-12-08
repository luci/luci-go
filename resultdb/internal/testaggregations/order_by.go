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

package testaggregations

import (
	"go.chromium.org/luci/common/data/aip132"
	"go.chromium.org/luci/common/errors"
)

// Ordering defines the order of test aggregations.
// The default sort order (and residual order) is always natural ID order.
type Ordering struct {
	// ByLevelFirst indicates results should be ordered by aggregation level first.
	// If the user requested more than one level of aggregation, this must always be true.
	ByLevelFirst bool
	// ByUIPriority indicates whether the next ordering (first if ByLevelFirst is false)
	// should be by UI priority descending. If this is false, the natural ID order is used instead.
	ByUIPriority bool
}

// idFieldPath describes the ID of the aggregation, which is stored in the id.id field.
var idFieldPath = aip132.NewFieldPath("id", "id")

// levelFieldPath describes the level of the aggregation, which is stored in the id.level field.
var levelFieldPath = aip132.NewFieldPath("id", "level")

// uiPriorityFieldPath describes the UI priority of the aggregation, which is stored in the ui_priority field.
var uiPriorityFieldPath = aip132.NewFieldPath("ui_priority")

// ParseOrderBy parses an AIP-132 order_by parameter for use with test aggregations.
//
// Supported orders are:
// - "" (gives default order: currently id.level,id.id)
// - "id.level,id.id" (explicit default)
// - "id.level,ui_priority desc,id.id" (UI sort order)
//
// In case of single-level queries id.level may be elided as it is implicit.
func ParseOrderBy(orderBy string) (Ordering, error) {
	parts, err := aip132.ParseOrderBy(orderBy)
	if err != nil {
		return Ordering{}, err
	}
	if len(parts) == 0 {
		// Use default order: Level asc, ID asc.
		return Ordering{ByLevelFirst: true}, nil
	}
	var result Ordering
	index := 0
	if parts[index].FieldPath.Equals(levelFieldPath) {
		if parts[index].Descending {
			return Ordering{}, errors.New("can only sort by `id.level` in ascending order")
		}
		result.ByLevelFirst = true
		index++
	}
	if index < len(parts) && parts[index].FieldPath.Equals(uiPriorityFieldPath) {
		if !parts[index].Descending {
			return Ordering{}, errors.New("can only sort by `ui_priority` in descending order")
		}
		result.ByUIPriority = true
		index++
	}
	if index < len(parts) && parts[index].FieldPath.Equals(idFieldPath) {
		if parts[index].Descending {
			return Ordering{}, errors.New("can only sort by `id.id` in ascending order")
		}
		// The default (and residual) order is always id.id ascending, so no need to set anything.
		index++
	}
	if index < len(parts) {
		return Ordering{}, errors.Fmt("unsupported order by clause: %q; supported orders are `id.level,id.id` or `id.level,ui_priority desc,id.id` (id.level may be elided if only one level is requested)", orderBy)
	}
	return result, nil
}
