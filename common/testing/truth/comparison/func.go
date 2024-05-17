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

package comparison

import (
	"fmt"
	"runtime"

	"go.chromium.org/luci/common/testing/truth/failure"
)

// Func takes in a value-to-be-compared and returns a failure.Summary if the value
// does not meet the expectation of this comparison.Func.
//
// Example:
//
//	func BeTrue(value bool) *failure.Summary {
//	  if !value {
//	    return comparison.NewSummaryBuilder("should.BeTrue").Summary
//	  }
//	  return nil
//	}
//
// In this example, BeTrue is a comparison.Func.
type Func[T any] func(T) *failure.Summary

// WithLineContext returns a transformed Func to add an "at" SourceContext with
// one frame containing the filename and line number of the frame calling
// WithLineContext, plus skipFrames[0] (if provided).
//
// Example:
//
//	check.That(t, something, should.Equal(100).WithLineContext())
//
// You usually will not need this, but it's very useful when writing a helper
// function for tests (e.g. using t.Helper()) to let you add the location of the
// specific assert inside of the helper function along side the 'top most' frame
// location, as computed directly by the Go testing library.
//
// Example:
//
//	func TestThing(t *testing.T) {
//	  myHelper := func(actual, expected myType) {
//	    t.Helper()  // makes Go 'testing' package skip this to find original call.
//
//	    // We add WithLineContext to these comparisons so that if they fail, we
//	    // will see the file:line within this helper function.
//	    check.That(t, actual.field1, should.Equal(expected.field1).WithLineContext())
//	    check.That(t, actual.field2, should.Equal(expected.field2).WithLineContext())
//	 }
//	 // ...
//	 myHelper(a, expected)
//	}
//
// In this example, the test will output something like:
//
//	--- FAIL: FakeTestName (0.00s)
//	    example_test.go:XX: Check should.Equal[int] FAILED
//	       (at example_test.go:YY)
//	       Actual: 10
//	       Expected: 20
//
// Where XX is the line of the myHelper call, and YY is the line of the actual
// should.Equal check inside of the helper function.
func (cmp Func[T]) WithLineContext(skipFrames ...int) Func[T] {
	if len(skipFrames) > 1 {
		panic(fmt.Errorf(
			"comparison.Func.WithLineContext: skipFrames has more than one value: %v", skipFrames))
	}

	skip := 1
	if len(skipFrames) > 0 {
		skip = 1 + skipFrames[0]
	}
	_, filename, lineno, ok := runtime.Caller(skip)
	if !ok {
		return cmp
	}

	return func(actual T) *failure.Summary {
		ret := cmp(actual)
		if ret != nil {
			ret.SourceContext = append(ret.SourceContext, &failure.Stack{
				Name:   "at",
				Frames: []*failure.Stack_Frame{{Filename: filename, Lineno: int64(lineno)}},
			})
		}
		return ret
	}
}
