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
	"fmt"
	"slices"

	"go.chromium.org/luci/common/testing/assert/comparison"
	"golang.org/x/exp/constraints"
)

// TODO(iannucci): these implementations are all extremely similar; consider
// doing a bit of codegen to avoid copy/paste errors.

// BeBetween returns a comparison.Func which checks if an ordered value sorts between
// a lower and upper bound.
func BeBetween[T constraints.Ordered](lower, upper T) comparison.Func[T] {
	const cmpName = "should.BeBetween"

	if lower >= upper {
		return func(t T) *comparison.Failure {
			return comparison.NewFailureBuilder(cmpName, lower).
				Because("%s: `lower` >= `upper`: %#v >= %#v", cmpName, lower, upper).
				Failure
		}
	}

	return func(actual T) *comparison.Failure {
		if actual <= lower {
			return comparison.NewFailureBuilder(cmpName, lower).
				Because("Actual value was too low.").
				Actual(actual).
				AddFindingf("Expected", "> %#v", lower).
				Failure
		}
		if actual >= upper {
			return comparison.NewFailureBuilder(cmpName, lower).
				Because("Actual value was too high.").
				Actual(actual).
				AddFindingf("Expected", "< %#v", upper).
				Failure
		}

		return nil
	}
}

// BeBetweenOrEqual returns a comparison.Func which checks if an ordered value sorts between
// (or equal to) a lower and upper bound.
func BeBetweenOrEqual[T constraints.Ordered](lower, upper T) comparison.Func[T] {
	const cmpName = "should.BeBetweenOrEqual"

	if lower > upper {
		return func(t T) *comparison.Failure {
			return comparison.NewFailureBuilder(cmpName, lower).
				Because("%s: `lower` > `upper`: %#v > %#v", cmpName, lower, upper).
				Failure
		}
	}

	return func(actual T) *comparison.Failure {
		if actual < lower {
			return comparison.NewFailureBuilder(cmpName, lower).
				Because("Actual value was too low.").
				Actual(actual).
				AddFindingf("Expected", ">= %#v", lower).
				Failure
		}
		if actual > upper {
			return comparison.NewFailureBuilder(cmpName, lower).
				Because("Actual value was too high.").
				Actual(actual).
				AddFindingf("Expected", "<= %#v", upper).
				Failure
		}

		return nil
	}
}

// BeLessThan returns a comparison.Func which checks if an ordered value sorts between
// (or equal to) a lower and upper bound.
func BeLessThan[T constraints.Ordered](upper T) comparison.Func[T] {
	return func(actual T) *comparison.Failure {
		if actual < upper {
			return nil
		}

		return comparison.NewFailureBuilder("should.BeLessThan", upper).
			Because("Actual value was too high.").
			Actual(actual).
			AddFindingf("Expected", fmt.Sprintf("< %#v", upper)).
			Failure
	}
}

// BeLessThanOrEqual returns a comparison.Func which checks if an ordered value sorts between
// (or equal to) a lower and upper bound.
func BeLessThanOrEqual[T constraints.Ordered](upper T) comparison.Func[T] {
	return func(actual T) *comparison.Failure {
		if actual <= upper {
			return nil
		}

		return comparison.NewFailureBuilder("should.BeLessThanOrEqual", upper).
			Because("Actual value was too high.").
			Actual(actual).
			AddFindingf("Expected", fmt.Sprintf("<= %#v", upper)).
			Failure
	}
}

// BeGreaterThan returns a comparison.Func which checks if an ordered value is
// greater than a lower bound.
func BeGreaterThan[T constraints.Ordered](lower T) comparison.Func[T] {
	return func(actual T) *comparison.Failure {
		if actual > lower {
			return nil
		}

		return comparison.NewFailureBuilder("should.BeGreaterThan", lower).
			Because("Actual value was too low.").
			Actual(actual).
			AddFindingf("Expected", fmt.Sprintf("> %#v", lower)).
			Failure
	}
}

// BeGreaterThan returns a comparison.Func which checks if an ordered value is
// greater than (or equal to) a lower bound.
func BeGreaterThanOrEqual[T constraints.Ordered](lower T) comparison.Func[T] {
	return func(actual T) (fail *comparison.Failure) {
		if actual >= lower {
			return nil
		}

		return comparison.NewFailureBuilder("should.BeGreaterThanOrEqual", lower).
			Because("Actual value was too low.").
			Actual(actual).
			AddFindingf("Expected", fmt.Sprintf(">= %#v", lower)).
			Failure
	}
}

// BeSorted is a comparison.Func which ensures that a given slice is in sorted
// order.
func BeSorted[T constraints.Ordered](actual []T) (fail *comparison.Failure) {
	if slices.IsSorted(actual) {
		return nil
	}

	sorted := slices.Clone(actual)
	slices.Sort(sorted)

	return comparison.NewFailureBuilder("should.BeSorted", actual).
		SmartCmpDiff(actual, sorted).
		Failure
}
