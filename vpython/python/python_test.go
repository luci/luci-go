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
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func f(flag string, arg ...string) (result CommandLineFlag) {
	result.Flag = flag
	switch len(arg) {
	case 0:
	case 1:
		result.Arg = arg[0]
	default:
		panic(fmt.Errorf("unsupported number of arguments in %v", arg))
	}
	return
}

func TestParseCommandLine(t *testing.T) {
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
				Flags: []CommandLineFlag{f("a"), f("b"), f("Q", "'foo.bar.baz'"), f("X", "'foo.bar.baz'"),
					f("W", "bar")},
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
				Flags:  []CommandLineFlag{f("W", "bar")},
				Args:   []string{"-foo", "-bar"},
			},
			[]string{"-Wbar", "-", "-foo", "-bar"},
		},

		{[]string{"--", "-c", "-foo", "-bar"},
			CommandLine{
				Target: ScriptTarget{"-c"},
				Args:   []string{"-foo", "-bar"},
				Flags:  []CommandLineFlag{f("-")},
			},
			[]string{"--", "-c", "-foo", "-bar"},
		},

		{[]string{"-a", "-Wfoo", "-", "--", "foo"},
			CommandLine{
				Target: ScriptTarget{"-"},
				Flags:  []CommandLineFlag{f("a"), f("W", "foo")},
				Args:   []string{"--", "foo"},
			},
			[]string{"-a", "-Wfoo", "-", "--", "foo"},
		},

		{[]string{"-a", "-b", "-tt", "-W", "foo", "-Wbar", "-c", "<script>", "--", "arg"},
			CommandLine{
				Target: CommandTarget{"<script>"},
				Flags:  []CommandLineFlag{f("a"), f("b"), f("t"), f("t"), f("W", "foo"), f("W", "bar")},
				Args:   []string{"--", "arg"},
			},
			[]string{"-a", "-b", "-t", "-t", "-Wfoo", "-Wbar", "-c", "<script>", "--", "arg"},
		},

		{[]string{"-tt", "-W", "foo", "-Wbar", "-c", "<script>", "-Wbaz", "-v", "--", "arg"},
			CommandLine{
				Target: CommandTarget{"<script>"},
				Flags:  []CommandLineFlag{f("t"), f("t"), f("W", "foo"), f("W", "bar")},
				Args:   []string{"-Wbaz", "-v", "--", "arg"},
			},
			[]string{"-t", "-t", "-Wfoo", "-Wbar", "-c", "<script>", "-Wbaz", "-v", "--", "arg"},
		},

		// NOTE: -W-c is invalid argument to "-W", but it parses.
		{[]string{"-vWfoo", "-vvvW", "-c", "script", "-arg"},
			CommandLine{
				Target: ScriptTarget{"script"},
				Flags:  []CommandLineFlag{f("v"), f("W", "foo"), f("v"), f("v"), f("v"), f("W", "-c")},
				Args:   []string{"-arg"},
			},
			[]string{"-v", "-Wfoo", "-v", "-v", "-v", "-W-c", "script", "-arg"},
		},

		{[]string{"-vOOWfoo", "-Ovv", "-OOvv", "-c", "script", "-arg"},
			CommandLine{
				Target: CommandTarget{"script"},
				Flags: []CommandLineFlag{
					f("v"), f("OO"), f("W", "foo"),
					f("O"), f("v"), f("v"),
					f("OO"), f("v"), f("v"),
				},
				Args: []string{"-arg"},
			},
			[]string{"-v", "-OO", "-Wfoo", "-O", "-v", "-v", "-OO", "-v", "-v", "-c", "script", "-arg"},
		},

		{[]string{"-a", "-b", "-m'foo.bar.baz'", "arg"},
			CommandLine{
				Target: ModuleTarget{"'foo.bar.baz'"},
				Flags:  []CommandLineFlag{f("a"), f("b")},
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

func TestCommandLine(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		desc     string
		flags    []CommandLineFlag
		actionFn func(*CommandLine)
		expected []CommandLineFlag
	}{
		{"Can remove a flag, '-OO'",
			[]CommandLineFlag{f("c"), f("OO"), f("S")},
			func(cl *CommandLine) { cl.RemoveAllFlag("OO") },
			[]CommandLineFlag{f("c"), f("S")},
		},

		{"Can remove all instances of a flag, '-W'",
			[]CommandLineFlag{f("W", "foo"), f("a"), f("b"), f("W"), f("c"), f("W", "bar"),
				f("W", "baz"), f("d"), f("W", "qux")},
			func(cl *CommandLine) { cl.RemoveAllFlag("W") },
			[]CommandLineFlag{f("a"), f("b"), f("c"), f("d")},
		},

		{"Can remove an exact instance of a flag, '-W'",
			[]CommandLineFlag{f("W", "foo"), f("a"), f("b"), f("W"), f("c"), f("W", "bar"),
				f("W", "baz"), f("d"), f("W", "qux")},
			func(cl *CommandLine) { cl.RemoveFlag(CommandLineFlag{"W", "bar"}) },
			[]CommandLineFlag{f("W", "foo"), f("a"), f("b"), f("W"), f("c"),
				f("W", "baz"), f("d"), f("W", "qux")},
		},

		{"Can add a nonexistent flag",
			[]CommandLineFlag{f("v"), f("O")},
			func(cl *CommandLine) { cl.AddSingleFlag("B") },
			[]CommandLineFlag{f("v"), f("O"), f("B")},
		},

		{"Will not add a redundant flag, but will add a unique one",
			[]CommandLineFlag{f("v"), f("O"), f("W", "foo"), f("W")},
			func(cl *CommandLine) {
				cl.AddFlag(CommandLineFlag{"W", "foo"})
				cl.AddSingleFlag("W")
				cl.AddFlag(CommandLineFlag{"W", "bar"})
			},
			[]CommandLineFlag{f("v"), f("O"), f("W", "foo"), f("W"), f("W", "bar")},
		},
	}

	Convey(`Testing CommandLine functionality`, t, func() {
		for i, tc := range testCases {
			Convey(fmt.Sprintf(`Test case #%d: %v`, i, tc.desc), func() {
				cl := CommandLine{
					Flags: tc.flags,
				}
				tc.actionFn(&cl)
				So(cl.Flags, ShouldResemble, tc.expected)
			})
		}
	})
}
