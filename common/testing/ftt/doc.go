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

// Package ftt (aka "from the top") replicates the test exectution order
// semantics of `github.com/smartystreets/goconvey/convey` while fixing some
// notable issues:
//
//   - Required, explicit, testing context (No more dependency on
//     the very-magical https://github.com/jtolio/gls)
//   - Uses testing.T.Run to be compatible with `go test -run <regex>`.
//   - Sub-tests can now run in parallel.
//   - Explicit compatibility goal with `go test`; mixing and matching
//     convey-based and non-convey-based tests in the same test program is fine
//     because this library only colors inside the lines.
//   - Assertions are no longer 'built in'; instead tests can use any assertion
//     library (or none) that they like as long as it works with `*testing.T`.
//     See `go.chromium.org/luci/common/testing/assert` for a typesafe one with
//     low dependencies used in this repo.
//
// This is, however, missing functionality that the original convey package has:
//   - No assertions show up in the `goconvey` UI. At some point we should build
//     a new Web/Terminal UI, but it should ingest the standard test2json output
//     from `go test -json`, not rely on a custom format.
//     Also see https://github.com/golang/go/issues/43936 which could be a way
//     to output assertion metadata sanely.
//   - It is not "BDD-ish" (but we're considering this a feature :)).
//
// # From the top?
//
// FTT tests always execute from the root down to the subtest, skipping all
// other subtests along the way, and enqueuing any new sub-sub tests discovered.
//
// All tests MUST have stable names (the string passed to Run() or Parallel()).
// It is OK to generate the names passed to Run() and Parallel(), as long as the
// generated names are deterministic (but order does not matter).
//
// For example (good):
//
//	import (
//	  "testing"
//	  "go.chromium.org/luci/testing/ftt"
//
//	  // Optional: This just adds the two symbols Assert and Check.
//	  . "go.chromium.org/luci/testing/assert"
//	  // Optional: Comparisons to use with Assert/Check.
//	  "go.chromium.org/luci/testing/assert/should"
//	)
//
//	func TestWidget(t *testing.T) {
//	  t.Parallel()
//
//	  ftt.Parallel("in common context", t, func(t *ftt.Test) {
//	    ctx := initializeCommonContext(...)                              // 1
//
//	    t.Parallel("works", func(t *ftt.Test) {
//	       ctx = goodConfig(ctx)                                         // 2
//
//	       t.Parallel("with defaults", func(t *ftt.Test) {
//	         Assert(t, widget(ctx), should.ErrLike(nil))                 // 3
//	         // Or something like:
//	         // if err := widget(ctx); err != nil {
//	         //   t.Failf("widget did not work: %#v", err)
//	         // }
//	       })
//
//	       t.Parallel("with options", func(t *ftt.Test) {
//	         Assert(t, widget(ctx, &options{...}), should.ErrLike(nil))  // 4
//	         // Or something like:
//	         // if err := widget(ctx, &options{...}); err != nil {
//	         //   t.Failf("widget did not work: %#v", err)
//	         // }
//	       })
//	    })
//
//	    t.Parallel("fails", func(t *ftt.Test) {
//	       ctx = badChanges(ctx)                                         // 5
//	       Assert(t, widget(ctx), should.ErrLike("bad stuff"))           // 6
//
//	       // Or something like:
//	       // err := widget(ctx)
//	       // if err == nil {
//	       //   t.Failf("widget should have failed, but didn't")
//	       // } else if strings.Contains(err.Error(), "bad stuff") {
//	       //   t.Failf(`widget failed with the wrong error: %s (expected it to contain "bad stuff")`)
//	       // }
//	    })
//	  })
//	}
//
// This would run the following tests, each of which executes the (numbered
// statements):
//
//   - TestWidget                      (1)
//     Launches "TestWidget/works"
//     Launches "TestWidget/fails"
//   - TestWidget/works                (1, 2)
//     Launches "TestWidget/works/with_defaults"
//     Launches "TestWidget/works/with_options"
//   - TestWidget/works/with_defaults  (1, 2, 3)
//   - TestWidget/works/with_options   (1, 2, 4)
//   - TestWidget/fails                (1, 5, 6)
//
// An example of generating bad names could be:
//
//	func TestBad(t *testing.T) {
//	  currentName := ""
//
//	  ftt.Run("root", func(t *ftt.Test) {
//	    currentName += "a"
//	    t.Run(currentName, func(t *ftt.Test) { ... })
//	  })
//	}
//
// In this example, it would run `root`, and schedule `root/a`, but then the
// `root/a` test would only see `t.Run("aa", ...)`. This example is clearly
// contrived, but this situation can arise if you're doing something like
// generating random test cases, but you don't generate them the same way on
// every execution of the root callback.
//
// # Test discovery
//
// Test discovery is not doing any source parsing, stack walking, or anything like that.
//
// Each test which runs carries a piece of state which is the "path" through
// the tree of tests, using the names provided to Run/Parallel. The root test
// starts with an empty path.
//
// When executing a test function (and recall that ALL subtests start "from the
// top"), if the execution encounters a t.Run or t.Parallel call:
//   - If the current path is empty, execute that callback, registering any
//     sub-tests immediately found within it.
//   - If the given name matches the first element of the remaining path,
//     consume that element from the remaining path, and execute the callback with
//     the shortened path value. Keep state to ensure that any other sibling
//     t.Run/t.Parallel calls are ignored.
//   - If the given name does not match the first element of the remaining path,
//     ignore it.
//
// In the example from the previous section, TestWidget runs and encounters two
// t.Parallel calls ('works' and 'fails') and runs these sub tests in parallel.
// Each is started with the path state `[]string{"works"}` and
// `[]string{"fails"}` respectively, and will recurse/ignore those sub-tests
// according to the algorithm above.
//
// This continues until no new sub-tests are discovered, and all running tests
// complete.
//
// Within each callback, the *Test struct embeds the `*testing.T` for that
// subtest. This is exactly the same T object you would get when using
// `t.Run(...)`, and you can call any/
//
// # BDD-ish?
//
// Specifically, tests and subtests in goconvey are intended to have "readable"
// names:
//
//	Convey MyThing
//	  Convey works with X
//	    Convey when blah blah
//
// `ftt` intentionally doesn't try to replicate this, because this BDD
// style ended up hiding the important "from the top" functionality that was
// actually going on.
//
// # Terminology in this package
//
// This package has picked a very limited set of symbols:
//
//   - 'Run' - This creates a new sub-test, and executes it serially.
//   - 'Parallel' - This creates a new sub-test, and executes it in parallel.
//   - 'Each' - This consumes a list of inputs.
//
// There are also two variants of Run and Parallel; the package-level
// definitions, and the definitions on the Test type. The package-level symbols
// (ftt.Run and ftt.Parallel) create new "from the top" roots. The methods on
// Test create new tests which run "from the top" (the root in which they are
// contained).
//
// Under the hood these all turn into `testing.T.Run` invocations with, or
// without, a t.Parallel invocation. It is recommended to familiarize yourself
// with how Go sub-tests work on the vanilla testing library to understand the
// semantics and caveats which apply.
//
// # How does it work?
//
// `ftt` structures tests into trees. Every tree starts by calling `ftt.Run` or
// `ftt.Parallel` with a callback taking a *Test (let's call this the 'root
// callback' of the tree). A given Go TestSomething function may have multiple
// ftt roots, but they SHOULD all be immediate children of TestSomething.
//
// As soon as you pass the root callback to ftt, it will spawn a go subtest
// (with the standard `testing.T.Run`), and will keep a 'state' which is shared
// by all subtests under the root callback. As the callback executes, it may
// call Test.Run or Test.Parallel to add a sub-test to the state.
//
// There are three execution modes that an ftt test may be in: descending, leaf,
// or ascending. When ftt runs the root callback for the first time, it always
// starts in the `leaf` state.
//
//   - leaf means that all Test.Run or Test.Parallel calls should register a new
//     sub-test of the current leaf. So if your root is called 'root' and has
//     two Test.Run calls for 'a' and 'b', these will register new subtests
//     called 'root/a' and 'root/b', respectively. These new subtests will start
//     in the 'descending' state by re-executing the root callback. Once the
//     leaf callback exits, we switch to the 'ascending' state.
//
//   - descending means that we should ignore all Test.Run or Test.Parallel
//     calls which do not get us closer to our target branch of the tree. If we
//     start the root callback with a target branch of "root/b", we would ignore
//     the call to Test.Run("a") (it would be a no-op). Once we find the target
//     branch, we switch to the 'leaf' state.
//
//   - ascending means that we ignore all Test.Run and Test.Parallel calls
//     unconditionally. Our target branch of the test tree ran to completion and
//     we're just unwinding back to the end of the root callback. We don't need
//     to register any additional subtests because they will already have been
//     registered by the test which ran the current callback in the leaf state.
//
// Aside from this, there is a bit of bookeeping necessary to detect when we
// completely failed to find our target branch (which could happen if the test
// is generating test names non-deterministically).
package ftt
