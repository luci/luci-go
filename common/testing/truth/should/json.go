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

package should

import (
	json "encoding/json"

	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
)

// EqualJSON is a Comparison that compares two []bytes to see
// whether they are structurally equal or not when compared as JSON.
//
// The output is a diff of the canonicalized JSON structures, or nil
// if they are structurally equal.
func EqualJSON(expected []byte) comparison.Func[[]byte] {
	const cmpName = "should.EqualJSON"
	return func(actual []byte) *failure.Summary {
		e, err := canonicalizeJSON(expected)
		if err != nil {
			return comparison.NewSummaryBuilder(cmpName).
				Expected(expected).
				Because("expected is not valid JSON").
				Summary
		}
		a, err := canonicalizeJSON(actual)
		if err != nil {
			return comparison.NewSummaryBuilder(cmpName).
				Actual(actual).
				Because("actual is not valid JSON").
				Summary
		}
		res := Equal(string(e))(string(a))
		if res != nil {
			comparison.SetComparison(res, cmpName, expected, actual)
		}
		return res
	}
}

func canonicalizeJSON(input []byte) ([]byte, error) {
	var parsed interface{}
	if err := json.Unmarshal(input, &parsed); err != nil {
		return nil, err
	}
	return json.MarshalIndent(parsed, "", "  ")
}
