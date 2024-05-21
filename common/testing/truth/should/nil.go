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

func isNil(cmpName string, actual any) (isNil bool, ret *failure.Summary) {
	defer func() {
		if recover() != nil {
			ret = comparison.NewSummaryBuilder(cmpName).
				Because("`%T` cannot be checked for nil", actual).
				Summary
		}
	}()
	isNil = reflect.ValueOf(actual).IsNil()
	return
}

// BeNil implements comparison.Func[any] and asserts that `actual` is a nillable
// type (like a pointer, slice, channel, etc) and is nil. This will also accept
// untyped nils (e.g. `any(nil)`).
//
// Note that this is a stricter form than should.BeZero.
func BeNil(actual any) *failure.Summary {
	if err, ok := actual.(error); ok {
		return ErrLikeError(nil)(err)
	}

	const cmpName = "should.BeNil"

	if actual == nil {
		return nil
	}
	if isNil, rslt := isNil(cmpName, actual); isNil || rslt != nil {
		return rslt
	}

	return comparison.NewSummaryBuilder(cmpName).Actual(actual).Summary
}

// NotBeNil implements comparison.Func[any] and asserts that `actual` is a nillable
// type (like a pointer, slice, channel, etc) and is not nil.
//
// Note that this is a slightly stricter form than should.NotBeZero.
func NotBeNil(actual any) *failure.Summary {
	const cmpName = "should.NotBeNil"

	if actual != nil {
		if isNil, rslt := isNil(cmpName, actual); !isNil || rslt != nil {
			return rslt
		}
	}

	return comparison.NewSummaryBuilder(cmpName).Summary
}

// BeZero implements comparison.Func[any] and asserts that `actual` is
// a zero value (by Go's definition of zero values, e.g. empty strings, structs,
// nil pointers, etc).
func BeZero(actual any) *failure.Summary {
	const cmpName = "should.BeZero"

	if actual == nil {
		return nil
	}
	if reflect.ValueOf(actual).IsZero() {
		return nil
	}

	return comparison.NewSummaryBuilder(cmpName, actual).
		Actual(actual).Summary
}

// NotBeZero implements comparison.Func[any] and asserts that `actual` is
// a non-zero value (by Go's definition of zero values, e.g. empty strings, structs,
// nil pointers, etc).
func NotBeZero(actual any) *failure.Summary {
	const cmpName = "should.NotBeZero"

	if actual != nil {
		if !reflect.ValueOf(actual).IsZero() {
			return nil
		}
	}

	return comparison.NewSummaryBuilder(cmpName, actual).Summary
}
