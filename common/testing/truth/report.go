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

// Package truth implements an extensible, simple, assertion library for Go
// with minimal dependencies.
//
// # Why have an assertion library at all?
//
// While it is recommended to not use assertion libraries for Go
// tests, we have found that approach to be lacking in the following
// ways:
//
//  1. Writing good error messages is difficult; when an assertion fails,
//     the error message needs to indicate why. As the error messages get more
//     complex, you may be tempted to write helpers... at which point you've
//     built an assertion library.
//  2. If you refuse to put the helpers in a library, you now have one or more
//     custom functions per package, which is worse.
//  3. If you ignore the problem, you may be tempted to write shorter error
//     messages (or fewer tests), which makes test failures more cumbersome
//     to debug.
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
// were almost always forced to use `any` for inputs, and extensive amounts of
// "reflect" based code.
//
// This made the APIs of such assertion libraries more difficult to grok for
// readers, and meant that a large class of assertion failures only showed up at
// runtime, which was unfortunate.
//
// While this library does still do some runtime type reflection (to convert
// from `actual any` to `T` for the given Comparison in assert.Loosely and
// check.Loosely), this conversion is done in exactly one place (this package),
// and does not require each comparison to reimplement this. In addition, the
// default symbols assert.That and check.That do no dynamic type inference at
// all, which should hopefully encourage test authors to follow this stricter
// style by default.
//
// # Why now, and why this style?
//
// At the time this library was written, our codebase had a large amount of testing
// code written with `github.com/smartystreets/goconvey/convey` which is a "BDD
// style" testing framework (sort of). We liked the assertion syntax well enough
// to emulate it here; in convey assertions looked like:
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
// that the right number of expected values were passed, etc.).
//
// Further, the return type of `string` is also dissatisfyingly untyped... there
// were global variables which could manipulate the goconvey assertions so that
// they returned encoded JSON instead of a plain string message, but this was
// even more surprising and dissatisfying.
//
// For goconvey assertions, you also had to also use the controversial "Convey"
// suite syntax (see the sister library `ftt` adjacent to `truth`, which
// implements the test layout/format without the "BDD style" flavoring).
// This `assert` library has no such restriction and works directly with
// anything implementing testing.TB.
//
// This library is a way for us to provide high quality assertion replacements
// for the So assertions of goconvey.
//
// # Usage
//
//	import (
//	  "testing"
//
//	  "go.chromium.org/luci/common/testing/truth/assert"
//
//	  // check makes non-fatal assertions (t.Fail()), in opposition to assert
//	  // which makes fatal assertions (t.FailNow()).
//	  "go.chromium.org/luci/common/testing/truth/check"
//
//	  // Optional; these are a collection of useful, common, comparisons, but
//	  // which are not required.
//	  "go.chromium.org/luci/common/testing/truth/should"
//
//	  // Optional; this library will let you write your own comparison functions
//	  // "go.chromium.org/luci/common/testing/truth/comparison"
//	)
//
//	func TestSomething(t *testing.T) {
//	   // This check will fail, but doesn't immediately stop the test.
//	   check.That(t, 100, should.Equal(200))
//
//	   // Checks that `someFunction` returns an `int` with the value `100`.
//	   // The type of someFunction() is enforced at compile time.
//	   assert.That(t, someFunction(), should.Equal(100))
//
//	   // Checks that `someFunction` returns some value lossesly assignable to
//	   // `int8` which equals 100.
//	   assert.Loosely(t, someOtherFunction(), should.Equal[int8](100))
//
//	   // Checks that `someFunction` returns some value assignable to
//	   // `*someStruct` which is populated in the same way.
//	   //
//	   // NOTE: should.Resemble correctly handles comparisons between protobufs
//	   // and types containing protobufs, by default, using the excellent
//	   // `github.com/google/go-cmp/cmp` library under the hood for comparisons.
//	   assert.That(t, someFunctionReturningStruct(), should.Resemble(&someStruct{
//	     ...
//	   }))
//	}
package truth

import (
	"os"
	"testing"

	"golang.org/x/term"

	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
)

// Verbose indicates that the truth library should always render verbose
// Findings in comparison Failures.
//
// See the example test for a quick "how to use this".
//
// By default this is true when the '-test.v' flag is passed to the binary (this
// is what gets set on the binary when you run `go test -v`).
//
// You may override this in your TestMain function.
var Verbose bool

// Colorize indicates that the truth library should colorize its output (used
// to highlight diffs produced by some comparison functions, such as
// should.Resemble).
//
// By default this is true when Stdout is connected to a terminal.
//
// You may override this in your TestMain function.
var Colorize bool

// FullSourceContextFilenames indicates that the truth library should print out
// full file paths for any SourceContexts in failure.Summaries.
//
// By default, this is true when the '-test.fullpath' flag is passed to the
// binary (this is what gets set on the binary when you run `go test
// -fullpath`).
//
// You may override this in your TestMain function.
var FullSourceContextFilenames bool

func init() {
	Colorize = term.IsTerminal(int(os.Stdout.Fd()))

	// We tried to do this the right way, but were thwarted:
	//
	// Strike 1: Use testing.Verbose()
	// (ignore for a moment that testing does not expose fullpath).
	//
	// Unfortunately, this will panic if the flags have not yet been parsed (which
	// is reasonable), but flag parsing happens in TestMain, meaning that we
	// cannot initialize these from our own init() (which would run before
	// TestMain).
	//
	// Strike 2: Re-define flags "-test.v" and "-test.fullpath" on flag.CommandLine
	// Unfortunately, flag does not allow this, and will panic.
	//
	// Strike 3: Use flag.CommandLine.Lookup plus a sync.Once
	// Ok, so we could look up the '-test.fullpath' and '-test.v' flags in the
	// global command. We still know that we can't actually initialize these in
	// init(), so we'll use sync.Once to pick these up once in render().
	//
	// Unfortunately this means that users attempting to set these values in
	// TestMain will then not be able to affect them, meaning that these module
	// booleans would need to be something like *bool. Meh.
	//
	// Strike 4: Require all tests using this to call truth.Initialize() or
	// similar.
	// These features are so niche that it's unlikely that anyone will remember to
	// do this, so they will just always be `false`. Meh.
	//
	// Strike 5: Even if someone wanted to use one of these as a positional
	// argument... eh.
	//
	// This is technically possible, but would be extremely convoluted, and likely
	// to be a bad idea for other reasons.

	var foundFP bool
	var foundV bool
	for _, val := range os.Args {
		if !foundFP && (val == "-test.fullpath" || val == "-test.fullpath=true") {
			FullSourceContextFilenames = true
			foundFP = true
		} else if !foundV && (val == "-test.v" || val == "-test.v=true") {
			Verbose = true
			foundV = true
		}
		if foundFP && foundV {
			break
		}
	}
}

func render(f *failure.Summary) string {
	return comparison.RenderCLI{
		Verbose:       Verbose,
		Colorize:      Colorize,
		FullFilenames: FullSourceContextFilenames,
	}.Summary("", f)
}

// Report checks that there is no failure (i.e. `failure` is nil).
//
// If failure is not nil, this will Log the error with t.Log, using `name` as a prefix.
//
// Note: The recommended way to use the truth library is to import the `assert`
// and/or `check` sub-packages, which will call into this function. Direct use
// of truth.Report should be very rare.
func Report(t testing.TB, name string, failure *failure.Summary) {
	if failure == nil {
		return
	}
	t.Helper()
	t.Log(name, render(failure))
	return
}
