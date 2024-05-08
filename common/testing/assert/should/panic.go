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

	"go.chromium.org/luci/common/runtime/paniccatcher"
	"go.chromium.org/luci/common/testing/assert/comparison"
)

// Panic checks whether a function panics.
func Panic(fn func()) *comparison.Failure {
	thing := paniccatcher.PCall(fn)
	if thing == nil {
		return comparison.NewFailureBuilder("should.Panic").Failure
	}
	return nil
}

// PanicLikeString checks if something panics like with a string or error that matches target.
//
// It fails when you panic with anything that is NOT a string or an error.
func PanicLikeString(substring string) comparison.Func[func()] {
	const cmpName = "should.PanicLikeString"
	return func(fn func()) *comparison.Failure {
		caught := paniccatcher.PCall(fn)
		if caught == nil {
			return comparison.NewFailureBuilder(cmpName).
				Because("function did not panic").
				Failure
		}
		exn := caught.Reason
		var str string
		switch v := exn.(type) {
		case string:
			str = v
		case error:
			str = v.Error()
		default:
			return comparison.NewFailureBuilder(cmpName).
				Because("panic reason is neither error nor string").
				Actual(substring).
				Expected(str).
				Failure
		}
		if strings.Contains(str, substring) {
			return nil
		}
		return comparison.NewFailureBuilder(cmpName).
			Because("Actual is missing substring.").
			Actual(str).
			AddFindingf("Substring", substring).
			Failure
	}
}

// PanicLikeError checks if something panics like a given error.
func PanicLikeError(target error) comparison.Func[func()] {
	const cmpName = "should.PanicLikeError"
	return func(fn func()) *comparison.Failure {
		if target == nil {
			return comparison.NewFailureBuilder(cmpName).
				Because("nil as expected panic is not allowed; use runtime.PanicNilError instead").
				Failure
		}
		thing := paniccatcher.PCall(fn)
		if thing == nil {
			return comparison.NewFailureBuilder(cmpName).
				Because("function did not panic").
				Failure
		}
		exn := thing.Reason
		e, ok := exn.(error)
		if !ok {
			return comparison.NewFailureBuilder(cmpName).
				Because("caught panic is not an error").
				Actual(e).
				Expected(target).
				AddFindingf("stack", thing.Stack).
				WarnIfLong().
				Failure
		}
		if !errors.Is(e, target) {
			return comparison.NewFailureBuilder(cmpName).
				Because("error does not match target").
				Actual(e).
				Expected(target).
				Failure
		}
		return nil
	}
}

// PanicLike checks whether a function panics like a string or an error, which is cool.
//
// Deprecated: use PanicLikeString or PanicLikeError depending on the type.
func PanicLike(target any) comparison.Func[func()] {
	switch v := target.(type) {
	case string:
		return PanicLikeString(v)
	case error:
		return PanicLikeError(v)
	default:
		panic(fmt.Errorf("PanicLike expects a string or an error, got %T", target))
	}
}
