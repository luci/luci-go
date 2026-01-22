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

package testresultsv2

import (
	"go.chromium.org/luci/common/data/aip132"
	"go.chromium.org/luci/common/errors"
)

// idFieldPath describes the structured test ID of the test result, which is stored in the test_id_structured field.
var idFieldPath = aip132.NewFieldPath("test_id_structured")

// Ordering represents the sort order of test results.
type Ordering int

const (
	// Verdicts should be sorted by primary key. This is the best for RPC performance as it follows the natural
	// table ordering.
	OrderingByPrimaryKey Ordering = iota
	// Verdicts should be sorted by (structured) test identifier.
	OrderingByTestID
)

// ParseOrderBy parses the AIP-132 order_by string.
func ParseOrderBy(orderBy string) (Ordering, error) {
	parts, err := aip132.ParseOrderBy(orderBy)
	if err != nil {
		return 0, err
	}
	var byTestID bool
	index := 0
	if index < len(parts) && parts[index].FieldPath.Equals(idFieldPath) {
		if parts[index].Descending {
			return 0, errors.Fmt("can only sort by %q in ascending order", idFieldPath.String())
		}
		byTestID = true
		index++
	}
	if index < len(parts) {
		return 0, errors.Fmt(`unsupported order by clause: %q; supported orders are "test_id_structured" (test ID order) or "" (system-preferred order)`, orderBy)
	}
	if byTestID {
		return OrderingByTestID, nil
	}
	return OrderingByPrimaryKey, nil
}
