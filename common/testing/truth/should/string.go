// Copyright 2024 The LUCI Authors.
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
	"strings"
	"unicode"

	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
)

// ContainSubstring returns a comparison.Func which checks to see if a string
// contains `substr`.
func ContainSubstring(substr string) comparison.Func[string] {
	return func(actual string) *failure.Summary {
		if strings.Contains(actual, substr) {
			return nil
		}
		return comparison.NewSummaryBuilder("should.ContainSubstring").
			SmartCmpDiff(actual, substr).
			RenameFinding("Expected", "Substring").
			Summary
	}
}

// NotContainSubstring returns a comparison.Func which checks to see if a string
// does not contain `substr`.
func NotContainSubstring(substr string) comparison.Func[string] {
	return func(actual string) *failure.Summary {
		if !strings.Contains(actual, substr) {
			return nil
		}
		return comparison.NewSummaryBuilder("should.NotContainSubstring").
			SmartCmpDiff(actual, substr).
			RenameFinding("Expected", "Substring").
			Summary
	}
}

// HaveSuffix returns a comparison.Func which checks to see if a string
// ends with `suffix`.
func HaveSuffix(suffix string) comparison.Func[string] {
	return func(actual string) *failure.Summary {
		if strings.HasSuffix(actual, suffix) {
			return nil
		}
		return comparison.NewSummaryBuilder("should.HaveSuffix").
			SmartCmpDiff(actual, suffix).
			RenameFinding("Expected", "Suffix").
			Summary
	}
}

// NotHaveSuffix returns a comparison.Func which checks to see if a string
// does not end with `suffix`.
func NotHaveSuffix(suffix string) comparison.Func[string] {
	return func(actual string) *failure.Summary {
		if !strings.HasSuffix(actual, suffix) {
			return nil
		}
		return comparison.NewSummaryBuilder("should.NotHaveSuffix").
			SmartCmpDiff(actual, suffix).
			RenameFinding("Expected", "Suffix").
			Summary
	}
}

// HavePrefix returns a comparison.Func which checks to see if a string
// ends with `prefix`.
func HavePrefix(prefix string) comparison.Func[string] {
	return func(actual string) *failure.Summary {
		if strings.HasPrefix(actual, prefix) {
			return nil
		}
		return comparison.NewSummaryBuilder("should.HavePrefix").
			SmartCmpDiff(actual, prefix).
			RenameFinding("Expected", "Prefix").
			Summary
	}
}

// NotHavePrefix returns a comparison.Func which checks to see if a string
// does not end with `prefix`.
func NotHavePrefix(prefix string) comparison.Func[string] {
	return func(actual string) *failure.Summary {
		if !strings.HasPrefix(actual, prefix) {
			return nil
		}
		return comparison.NewSummaryBuilder("should.NotHavePrefix").
			SmartCmpDiff(actual, prefix).
			RenameFinding("Expected", "Prefix").
			Summary
	}
}

// BeBlank implements comparison.Func[string] and asserts that `actual` contains
// only whitespace chars.
func BeBlank(actual string) *failure.Summary {
	const cmpName = "should.BeBlank"

	for i, ch := range actual {
		if unicode.IsSpace(ch) {
			continue
		}
		return comparison.NewSummaryBuilder(cmpName).
			Because("string contains non-blank character").
			Actual(actual).
			AddFindingf("non-whitespace char", "%q", ch).
			AddFindingf("rune index", "%d", i).
			Summary
	}

	return nil
}

// NotBeBlank implements comparison.Func[string] and asserts that `actual`
// contains at least one non-whitespace char.
func NotBeBlank(actual string) *failure.Summary {
	const cmpName = "should.NotBeBlank"

	isBlank := BeBlank(actual) == nil

	if isBlank {
		return comparison.NewSummaryBuilder(cmpName).
			Because("string contains only whitespace").
			Actual(actual).
			Summary
	}
	return nil
}
