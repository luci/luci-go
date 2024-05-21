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

package test_outputs

import (
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"testing"
)

func myCustomComparison(actual any, expected ...any) string {
	if len(expected) != 1 {
		return "wrong number of expected arguments (should be 1)"
	}
	actualS, actualIsString := actual.(string)
	expectedS, expectedIsString := expected[0].(string)
	if !actualIsString {
		return "actual is not a string"
	}
	if !expectedIsString {
		return "expected is not a string"
	}
	if actualS == expectedS {
		return ""
	}
	return "actual != expected"
}

func TestCustomComparisonTest(t *testing.T) {
	t.Parallel()

	ftt.Run("something", t, func(t *ftt.Test) {
		assert.Loosely(t, "cheese", convey.Adapt(myCustomComparison)("cheese"))
	})
}
