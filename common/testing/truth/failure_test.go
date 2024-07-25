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

package truth_test

import (
	"flag"
	"os"
	"os/exec"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/should"
)

var runFailureTests = flag.Bool("assert.run-failure-tests", false, "If set, run failure tests.")

func maybeSkipFailureTests(t testing.TB) {
	if !*runFailureTests {
		t.SkipNow()
	}
}

func TestAllFailures(t *testing.T) {
	me, err := os.Executable()
	if err != nil {
		panic("cannot resolve test executable")
	}

	// NOTE: it would be cool if there was a way that these could count towards
	// coverage, but as of Go1.22 there is no easy way to do this. However, the
	// design of assert.That & friends makes it easy to cover them with unit tests.
	//
	// This test is just to make sure that the fake roughly aligns with reality,
	// and that assert.That/check.That/etc. will correctly fail the tests.
	testList, err := exec.Command(me, "-test.list", "TestFailure_").CombinedOutput()
	testListRaw := string(testList)
	if err != nil {
		t.Fatal("failed to list tests", testListRaw)
	}
	failureTests := strings.Split(testListRaw, "\n")
	failureTests = failureTests[:len(failureTests)-1] // trim off last line

	for _, name := range failureTests {
		t.Run(name, func(t *testing.T) {
			out, err := exec.Command(me, "-test.run", name, "-assert.run-failure-tests").CombinedOutput()
			t.Log("output: ", string(out))
			switch err.(type) {
			case nil:
				t.Fatal("test case failed to exit 1")
			case *exec.ExitError:
				// pass
			default:
				t.Error("unknown error", err)
			}
		})
	}
}

func TestFailure_AssertSimple(t *testing.T) {
	maybeSkipFailureTests(t)

	assert.That(t, 100, should.Equal(200))
}

func TestFailure_AssertLong(t *testing.T) {
	maybeSkipFailureTests(t)

	a := strings.Repeat("X", 1000) + "woat" + strings.Repeat("X", 1000)
	b := strings.Repeat("X", 1000) + "merp" + strings.Repeat("X", 1000)

	assert.That(t, a, should.Equal(b))
}

func TestFailure_MultiCheck(t *testing.T) {
	maybeSkipFailureTests(t)

	check.That(t, 100, should.Equal(20))
	check.That(t, 100, should.Equal(100))
	check.That(t, 100, should.Equal(200))
}

func TestFailure_BadConversion(t *testing.T) {
	maybeSkipFailureTests(t)

	check.Loosely(t, 100, should.Equal("hello"))
	check.Loosely(t, uint8(100), should.Equal(100)) // passes
	check.Loosely(t, 10000, should.Equal(uint8(100)))
}
