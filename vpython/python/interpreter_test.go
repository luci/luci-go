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
	"fmt"
	"os"
	"os/exec"
	"testing"

	"go.chromium.org/luci/common/system/environ"

	"golang.org/x/net/context"

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

		Convey(`Testing hash`, func() {
			hash, err := i.Hash()
			So(err, ShouldBeNil)
			So(hash, ShouldNotEqual, "")

			t.Logf("Calculated interpreter hash: %s", hash)
		})

		Convey(`Testing IsolatedCommand`, func() {
			cmd := i.IsolatedCommand(c, NoTarget{}, "foo", "bar")
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
		{"Python 2.7.1\n", Version{2, 7, 1}},
		{"Python 2.7.1+local string", Version{2, 7, 1}},
		{"Python 3", Version{3, 0, 0}},
		{"Python 3.1.2.3.4", Version{3, 1, 2}},
	}

	versionFailures := []struct {
		output string
		err    string
	}{
		{"", "unknown version output"},
		{"Python2.7.11\n", "unknown version output"},
		{"Python", "unknown version output"},
		{"Python 2.7.11 foo bar junk", "non-canonical Python version string"},
	}

	Convey(`Testing Interpreter.GetVersion`, t, func() {
		c := context.Background()

		i := Interpreter{
			Python: self,

			testCommandHook: func(cmd *exec.Cmd) {
				var env environ.Env
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
			So(func() { IsolateEnvironment(nil) }, ShouldNotPanic)
		})

		Convey(`Will remove environment variables`, func() {
			env := environ.New([]string{"FOO=", "BAR=", "PYTHONPATH=a:b:c", "PYTHONHOME=/foo/bar", "PYTHONNOUSERSITE=0"})
			IsolateEnvironment(&env)
			So(env.Sorted(), ShouldResemble, []string{"BAR=", "FOO=", "PYTHONNOUSERSITE=1"})
		})
	})
}
