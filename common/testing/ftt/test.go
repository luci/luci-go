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

package ftt

import (
	"strings"
	"testing"
)

type testKind bool

func (t testKind) String() string {
	if t {
		return "Parallel"
	}
	return "Run"
}

// Test is the structure used to maintain `ftt` state.
//
// This fully implements `testing.TB` and directly exposes the underlying
// `*testing.T` to be compatible with testing libraries which need it.
type Test struct {
	// Regular *testing.T which belongs to the currently-running test.
	//
	// Prefer to use `Run()` and `Parallel()` as implemented on Test, rather than
	// on T here, as this will preserve all organizational state for `ftt`.
	*testing.T

	// This is the state of all sub-tests which share the same ftt root.
	state *state

	// Current execution mode, either descending, leaf or ascending.
	// This affects the behavior of Test.Run and Test.Parallel.
	//
	// See documentation for executionState.
	executionState executionState

	// target is the path of Parallel (or Run) `name` arguments that this Test
	// is descending for.
	target []string

	// true if target was created with Parallel vs Run.
	//
	// This is used to print better debugging messages, but otherwise does not
	// affect Test functionality.
	targetParallel testKind

	// targetIdx is the currently found item in target. If this == len(target) it
	// means we found the target Parallel or Run and are currently executing its
	// callback.
	//
	// Said another way, this is our current depth in our ftt tree.
	targetIdx int // counts 0->len(target)

	// found accumulates all the found `name` arguments for a given targetIdx.
	//
	// This is used to help the error report when the user has non-deterministic
	// Parallel `name`s and a non-leaf Parallel callback completes without
	// incrementing targetIdx.
	found []string
}

// handleSuiteInLeaf handles the case where we encounter a Test.Run or
// Test.Parallel while in the `leaf` executionState.
//
// In this case we need to schedule a new test with a target of Test.name+name
// - we don't need the passed callback right now, because the new scheduled Test
// will descend from the root back down to this new target and execute it with
// handleSuiteDescent().
func (t *Test) handleSuiteInLeaf(name string, parallel testKind) {
	t.Helper()

	subTarget := make([]string, len(t.target), len(t.target)+1)
	copy(subTarget, t.target)
	subTarget = append(subTarget, name)

	if t.state.testNames.add(subTarget) {
		t.tFatalf("Found duplicate test suite %q/%s(%q)", strings.Join(t.target, "/"),
			parallel, name)
	}

	t.T.Run(name, func(got *testing.T) {
		if parallel {
			got.Parallel()
		}

		// NOTE: testBuffer is always nil outside of ftt's own tests.
		if testBuffer != nil {
			defer testBuffer.handlePanic()
		}

		t := &Test{
			state:          t.state,
			target:         subTarget,
			targetParallel: parallel,
			executionState: descending,
			T:              got,
		}

		defer func() {
			// if cb() finishes and we are still in the descending state, it means
			// we failed to find the target. We only want to emit this error
			// once though, so we set the state to ascending after emitting the
			// error.
			if t.executionState == descending {
				t.executionState = ascending
				t.tErrorf("Failed to find %q/ftt.%s(%q) - found %s", t.state.rootName,
					t.targetParallel, t.target[0], t.found)
			}
		}()

		t.state.rootCb(t)
	})
}

// handleSuiteDescent handles the case where we encounter a Test.Run or
// Test.Parallel while in the descending state.
//
// There are three cases:
//   - `name` doesn't match our current target[targetIdx] - we ignore this
//     suite.
//   - `name` matches our current target[targetIdx], but this is not the last
//     one in target. We advance targetIdx and execute the callback to look
//     for the next branch in the tree.
//   - `name` matches our current target[targetIdx], and it is the last
//     one in target. We switch the executionState to `leaf` and execute the
//     callback.
//
// This function also handles collection of suite names in the event that we
// cannot find `target` at all, and will emit an error if we descend into `cb`
// but we end up never finding the next branch in the tree.
func (t *Test) handleSuiteDescent(name string, cb func(*Test)) {
	t.Helper()

	if t.target[t.targetIdx] == name {
		// We found the next name in our target, we need to run `cb`.
		observedIdx := t.targetIdx
		t.targetIdx++

		if t.targetIdx == len(t.target) {
			// we found the last link in our target, so set the new state to
			// leaf, and set a defer to set the ascending state after
			// cb() completes.
			t.found = nil
			t.executionState = leaf
			defer func() {
				t.executionState = ascending
			}()
		} else {
			t.found = t.found[:0]
			defer func() {
				// if cb() finishes and we are still in the descending state, it means
				// we failed to find the target. We only want to emit this error
				// once though, so we set the state to ascending after emitting the
				// error.
				if t.executionState == descending {
					t.executionState = ascending
					t.tErrorf("Failed to find %q/ftt.%s(%q) - found %s",
						t.state.rootName+"/"+strings.Join(t.target[:observedIdx+1], "/"),
						t.targetParallel, t.target[observedIdx+1], t.found)
				}
			}()
		}

		cb(t)
	} else {
		t.found = append(t.found, name)
	}
}

func (t *Test) suiteImpl(name string, cb func(*Test), parallel testKind) {
	if cb == nil {
		return
	}

	t.Helper()

	switch t.executionState {
	case ascending:
		// the current call is a sibling suite of one Parallel we executed already while
		// descending - we don't need to do anything.
		return

	case descending:
		t.handleSuiteDescent(name, cb)

	case leaf:
		t.handleSuiteInLeaf(name, parallel)
	}
}

// Parallel runs `cb` as a parallel sub-test which executes all functions from
// TestRoot to this callsite, followed by `cb`.
func (t *Test) Parallel(name string, cb func(*Test)) {
	t.suiteImpl(name, cb, true)
}

// Run runs `cb` as a serial sub-test which executes all functions from
// TestRoot to this callsite, followed by `cb`.
func (t *Test) Run(name string, cb func(*Test)) {
	t.suiteImpl(name, cb, false)
}

func (t *Test) tErrorf(format string, args ...any) {
	t.Helper()
	if testBuffer == nil {
		t.Errorf(format, args...)
		return
	}
	testBuffer.Errorf(format, args...)
}

func (t *Test) tFatalf(format string, args ...any) {
	t.Helper()
	if testBuffer == nil {
		t.Fatalf(format, args...)
		return
	}
	testBuffer.Fatalf(format, args...)
}
