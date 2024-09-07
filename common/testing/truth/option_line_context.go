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

package truth

import (
	"fmt"
	"runtime"

	"go.chromium.org/luci/common/testing/truth/failure"
)

// LineContext returns an Option which adds an "at" SourceContext with
// one frame containing the filename and line number of the frame calling
// LineContext, plus skipFrames[0] (if provided).
//
// Example:
//
//	check.That(t, something, should.Equal(100), truth.LineContext())
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
//	    // We add LineContext to these comparisons so that if they fail, we
//	    // will see the file:line within this helper function.
//	    check.That(t, actual.field1, should.Equal(expected.field1).LineContext())
//	    check.That(t, actual.field2, should.Equal(expected.field2).LineContext())
//	  }
//	  // ...
//	  myHelper(a, expected)
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
func LineContext(skipFrames ...int) Option {
	if len(skipFrames) > 1 {
		panic(fmt.Errorf(
			"truth.LineContext: skipFrames has more than one value: %v", skipFrames))
	}

	skip := 1
	if len(skipFrames) > 0 {
		skip = 1 + skipFrames[0]
	}
	_, filename, lineno, ok := runtime.Caller(skip)
	if !ok {
		return nil
	}

	return ModifySummary(func(s *failure.Summary) {
		s.SourceContext = append(s.SourceContext, &failure.Stack{
			Name:   "at",
			Frames: []*failure.Stack_Frame{{Filename: filename, Lineno: int64(lineno)}},
		})
	})
}
