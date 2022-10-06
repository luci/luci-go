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

package lang

import (
	"bytes"
	"sort"
	"strings"
)

// Merge merges two failure association rules with a logical "OR".
func Merge(rule1 string, rule2 string) (string, error) {
	r1, err := Parse(rule1)
	if err != nil {
		return "", err
	}
	r2, err := Parse(rule2)
	if err != nil {
		return "", err
	}
	// The final "OR" conjoined terms.
	var allTerms []*boolTerm
	allTerms = append(allTerms, r1.expr.Terms...)
	allTerms = append(allTerms, r2.expr.Terms...)

	// Sort the top-level OR'ed terms.
	// A common case we see is a list of tests:
	// test = "mytest://a" OR
	// test = "mytest://b" ...
	// and having the terms sorted makes it easier to
	// read the rule.
	var stringTerms []string
	for _, term := range allTerms {
		var buf bytes.Buffer
		term.format(&buf)
		stringTerms = append(stringTerms, buf.String())
	}
	sort.Strings(stringTerms)

	// Note that this is only valid because OR is the operator
	// with the lowest precedence in our language.
	// Otherwise we would have to be concerned about inserting
	// parentheses.
	return strings.Join(stringTerms, " OR\n"), nil
}
