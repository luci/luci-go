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
	"go.chromium.org/luci/common/testing/assert/comparison"
)

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
	cmpName := "should.ErrLike"

	switch expected := stringOrError.(type) {
	case nil:
		return func(actual error) *comparison.Failure {
			if actual == nil {
				return nil
			}
			return comparison.NewFailureBuilder(cmpName, nil).
				Because("Error was non-nil").
				Actual(actual).
				Failure
		}

	case string:
		return func(actual error) *comparison.Failure {
			actualErrStr := actual.Error()
			if strings.Contains(actualErrStr, expected) {
				return nil
			}
			return comparison.NewFailureBuilder(cmpName, expected).
				Because("`actual.Error()` is missing substring.").
				Actual(actualErrStr).
				AddFormattedFinding("Substring", expected).
				Failure
		}

	case error:
		return func(actual error) *comparison.Failure {
			if errors.Is(actual, expected) {
				return nil
			}
			return comparison.NewFailureBuilder(cmpName, expected).
				Because("Actual does not contain the expected error.").
				SmartCmpDiff(actual, expected, cmpopts.EquateErrors()).
				Failure
		}
	}

	panic(fmt.Errorf("should.ErrLike: got `%T`, not `nil`, `string`, or `error`", stringOrError))
}
