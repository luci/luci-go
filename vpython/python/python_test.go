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

func f(flag rune, arg ...string) (result CommandLineFlag) {
	result.Flag = string(flag)
	switch len(arg) {
	case 0:
	case 1:
		result.Arg = arg[0]
	default:
		panic(fmt.Errorf("unsupported number of arguments in %v", arg))
	}
	return
}

func TestParsePythonCommandLine(t *testing.T) {
	t.Parallel()

	successes := []struct {
		args  []string
		cmd   CommandLine
		build []string
	}{
		{nil, CommandLine{Target: NoTarget{}}, []string{}},

		{[]string{"-a", "-b", "-Q'foo.bar.baz'", "-X", "'foo.bar.baz'", "-Wbar"},
			CommandLine{
				Target: NoTarget{},
				Flags: []CommandLineFlag{f('a'), f('b'), f('Q', "'foo.bar.baz'"), f('X', "'foo.bar.baz'"),
					f('W', "bar")},
			},
			[]string{"-a", "-b", "-Q'foo.bar.baz'", "-X'foo.bar.baz'", "-Wbar"},
		},

		{[]string{"path.py", "--", "foo", "bar"},
			CommandLine{
				Target: ScriptTarget{"path.py"},
				Args:   []string{"--", "foo", "bar"},
			},
			[]string{"path.py", "--", "foo", "bar"},
		},

		{[]string{"-Wbar", "-", "-foo", "-bar"},
			CommandLine{
				Target: ScriptTarget{"-"},
				Flags:  []CommandLineFlag{f('W', "bar")},
				Args:   []string{"-foo", "-bar"},
			},
			[]string{"-Wbar", "-", "-foo", "-bar"},
		},

		{[]string{"--", "-c", "-foo", "-bar"},
			CommandLine{
				Target: ScriptTarget{"-c"},
				Args:   []string{"-foo", "-bar"},
				Flags:  []CommandLineFlag{f('-')},
			},
			[]string{"--", "-c", "-foo", "-bar"},
		},

		{[]string{"-a", "-Wfoo", "-", "--", "foo"},
			CommandLine{
				Target: ScriptTarget{"-"},
				Flags:  []CommandLineFlag{f('a'), f('W', "foo")},
				Args:   []string{"--", "foo"},
			},
			[]string{"-a", "-Wfoo", "-", "--", "foo"},
		},

		{[]string{"-a", "-b", "-tt", "-W", "foo", "-Wbar", "-c", "<script>", "--", "arg"},
			CommandLine{
				Target: CommandTarget{"<script>"},
				Flags:  []CommandLineFlag{f('a'), f('b'), f('t'), f('t'), f('W', "foo"), f('W', "bar")},
				Args:   []string{"--", "arg"},
			},
			[]string{"-a", "-b", "-t", "-t", "-Wfoo", "-Wbar", "-c", "<script>", "--", "arg"},
		},

		{[]string{"-tt", "-W", "foo", "-Wbar", "-c", "<script>", "-Wbaz", "-v", "--", "arg"},
			CommandLine{
				Target: CommandTarget{"<script>"},
				Flags:  []CommandLineFlag{f('t'), f('t'), f('W', "foo"), f('W', "bar")},
				Args:   []string{"-Wbaz", "-v", "--", "arg"},
			},
			[]string{"-t", "-t", "-Wfoo", "-Wbar", "-c", "<script>", "-Wbaz", "-v", "--", "arg"},
		},

		// NOTE: -W-c is invalid argument to "-W", but it parses.
		{[]string{"-vWfoo", "-vvvW", "-c", "script", "-arg"},
			CommandLine{
				Target: ScriptTarget{"script"},
				Flags:  []CommandLineFlag{f('v'), f('W', "foo"), f('v'), f('v'), f('v'), f('W', "-c")},
				Args:   []string{"-arg"},
			},
			[]string{"-v", "-Wfoo", "-v", "-v", "-v", "-W-c", "script", "-arg"},
		},

		{[]string{"-a", "-b", "-m'foo.bar.baz'", "arg"},
			CommandLine{
				Target: ModuleTarget{"'foo.bar.baz'"},
				Flags:  []CommandLineFlag{f('a'), f('b')},
				Args:   []string{"arg"},
			},
			[]string{"-a", "-b", "-m", "'foo.bar.baz'", "arg"},
		},
	}

	failures := []struct {
		args []string
		err  string
	}{
		{[]string{"-a", "-b", "-Q"}, "two-value flag missing second value"},
		{[]string{"-c"}, "missing second value"},
		{[]string{"-\x80"}, "invalid rune in flag"},
	}

	Convey(`Testing Python command-line parsing`, t, func() {
		for _, tc := range successes {
			Convey(fmt.Sprintf(`Success cases: %v`, tc.args), func() {
				cmd, err := ParseCommandLine(tc.args)
				So(err, ShouldBeNil)

				builtArgs := cmd.BuildArgs()
				So(cmd, ShouldResemble, &tc.cmd)
				So(builtArgs, ShouldResemble, tc.build)
			})
		}

		for _, tc := range failures {
			Convey(fmt.Sprintf(`Error cases: %v`, tc.args), func() {
				_, err := ParseCommandLine(tc.args)
				So(err, ShouldErrLike, tc.err)
			})
		}
	})
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
			cmd := i.IsolatedCommand(c, "foo", "bar")
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
