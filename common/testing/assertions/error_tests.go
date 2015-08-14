// Copyright (c) 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package assertions

import (
	"fmt"

	"github.com/smartystreets/assertions"
)

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
//   So(nonNilErr, ShouldErrLike, "foo") // nonNilErr ShouldNotBeNil
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
	return fmt.Sprintf("unknown argument type %T, expected string or error", expected[0])
}
