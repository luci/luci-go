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

package assert

import (
	"strings"

	"go.chromium.org/luci/common/testing/assert/should"
)

func ExampleAssert() {
	// For the example we disable colorization and verbosity to make the Output
	// stable.
	defer disableColorization()()
	defer disableVerbosity()()

	// NOTE: in a real test, you would use `*testing.T` or similar - not something
	// like FakeTB. This will also show filename:lineno in the Output below, but
	// this is omitted for this example.
	t := new(fakeTB)

	Check(t, 100, should.Equal(100)) // OK - no output
	Check(t, 100, should.Equal(102)) // Fails!

	// Does not compile:
	// Check(t, uint8(8), should.Equal(8))
	// Check(t, 100, should.Equal("hello"))

	CheckL(t, 100, should.Equal(100))    // OK
	CheckL(t, uint8(8), should.Equal(8)) // OK, Compiles

	CheckL(t, 100, should.Equal(144))     // Fails!
	CheckL(t, 100, should.Equal("hello")) // Fails!

	// Long outputs automatically get diffed.
	veryLongA := strings.Repeat("X", 1000) + "arg" + strings.Repeat("X", 1000)
	veryLongB := strings.Repeat("X", 1000) + "B" + strings.Repeat("X", 1000)
	Check(t, veryLongA, should.Equal(veryLongB))

	// Output:
	// --- FAIL: FakeTestName (0.00s)
	//     filename.go:NN: Check should.Equal[int] FAILED
	//         Actual: 100
	//         Expected: 102
	//     filename.go:NN: Check should.Equal[int] FAILED
	//         Actual: 100
	//         Expected: 144
	//     filename.go:NN: Check builtin.LosslessConvertTo[string] FAILED
	//         ActualType: int
	//     filename.go:NN: Check should.Equal[string] FAILED
	//         Actual [verbose value len=2005 (pass -v to see)]
	//         Expected [verbose value len=2003 (pass -v to see)]
	//         Diff: \
	//               strings.Join({
	//               	... // 872 identical bytes
	//               	"XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX",
	//               	"XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX",
	//             - 	"arg",
	//             + 	"B",
	//               	"XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX",
	//               	"XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX",
	//               	... // 872 identical bytes
	//               }, "")
}