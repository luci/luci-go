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

package lowlatency

import (
	"go.chromium.org/luci/common/data/aip132"
	"go.chromium.org/luci/common/errors"
)

// positionFieldPath describes the source position field.
var positionFieldPath = aip132.NewFieldPath("position")

// Ordering represents the sort order of source verdicts.
type Ordering struct {
	// PositionAscending indicates whether to sort by source position in ascending order.
	PositionAscending bool
}

// ParseOrderBy parses the AIP-132 order_by string.
func ParseOrderBy(orderBy string) (Ordering, error) {
	if orderBy == "" {
		return Ordering{PositionAscending: false}, nil
	}

	parts, err := aip132.ParseOrderBy(orderBy)
	if err != nil {
		return Ordering{}, err
	}

	if len(parts) != 1 {
		return Ordering{}, errors.New(`unsupported order by clause; supported orders are "position" or "position desc"`)
	}

	if parts[0].FieldPath.Equals(positionFieldPath) {
		return Ordering{PositionAscending: !parts[0].Descending}, nil
	}

	return Ordering{}, errors.Fmt("unsupported order by field: %q", parts[0].FieldPath.String())
}
