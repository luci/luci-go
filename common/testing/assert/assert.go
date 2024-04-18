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

// Package assert implements an extensible, simple, assertion library for Go
// with minimal dependencies.
//
// # Why have an assertion library at all?
//
// While it is recommended to use 'stdlib style' for Go tests, we have found
// that approach to be lacking in the following ways:
//
//  1. Writing good error messages is difficult; when an assertion fails,
//     the error message needs to indicate why. As the error messages get more
//     complex, you may be tempted to write helpers... at which point you've
//     built an assertion library.
//  2. If you refuse to put the helpers in a library, you now have one custom
//     library per package, which is worse.
//  3. If you ignore the problem, you may be tempted to write shorter error
//     messages, at which point test failures become more cumbersome to debug.
//
// So, for our applications, we've found them to be helpful tools to write high
// quality tests. This library is NOT a requirement, but a tool. If it gets in
// the way, don't use it (or, perhaps better, improve it).
//
// # Why not X?
//
// At the time of writing, Go generics were relatively new, and no assertion
// libraries had adopted them in a meaningful way to make compilers do the
// legwork to make sure all the types lined up correctly when possible.
//
// One of the really bad things about early assertion libraries was that they
// were almost always forced to use `interface{}` (a.k.a. `any`) for inputs, and
// extensive amounts of "reflect" based code.
//
// This made the APIs of such assertion libraries more difficult to grok for
// readers, and meant that a large class of assertion failures only showed up at
// runtime, which was unfortunate.
//
// While this library does still do some runtime type reflection (to convert
// from `actual any` to `T` for the given Comparison), this conversion is done
// in exactly one place (this package), and does not require each comparison to
// do this.
//
// # Why now, and why this style?
//
// At the time this library was written, our codebase had a large amount of testing
// code written with `github.com/smartystreets/goconvey/convey` which is a "BDD
// style" testing framework (sort of). We liked the assertion syntax well enough
// to emulate it here; in that framework assertions look like:
//
//	So(actualValue, ShouldResemble, expectedValue)
//	So(somePointer, ShouldBeNil)
//	So(aString, ShouldBeOneOf, "a", "b", "c")
//
// However, this framework had the problem that assertion functions are
// difficult to document (since their signature is always
// `func(any, ...any) string`), had an extra implementation burden for
// implementing custom checkers (every implementation, even small helpers inside
// of a test package, had to do type-casting on the expected arguments, ensure
// that the right number of expected values, etc.).
//
// Further, the return type of `string` is also dissatisfyingly untyped... there
// were global variables which could manipulate the assertions so that they
// returned encoded JSON instead of a plain string message.
//
// For goconvey assertions, you also had to also use the controversial "Convey"
// suite syntax (see the sister library `ftt` adjacent to `assert`, which
// implements the test layout/format without the "BDD style" flavoring).
// This `assert` library has no such restriction.
//
// This library is a way for us to provide high quality assertion replacements
// for the So assertions of goconvey.
//
// # Usage
//
//	import (
//	  "testing"
//
//	  // Exports EXACTLY two symbols, Assert and Check.
//	  . "go.chromium.org/luci/common/testing/assert"
//
//	  // Optional; these are a collection of useful common comparisons, but
//	  // are by no means required.
//	  "go.chromium.org/luci/common/testing/assert/should"
//	)
//
//	func TestSomething(t *testing.T) {
//	   // Checks that `someFunction` returns some value assignable to `int`.
//	   // which equals 100.
//	   Assert(t, someFunction(), should.Equal(100))
//
//	   // Checks that `someFunction` returns some value assignable to `int8`
//	   // which equals 100.
//	   Assert(t, someFunction(), should.Equal[int8](100))
//
//	   // Checks that `someFunction` returns some value assignable to
//	   // `*someStruct` which is populated in the same way.
//	   //
//	   // NOTE: should.Resemble correctly handles comparisons between protobufs
//	   // and types containing protobufs, by default.
//	   Assert(t, someFunctionReturningStruct(), should.Resemble(&someStruct{
//	     ...
//	   }))
//	}
package assert

import (
	"reflect"

	"go.chromium.org/luci/common/data"
	"go.chromium.org/luci/common/testing/assert/interfaces"
	"go.chromium.org/luci/common/testing/assert/results"
)

// Assert compares `actual` using `compare`, which is typically a closure over some
// expected value.
//
// If `comparison` returns a non-nil Result, this logs it and calls t.FailNow().
//
// `actual` will be converted to T using the function
// [go.chromium.org/luci/common/data.LosslessConvertTo].
//
// If this conversion fails, a descriptive error will be logged and FailNow()
// called.
//
// `testingTB` is an interface which is a subset of testing.TB, but is
// unexported to allow this package to be cleanly .-imported.
func Assert[T any](t interfaces.TestingTB, actual any, compare results.Comparison[T]) {
	t.Helper()

	if !Check[T](t, actual, compare) {
		t.FailNow()
	}
}

// Check compares `actual` using `compare`, which is typically a closure over some
// expected value.
//
// If `comparison` returns a non-nil Result, this logs it and calls t.Fail(),
// returning true iff the comparison was successful.
//
// `actual` will be converted to T using the function
// [go.chromium.org/luci/common/data.LosslessConvertTo].
//
// `testingTB` is an interface which is a subset of testing.TB, but is
// unexported to allow this package to be cleanly .-imported.
func Check[T any](t interfaces.TestingTB, actual any, compare results.Comparison[T]) bool {
	t.Helper()

	actualTyped, ok := data.LosslessConvertTo[T](actual)
	var result *results.Result
	if !ok {
		result = results.NewResultBuilder().
			SetName("builtin.LosslessConvertTo", reflect.TypeOf(&actualTyped)).
			Result()
	} else {
		result = compare(actualTyped)
	}

	if result != nil {
		for _, line := range result.Render() {
			t.Log(line)
		}
		t.Fail()
		return false
	}
	return true
}
