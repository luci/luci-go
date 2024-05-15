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
	"reflect"

	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
)

// canLen returns true iff the `anything` Value can safely use with
// reflect.Value.Len().
func canLen(anything reflect.Value) bool {
	switch anything.Kind() {
	case reflect.Array, reflect.Slice, reflect.Map, reflect.String, reflect.Chan:
		return true
	case reflect.Pointer:
		return anything.Type().Elem().Kind() == reflect.Array
	}
	return false
}

// getLen returns the length of `anything` equivalent to the builtin `len()`
// function.
func getLen(cmpName string, anything any) (int, *failure.Summary) {
	val := reflect.ValueOf(anything)
	if !canLen(val) {
		return 0, comparison.NewSummaryBuilder(cmpName).
			Because("%T does not support `len()`", anything).
			Summary
	}
	return reflect.ValueOf(anything).Len(), nil
}

// HaveLength returns a Comparison which expects `actual` to have the given
// length.
//
// Supports all values which work with the `len()` builtin function.
func HaveLength(expected int) comparison.Func[any] {
	const cmpName = "should.HaveLength"

	if expected < 0 {
		return func(a any) *failure.Summary {
			return comparison.NewSummaryBuilder(cmpName).
				Because("Expected value is a negative length").
				Expected(expected).
				Summary
		}
	}

	return func(actual any) *failure.Summary {
		l, fail := getLen(cmpName, actual)
		if fail != nil {
			return fail
		}
		if l == expected {
			return nil
		}

		return comparison.NewSummaryBuilder(cmpName, actual).
			AddFindingf("len(Actual)", "%d", actual).
			Expected(expected).
			Summary
	}
}

// BeEmpty is a Comparison which expects `actual` to have length 0.
//
// Supports all values which work with the `len()` builtin function.
//
// TODO: Improve this when there is a `Lengthable` type constraint.
func BeEmpty(actual any) *failure.Summary {
	const cmpName = "should.BeEmpty"

	l, fail := getLen(cmpName, actual)
	if fail != nil {
		return fail
	}
	if l == 0 {
		return nil
	}
	return comparison.NewSummaryBuilder(cmpName, actual).Summary
}

// NotBeEmpty is a Comparison which expects `actual` to have a non-0
// length.
//
// Supports all values which work with the `len()` builtin function.
func NotBeEmpty(actual any) *failure.Summary {
	const cmpName = "should.NotBeEmpty"

	l, fail := getLen(cmpName, actual)
	if fail != nil {
		return fail
	}
	if l != 0 {
		return nil
	}
	return comparison.NewSummaryBuilder(cmpName, actual).Summary
}
