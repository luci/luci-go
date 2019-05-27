// Copyright 2015 The LUCI Authors.
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

package assertions

import (
	"fmt"

	"go.chromium.org/luci/common/errors"

	"github.com/smartystreets/assertions"
)

// ShouldContainErr checks if an `errors.MultiError` on the left side contains
// as one of its errors an `error` or `string` on the right side. If nothing is
// provided on the right, checks that the left side contains at least one non-nil
// error. If nil is provided on the right, checks that the left side contains
// at least one nil, even if it contains other errors.
//
// Equivalent to calling ShouldErrLike on each `error` in an `errors.MultiError`
// and succeeding as long as one of the ShouldErrLike calls succeeds.
//
// To avoid confusion, explicitly rejects the special case where the right side is
// an `errors.MultiError`.
func ShouldContainErr(actual interface{}, expected ...interface{}) string {
	if len(expected) > 1 {
		return fmt.Sprintf("ShouldContainErr requires 0 or 1 expected value, got %d", len(expected))
	}

	if actual == nil {
		return assertions.ShouldNotBeNil(actual)
	}

	me, ok := actual.(errors.MultiError)
	if !ok {
		return assertions.ShouldHaveSameTypeAs(actual, errors.MultiError{})
	}

	if len(expected) == 0 {
		return assertions.ShouldNotBeNil(me.First())
	}

	switch expected[0].(type) {
	case string:
	case error:
	case errors.MultiError:
		return fmt.Sprintf("expected value must not be a MultiError")
	default:
		if expected[0] != nil {
			return fmt.Sprintf("unexpected argument type %T, expected string or error", expected[0])
		}
	}

	for _, err := range me {
		if ShouldErrLike(err, expected[0]) == "" {
			return ""
		}
	}
	return fmt.Sprintf("expected MultiError to contain %q", expected[0])
}

// ShouldErrLike compares an `error` or `string` on the left side, to an `error`
// or `string` on the right side.
//
// If the righthand side is omitted, this expects `actual` to be nil.
//
// If a singular righthand side is provided, this expects the stringified
// `actual` to contain the stringified `expected[0]` to be a substring of it.
//
// Example:
//   // Usage                          Equivalent To
//   So(err, ShouldErrLike, "custom")    // `err.Error()` ShouldContainSubstring "custom"
//   So(err, ShouldErrLike, io.EOF)      // `err.Error()` ShouldContainSubstring io.EOF.Error()
//   So(err, ShouldErrLike, "EOF")       // `err.Error()` ShouldContainSubstring "EOF"
//   So(nilErr, ShouldErrLike)           // nilErr ShouldBeNil
//   So(nilErr, ShouldErrLike, nil)      // nilErr ShouldBeNil
//   So(nonNilErr, ShouldErrLike, "")    // nonNilErr ShouldNotBeNil
func ShouldErrLike(actual interface{}, expected ...interface{}) string {
	if len(expected) == 0 {
		return assertions.ShouldBeNil(actual)
	}
	if len(expected) != 1 {
		return fmt.Sprintf("ShouldErrLike requires 0 or 1 expected value, got %d", len(expected))
	}

	if expected[0] == nil {
		return assertions.ShouldBeNil(actual)
	} else if actual == nil {
		return assertions.ShouldNotBeNil(actual)
	}

	ae, ok := actual.(error)
	if !ok {
		return assertions.ShouldImplement(actual, (*error)(nil))
	}

	switch x := expected[0].(type) {
	case string:
		return assertions.ShouldContainSubstring(ae.Error(), x)
	case error:
		return assertions.ShouldContainSubstring(ae.Error(), x.Error())
	}
	return fmt.Sprintf("unexpected argument type %T, expected string or error", expected[0])
}

// ShouldPanicLike is the same as ShouldErrLike, but with the exception that it
// takes a panic'ing func() as its first argument, instead of the error itself.
func ShouldPanicLike(function interface{}, expected ...interface{}) (ret string) {
	f, ok := function.(func())
	if !ok {
		return fmt.Sprintf("unexpected argument type %T, expected `func()`", function)
	}
	defer func() {
		ret = ShouldErrLike(recover(), expected...)
	}()
	f()
	return ShouldErrLike(nil, expected...)
}

// ShouldUnwrapTo asserts that an error, when unwrapped, equals another error.
//
// The actual field will be unwrapped using errors.Unwrap and then compared to
// the error in expected.
func ShouldUnwrapTo(actual interface{}, expected ...interface{}) string {
	act, ok := actual.(error)
	if !ok {
		return fmt.Sprintf("ShouldUnwrapTo requires an error actual type, got %T", act)
	}

	if len(expected) != 1 {
		return fmt.Sprintf("ShouldUnwrapTo requires exactly one expected value, got %d", len(expected))
	}
	exp, ok := expected[0].(error)
	if !ok {
		return fmt.Sprintf("ShouldUnwrapTo requires an error expected type, got %T", expected[0])
	}

	return assertions.ShouldEqual(errors.Unwrap(act), exp)
}
