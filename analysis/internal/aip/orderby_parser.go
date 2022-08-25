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

package aip

import (
	"fmt"
	"regexp"
	"strings"
)

// This file contains a parser for AIP-132 order_by values.
// See google.aip.dev/132. Field names are a sequence of one
// or more identifiers matching the pattern [a-zA-Z0-9_]+,
// separated by dots (".").

// OrderBy represents a part of an AIP-132 order_by clause.
type OrderBy struct {
	// The name of the field. This is the externally-visible name
	// of the field, not the database name.
	Name string
	// Whether the field should be sorted in descending order.
	Descending bool
}

// columnOrderRE matches inputs like "some_field" or "some_field.child desc",
// with arbitrary spacing.
var columnOrderRE = regexp.MustCompile(`^ *(\w+(?:\.\w+)*) *( desc)? *$`)

// ParseOrderBy parses the given AIP-132 order_by clause. OrderBy
// directives are returned in the order they appear in the input.
func ParseOrderBy(orderby string) ([]OrderBy, error) {
	if strings.TrimSpace(orderby) == "" {
		return nil, nil
	}

	columnOrder := strings.Split(orderby, ",")
	result := make([]OrderBy, 0, len(columnOrder))
	for _, co := range columnOrder {
		parts := columnOrderRE.FindStringSubmatch(co)
		if parts == nil {
			return nil, fmt.Errorf("invalid ordering %q", co)
		}

		result = append(result, OrderBy{
			Name:       parts[1],
			Descending: parts[2] == " desc",
		})
	}
	return result, nil
}
