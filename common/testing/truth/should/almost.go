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

	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
)

func almostEqualComputeEpsilon[T ~float32 | ~float64](cmpName string, bits int, epsilon ...T) (ep T, errFn comparison.Func[T]) {
	if len(epsilon) > 1 {
		errFn = func(t T) *failure.Summary {
			return comparison.NewSummaryBuilder(cmpName, t).
				Because("%s: `epsilon` is a single optional value, got %d values", cmpName, len(epsilon)).
				Summary
		}
		return
	}

	if len(epsilon) == 1 {
		ep = epsilon[0]
	} else {
		// This construction computes the equivalent of FLT_EPSILON for float32 or
		// float64 respectively. Note that we have to 'cast' the result of the
		// function to T, but in reality this will be a no-op cast in the compiled
		// output.
		if bits == 32 {
			ep = T(math.Nextafter32(1, 2) - 1)
		} else {
			ep = T(math.Nextafter(1, 2) - 1)
		}
	}
	if ep < 0 {
		errFn = func(t T) *failure.Summary {
			return comparison.NewSummaryBuilder(cmpName, t).
				Because("%s: `epsilon` is negative: %v", cmpName, ep).
				Summary
		}
	}

	return
}

func almostEqualCheck[T ~float32 | ~float64](actual, target, epsilon T) (delta T, inEpsilon bool) {
	delta = actual - target
	deltaAbs := delta
	if deltaAbs < 0 {
		deltaAbs = -deltaAbs
	}
	inEpsilon = deltaAbs <= epsilon
	return
}

// AlmostEqual returns a comparison Func which checks if a floating point value
// is within `|epsilon|` of `target`.
//
// By default, this computes `epsilon` to be `math.Nextafter(1, 2) - 1` (or the
// 32 bit equivalent). This yields the next representable floating point value
// after `1` (effectively Float64frombits(Float64bits(x)+1)).
//
// You may optionally pass a (single, positive) explicit epsilon value.
func AlmostEqual[T ~float32 | ~float64](target T, epsilon ...T) comparison.Func[T] {
	const cmpName = "should.AlmostEqual"
	ep, errFn := almostEqualComputeEpsilon(cmpName, reflect.TypeOf(target).Bits(), epsilon...)
	if errFn != nil {
		return errFn
	}

	return func(actual T) *failure.Summary {
		delta, inEpsilon := almostEqualCheck(actual, target, ep)
		if inEpsilon {
			return nil
		}

		return comparison.NewSummaryBuilder(cmpName, target).
			Because("Actual value was %g off of target.", delta).
			Actual(actual).
			AddFindingf("Expected", "%g ± %g", target, ep).
			Summary
	}
}
