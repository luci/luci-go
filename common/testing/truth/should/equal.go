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
	"math"
	"reflect"
	"time"

	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
)

func checkIsNaN[T comparable](cmpName string, expected T) comparison.Func[T] {
	val := reflect.ValueOf(expected)
	switch kind := val.Kind(); kind {
	case reflect.Float32, reflect.Float64:
		if math.IsNaN(val.Float()) {
			return func(t T) *failure.Summary {
				return comparison.NewSummaryBuilder(cmpName, expected).
					Because("Cannot compare to float(NaN), use should.BeNaN instead.").
					Summary
			}
		}
	}
	return nil
}

var timeType = reflect.TypeFor[time.Time]()

func timeEqual[T any](cmpName string) comparison.Func[T] {
	return func(actual T) *failure.Summary {
		return comparison.NewSummaryBuilder(cmpName, actual).
			AddFindingf("Error", "should.Equal[time.Time] is always incorrect. Use should.Match instead.").
			Because("time.Time has internal fields due to support for monotonic time.\n" +
				"Comparing time.Time with struct equivalence leads to baffling results where \n" +
				"two time.Time objects with the same wallclock time end up being unequal.").
			Summary
	}
}

// Equal checks whether two objects are equal, as determined by Go's `==`
// operator.
//
// Notably, NaN (the float value) cannot compare to itself. This Comparison
// implementation will return a specific error in the event that `expected` and
// `actual` are NaN.
func Equal[T comparable](expected T) comparison.Func[T] {
	cmpName := "should.Equal"

	if reflect.TypeFor[T]() == timeType {
		return timeEqual[T](cmpName)
	}

	if fn := checkIsNaN(cmpName, expected); fn != nil {
		return fn
	}

	return func(actual T) (ret *failure.Summary) {
		if actual == expected {
			return nil
		}

		builder := comparison.NewSummaryBuilder(cmpName, expected)
		if reflect.TypeFor[T]().Kind() == reflect.Pointer {
			builder.AddFindingf("Warning",
				"This compared two pointers - did you want should.Match instead?")
		}

		return builder.SmartCmpDiff(actual, expected).Summary
	}
}

// NotEqual checks whether two objects are equal, as determined by Go's `!=`
// operator.
//
// Notably, NaN (the float value) cannot compare to itself. This Comparison
// implementation will return a specific error in the event that `expected` and
// `actual` are NaN.
func NotEqual[T comparable](expected T) comparison.Func[T] {
	cmpName := "should.NotEqual"

	if reflect.TypeFor[T]() == timeType {
		return timeEqual[T](cmpName)
	}

	if fn := checkIsNaN(cmpName, expected); fn != nil {
		return fn
	}

	return func(actual T) (ret *failure.Summary) {
		if actual != expected {
			return nil
		}

		return comparison.NewSummaryBuilder(cmpName, expected).
			Actual(actual).
			Summary
	}
}
