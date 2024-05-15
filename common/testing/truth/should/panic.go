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
	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
)

// Panic checks whether a function panics.
func Panic(fn func()) *failure.Summary {
	thing := paniccatcher.PCall(fn)
	if thing == nil {
		return comparison.NewSummaryBuilder("should.Panic").Summary
	}
	return nil
}

// PanicLikeString checks if something panics like with a string or error that matches target.
//
// It fails when you panic with anything that is NOT a string or an error.
func PanicLikeString(substring string) comparison.Func[func()] {
	const cmpName = "should.PanicLikeString"
	return func(fn func()) *failure.Summary {
		caught := paniccatcher.PCall(fn)
		if caught == nil {
			return comparison.NewSummaryBuilder(cmpName).
				Because("function did not panic").
				Summary
		}
		exn := caught.Reason
		var str string
		switch v := exn.(type) {
		case string:
			str = v
		case error:
			str = v.Error()
		default:
			return comparison.NewSummaryBuilder(cmpName).
				Because("panic reason is neither error nor string").
				Actual(substring).
				Expected(str).
				Summary
		}
		if strings.Contains(str, substring) {
			return nil
		}
		return comparison.NewSummaryBuilder(cmpName).
			Because("Actual is missing substring.").
			Actual(str).
			AddFindingf("Substring", substring).
			Summary
	}
}

// PanicLikeError checks if something panics like a given error.
func PanicLikeError(target error) comparison.Func[func()] {
	const cmpName = "should.PanicLikeError"
	return func(fn func()) *failure.Summary {
		if target == nil {
			return comparison.NewSummaryBuilder(cmpName).
				Because("nil as expected panic is not allowed; use runtime.PanicNilError instead").
				Summary
		}
		thing := paniccatcher.PCall(fn)
		if thing == nil {
			return comparison.NewSummaryBuilder(cmpName).
				Because("function did not panic").
				Summary
		}
		exn := thing.Reason
		e, ok := exn.(error)
		if !ok {
			return comparison.NewSummaryBuilder(cmpName).
				Because("caught panic is not an error").
				Actual(e).
				Expected(target).
				AddFindingf("stack", thing.Stack).
				WarnIfLong().
				Summary
		}
		if !errors.Is(e, target) {
			return comparison.NewSummaryBuilder(cmpName).
				Because("error does not match target").
				Actual(e).
				Expected(target).
				Summary
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
