// Copyright 2017 The LUCI Authors.
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

package python

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"go.chromium.org/luci/common/system/environ"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

const testGetVersionENV = "_VPYTHON_TEST_GET_VERSION"

// Setup for TestInterpreterGetVersion.
func init() {
	if v, ok := os.LookupEnv(testGetVersionENV); ok {
		rc := 0
		if _, err := os.Stdout.WriteString(v); err != nil {
			rc = 1
		}
		os.Exit(rc)
	}
}

func TestInterpreter(t *testing.T) {
	t.Parallel()

	self, err := os.Executable()
	if err != nil {
		t.Fatalf("failed to get executable name: %s", err)
	}

	Convey(`A Python interpreter`, t, func() {
		c := context.Background()

		i := Interpreter{
			Python: self,
		}

		Convey(`Testing IsolatedCommand`, func() {
			cmd := i.MkIsolatedCommand(c, NoTarget{}, "foo", "bar")
			defer cmd.Cleanup()
			So(cmd.Args, ShouldResemble, []string{self, "-B", "-E", "-s", "foo", "bar"})
		})

	})
}

func TestInterpreterGetVersion(t *testing.T) {
	t.Parallel()

	// For this test, we use our test binary as the executable and install an
	// "init" hook to get it to dump its version.
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("failed to get executable: %s", err)
	}
	versionString := ""

	versionSuccesses := []struct {
		output string
		vers   Version
	}{
		{"2.7.1", Version{2, 7, 1}},
		{"2.7.1+local string", Version{2, 7, 1}},
		{"3", Version{3, 0, 0}},
		{"3.1.2.3.4", Version{3, 1, 2}},
	}

	versionFailures := []struct {
		output string
		err    string
	}{
		{"", "unknown version output"},
		{"wat", "non-canonical Python version string: \"wat\""},
	}

	Convey(`Testing Interpreter.GetVersion`, t, func() {
		c := context.Background()

		i := Interpreter{
			Python: self,

			fileCacheDisabled: true,
			testCommandHook: func(cmd *exec.Cmd) {
				env := environ.New(nil)
				env.Set(testGetVersionENV, versionString)
				cmd.Env = env.Sorted()
			},
		}

		for _, tc := range versionSuccesses {
			Convey(fmt.Sprintf(`Can successfully parse %q => %s`, tc.output, tc.vers), func() {
				versionString = tc.output

				vers, err := i.GetVersion(c)
				So(err, ShouldBeNil)
				So(vers, ShouldResemble, tc.vers)

				cachedVers, err := i.GetVersion(c)
				So(err, ShouldBeNil)
				So(cachedVers, ShouldResemble, vers)
			})
		}

		for _, tc := range versionFailures {
			Convey(fmt.Sprintf(`Will fail to parse %q (%s)`, tc.output, tc.err), func() {
				versionString = tc.output

				_, err := i.GetVersion(c)
				So(err, ShouldErrLike, tc.err)
			})
		}
	})
}

func TestIsolateEnvironment(t *testing.T) {
	t.Parallel()

	Convey(`Testing IsolateEnvironment`, t, func() {
		Convey(`Will return nil if the environment is nil.`, func() {
			So(func() { IsolateEnvironment(nil, false) }, ShouldNotPanic)
		})

		Convey(`Will remove environment variables`, func() {
			env := environ.New([]string{"FOO=", "BAR=", "PYTHONPATH=a:b:c", "PYTHONHOME=/foo/bar", "PYTHONNOUSERSITE=0"})

			Convey(`When keepPythonPath is false`, func() {
				IsolateEnvironment(&env, false)
				So(env.Sorted(), ShouldResemble, []string{"BAR=", "FOO=", "PYTHONNOUSERSITE=1"})
			})

			Convey(`When keepPythonPath is true`, func() {
				IsolateEnvironment(&env, true)
				So(env.Sorted(), ShouldResemble, []string{"BAR=", "FOO=", "PYTHONNOUSERSITE=1", "PYTHONPATH=a:b:c"})
			})
		})
	})
}
