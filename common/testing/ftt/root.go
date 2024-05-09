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

import "testing"

// testBuffer is a global hook for testing ftt itself.
//
// testBuffer will cause all internally generated (i.e. by ftt itself)
// t.Error/t.Fatal messages to be redirected to this buffer. This is to allow
// ftt's own tests to evaluate if its logic around failing tests is working
// properly.
//
// It is ALWAYS NIL outside of ftt's own tests.
var testBuffer interface {
	handlePanic()
	Errorf(fmt string, args ...any)
	Fatalf(fmt string, args ...any)
} = nil

// topLevel implements the package level Parallel/Run functions.
//
// Args:
//   - parallel indicates that this function will not block on call, but instead
//     will block at the end of the enclosing test (i.e. Go's `testing` packge
//     will block on it).
func topLevel(name string, t *testing.T, cb func(*Test), parallel testKind) {
	if cb == nil {
		return
	}

	t.Run(name, func(t *testing.T) {
		if parallel {
			t.Parallel()
		}

		// NOTE: testBuffer is always nil outside of ftt's own tests.
		if testBuffer != nil {
			defer testBuffer.handlePanic()
		}

		cb(&Test{
			state: &state{
				rootCb:   cb,
				rootName: name,
			},
			executionState: leaf,
			T:              t,
		})
	})
}

// Parallel starts a new testing suite rooted in `cb` which will run in parallel
// with other sibling Parallel statements within the same test function.
func Parallel(name string, t *testing.T, cb func(*Test)) {
	topLevel(name, t, cb, true)
}

// Run starts a new testing suite rooted in `cb` which will run
// immediately, blocking until `cb` completes.
func Run(name string, t *testing.T, cb func(*Test)) {
	topLevel(name, t, cb, false)
}
