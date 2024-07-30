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
	"time"

	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
)

// HappenBefore returns a comparison.Func which checks that some actual time happened
// before 'target'.
func HappenBefore(target time.Time) comparison.Func[time.Time] {
	const cmpName = "should.HappenBefore"

	return func(actual time.Time) *failure.Summary {
		if actual.Before(target) {
			return nil
		}

		// check for equal because otherwise Diff will be empty.
		if actual.Equal(target) {
			return comparison.NewSummaryBuilder(cmpName).
				Because("Times were equal: %s", actual).
				Summary
		}

		return comparison.NewSummaryBuilder(cmpName).
			SmartCmpDiff(actual, target).
			Summary
	}
}

// HappenOnOrBefore returns a comparison.Func which checks that some actual time
// happened on or before 'target'.
func HappenOnOrBefore(target time.Time) comparison.Func[time.Time] {
	return func(actual time.Time) *failure.Summary {
		if actual.Equal(target) || actual.Before(target) {
			return nil
		}

		return comparison.NewSummaryBuilder("should.HappenOnOrBefore").
			SmartCmpDiff(actual, target).
			Summary
	}
}

// HappenAfter returns a comparison.Func which checks that some actual time
// happened after 'target'.
func HappenAfter(target time.Time) comparison.Func[time.Time] {
	const cmpName = "should.HappenAfter"

	return func(actual time.Time) *failure.Summary {
		if actual.After(target) {
			return nil
		}

		// check for equal because otherwise Diff will be empty.
		if actual.Equal(target) {
			return comparison.NewSummaryBuilder(cmpName).
				Because("Times were equal: %s", actual).
				Summary
		}

		return comparison.NewSummaryBuilder(cmpName).
			SmartCmpDiff(actual, target).
			Summary
	}
}

// HappenOnOrAfter returns a comparison.Func which checks that some actual time
// happened on or after 'target'.
func HappenOnOrAfter(target time.Time) comparison.Func[time.Time] {
	return func(actual time.Time) *failure.Summary {
		if actual.Equal(target) || actual.After(target) {
			return nil
		}

		return comparison.NewSummaryBuilder("should.HappenOnOrAfter").
			SmartCmpDiff(actual, target).
			Summary
	}
}

// HappenOnOrBetween returns a comparison.Func which checks that some actual time
// happened on or after 'lower' and on or after 'upper'.
//
// lower must be <= upper or this will panic.
func HappenOnOrBetween(lower, upper time.Time) comparison.Func[time.Time] {
	const cmpName = "should.HappenOnOrBetween"

	if !(lower.Before(upper) || lower.Equal(upper)) {
		panic(fmt.Errorf("%s: !lower.Before(upper)", cmpName))
	}

	return func(actual time.Time) *failure.Summary {
		if actual.Equal(lower) || actual.Equal(upper) {
			return nil
		}
		if actual.After(lower) && actual.Before(upper) {
			return nil
		}

		return comparison.NewSummaryBuilder(cmpName).
			Actual(actual).
			AddFindingf("Expected", "[%s, %s]", lower, upper).
			Summary
	}
}

// HappenWithin returns a comparison.Func which checks that some actual time
// happened within 'delta' of 'target'.
func HappenWithin(delta time.Duration, target time.Time) comparison.Func[time.Time] {
	const cmpName = "should.HappenWithin"

	if delta < 0 {
		panic(fmt.Errorf("%s: negative delta", cmpName))
	}

	lower := target.Add(-delta)
	upper := target.Add(delta)

	return func(actual time.Time) *failure.Summary {
		if actual.Equal(lower) || actual.Equal(upper) {
			return nil
		}
		if actual.After(lower) && actual.Before(upper) {
			return nil
		}

		return comparison.NewSummaryBuilder(cmpName).
			Actual(actual).
			AddFindingf("Expected", "%v ± %v", target, delta).
			Summary
	}
}
