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
	"errors"
	"fmt"
	"strings"

	"github.com/google/go-cmp/cmp/cmpopts"

	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
)

// ErrLikeString returns failure when the stringified error is not a substring
// of the target. Additionally, when the substring argument is empty, ErrLikeString
// always returns failure
// because the empty string is a subset of every string and you probably made a
// mistake.
func ErrLikeString(substring string) comparison.Func[error] {
	const cmpName = "should.ErrLikeString"
	if substring == "" {
		return func(error) *failure.Summary {
			return comparison.NewSummaryBuilder(cmpName).
				Because(`"" is a substring of every string. Use ErrLikeError(nil) or BeNil.`).
				Summary
		}
	}
	return func(actual error) *failure.Summary {
		if actual == nil {
			return comparison.NewSummaryBuilder(cmpName).
				Because("actual is nil and therefore doesn't contain any non-empty substrings").
				Actual(substring).
				AddFindingf("substring", substring).
				Summary
		}
		a := actual.Error()
		if strings.Contains(a, substring) {
			return nil
		}
		return comparison.NewSummaryBuilder(cmpName).
			Because("`actual.Error()` is missing substring.").
			Actual(a).
			AddFindingf("actual.Error()", "%q", a).
			Expected(substring).
			AddFindingf("Substring", substring).
			Summary
	}
}

// ErrLikeError checks whether your error errors.Is another error.
//
// nil is an acceptable target here indicating the absence of an error.
func ErrLikeError(target error) comparison.Func[error] {
	const cmpName = "should.ErrLikeError"
	if target == nil {
		return func(actual error) *failure.Summary {
			if actual == nil {
				return nil
			}
			return comparison.NewSummaryBuilder(cmpName).
				AddComparisonArgs("nil").
				Actual(actual).
				AddFindingf("actual.Error()", actual.Error()).
				Summary
		}
	}
	return func(actual error) *failure.Summary {
		// target MUST be non-nil at this point
		if actual == nil {
			return comparison.NewSummaryBuilder(cmpName, target).
				Because("Actual is nil but target is not").
				Expected(target).
				AddFindingf("target.Error()", target.Error()).
				Summary
		}
		if errors.Is(actual, target) {
			return nil
		}
		return comparison.NewSummaryBuilder(cmpName, target).
			Because("Actual does not contain the expected error.").
			SmartCmpDiff(actual, target, cmpopts.EquateErrors()).
			Summary
	}
}

// ErrLike returns a Comparison[error] which will check that `actual` is:
//
//   - `nil`, if `stringOrError` is nil.
//   - errors.Is(actual, stringOrError) if `stringOrError` is `error`.
//   - strings.Contains(actual.Error(), stringOrError) if `stringOrError` is
//     `string.
//
// # Examples
//
//	err := funcReturnsErr()
//	Assert(t, err, should.ErrLike("bad value"))  // strings.Contains
//	Assert(t, err, should.ErrLike(ErrBadValue))  // errors.Is
//	Assert(t, err, should.ErrLike(nil))          // err == nil
func ErrLike(stringOrError any) comparison.Func[error] {
	switch expected := stringOrError.(type) {
	case nil:
		return ErrLikeError(nil)
	case string:
		return ErrLikeString(expected)
	case error:
		return ErrLikeError(expected)
	}
	panic(fmt.Errorf("should.ErrLike: got `%T`, not `nil`, `string`, or `error`", stringOrError))
}
