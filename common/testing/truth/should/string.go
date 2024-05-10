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
)

// ContainSubstring returns a comparison.Func which checks to see if a string
// contains `substr`.
func ContainSubstring(substr string) comparison.Func[string] {
	return func(actual string) *comparison.Failure {
		if strings.Contains(actual, substr) {
			return nil
		}
		return comparison.NewFailureBuilder("should.ContainSubstring").
			SmartCmpDiff(actual, substr).
			RenameFinding("Expected", "Substring").
			Failure
	}
}

// NotContainSubstring returns a comparison.Func which checks to see if a string
// does not contain `substr`.
func NotContainSubstring(substr string) comparison.Func[string] {
	return func(actual string) *comparison.Failure {
		if !strings.Contains(actual, substr) {
			return nil
		}
		return comparison.NewFailureBuilder("should.NotContainSubstring").
			SmartCmpDiff(actual, substr).
			RenameFinding("Expected", "Substring").
			Failure
	}
}

// HaveSuffix returns a comparison.Func which checks to see if a string
// ends with `suffix`.
func HaveSuffix(suffix string) comparison.Func[string] {
	return func(actual string) *comparison.Failure {
		if strings.HasSuffix(actual, suffix) {
			return nil
		}
		return comparison.NewFailureBuilder("should.HaveSuffix").
			SmartCmpDiff(actual, suffix).
			RenameFinding("Expected", "Suffix").
			Failure
	}
}

// NotHaveSuffix returns a comparison.Func which checks to see if a string
// does not end with `suffix`.
func NotHaveSuffix(suffix string) comparison.Func[string] {
	return func(actual string) *comparison.Failure {
		if !strings.HasSuffix(actual, suffix) {
			return nil
		}
		return comparison.NewFailureBuilder("should.NotHaveSuffix").
			SmartCmpDiff(actual, suffix).
			RenameFinding("Expected", "Suffix").
			Failure
	}
}

// HavePrefix returns a comparison.Func which checks to see if a string
// ends with `prefix`.
func HavePrefix(prefix string) comparison.Func[string] {
	return func(actual string) *comparison.Failure {
		if strings.HasPrefix(actual, prefix) {
			return nil
		}
		return comparison.NewFailureBuilder("should.HavePrefix").
			SmartCmpDiff(actual, prefix).
			RenameFinding("Expected", "Prefix").
			Failure
	}
}

// NotHavePrefix returns a comparison.Func which checks to see if a string
// does not end with `prefix`.
func NotHavePrefix(prefix string) comparison.Func[string] {
	return func(actual string) *comparison.Failure {
		if !strings.HasPrefix(actual, prefix) {
			return nil
		}
		return comparison.NewFailureBuilder("should.NotHavePrefix").
			SmartCmpDiff(actual, prefix).
			RenameFinding("Expected", "Prefix").
			Failure
	}
}

// BeBlank implements comparison.Func[string] and asserts that `actual` contains
// only whitespace chars.
func BeBlank(actual string) *comparison.Failure {
	const cmpName = "should.BeBlank"

	for i, ch := range actual {
		if unicode.IsSpace(ch) {
			continue
		}
		return comparison.NewFailureBuilder(cmpName).
			Because("string contains non-blank character").
			Actual(actual).
			AddFindingf("non-whitespace char", "%q", ch).
			AddFindingf("rune index", "%d", i).
			Failure
	}

	return nil
}

// NotBeBlank implements comparison.Func[string] and asserts that `actual`
// contains at least one non-whitespace char.
func NotBeBlank(actual string) *comparison.Failure {
	const cmpName = "should.NotBeBlank"

	isBlank := BeBlank(actual) == nil

	if isBlank {
		return comparison.NewFailureBuilder(cmpName).
			Because("string contains only whitespace").
			Actual(actual).
			Failure
	}
	return nil
}
