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

// Package assert contains two functions, which allow you to make fluent truth
// comparisons which t.FailNow a test.
//
// Example:
//
//	assert.That(t, 10, should.Equal(20))
//	assert.Loosely(t, myCustomInt(10), should.Equal(20))
//
// In the example above, the test case would halt immediately after the first
// `assert.That`, because 10 does not equal 20, and `assert.That` will call
// t.FailNow().
//
// This package has a sibling package `check` which instead does t.Fail,
// allowing tests to make multiple checks without halting.
package assert

import (
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/option"
)

// That will compare `actual` using `compare(actual)`.
//
// If this results in a failure.Summary, it will be reported with truth.Report,
// and the test will be failed with t.Fail().
//
// Example: `assert.That(t, 10, should.Equal(20))`
//
// Returns `true` iff `compare(actual)` returned no failure (i.e. nil)
func That[T any](t truth.TestingTB, actual T, compare comparison.Func[T], opts ...option.Option) {
	if summary := option.ApplyAll(compare(actual), opts); summary != nil {
		t.Helper()
		truth.Report(t, "assert.That", summary)
		t.FailNow()
	}
}

// Loosely will compare `actual` using `compare.CastCompare(actual)`.
//
// If this results in a failure.Summary, it will be reported with truth.Report,
// and the test will be failed with t.Fail().
//
// Example: `assert.Loosely(t, 10, should.Equal(20))`
//
// Returns `true` iff `compare.CastCompare(actual)` returned no failure (i.e. nil)
func Loosely[T any](t truth.TestingTB, actual any, compare comparison.Func[T], opts ...option.Option) {
	if summary := option.ApplyAll(compare.CastCompare(actual), opts); summary != nil {
		t.Helper()
		truth.Report(t, "assert.Loosely", summary)
		t.FailNow()
	}
}
