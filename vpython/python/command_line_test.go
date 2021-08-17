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

		// Script target with flag separator.
		{[]string{"path.py", "--", "foo", "bar"},
			CommandLine{
				Target: ScriptTarget{"path.py", false},
				Args:   []string{"--", "foo", "bar"},
			},
			[]string{"path.py", "--", "foo", "bar"},
		},

		// Command target with flag separator.
		{[]string{"-v", "-c", "<script>", "--", "-foo", "-bar"},
			CommandLine{
				Flags:  []CommandLineFlag{f("v")},
				Target: CommandTarget{"<script>"},
				Args:   []string{"--", "-foo", "-bar"},
			},
			[]string{"-v", "-c", "<script>", "--", "-foo", "-bar"},
		},

		// Script target before with flag separator.
		{[]string{"-v", "<script>", "--", "-foo", "-bar"},
			CommandLine{
				Flags:  []CommandLineFlag{f("v")},
				Target: ScriptTarget{"<script>", false},
				Args:   []string{"--", "-foo", "-bar"},
			},
			[]string{"-v", "<script>", "--", "-foo", "-bar"},
		},

		// Script target after flag separator.
		{[]string{"-v", "--", "<script>", "-foo", "-bar"},
			CommandLine{
				Flags:         []CommandLineFlag{f("v")},
				Target:        ScriptTarget{"<script>", true},
				FlagSeparator: true,
				Args:          []string{"-foo", "-bar"},
			},
			[]string{"-v", "--", "<script>", "-foo", "-bar"},
		},

		{[]string{"-v", "-cprint", "foo", "bar"},
			CommandLine{
				Target: CommandTarget{"print"},
				Flags:  []CommandLineFlag{f("v")},
				Args:   []string{"foo", "bar"},
			},
			[]string{"-v", "-c", "print", "foo", "bar"},
		},

		{[]string{"-Wbar", "-", "-foo", "-bar"},
			CommandLine{
				Target: ScriptTarget{"-", false},
				Flags:  []CommandLineFlag{f("W", "bar")},
				Args:   []string{"-foo", "-bar"},
			},
			[]string{"-Wbar", "-", "-foo", "-bar"},
		},

		// NOTE: This will parse the first positional argument, "-c", as the path
		// to the script to run. This is generally not done, but exercises the
		// positional argument code.
		{[]string{"--", "-c", "-foo", "-bar"},
			CommandLine{
				Target:        ScriptTarget{"-c", true},
				Args:          []string{"-foo", "-bar"},
				FlagSeparator: true,
			},
			[]string{"--", "-c", "-foo", "-bar"},
		},

		{[]string{"-a", "-Wfoo", "-", "--", "foo"},
			CommandLine{
				Target: ScriptTarget{"-", false},
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
				Target: ScriptTarget{"script", false},
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

		// First separator signals end of flags, next is first positional argument,
		// which is the path of the script. Remainder are positional arguments to
		// the script.
		{[]string{"--", "--", "--", "--"},
			CommandLine{
				Target:        ScriptTarget{"--", true},
				FlagSeparator: true,
				Args:          []string{"--", "--"},
			},
			[]string{"--", "--", "--", "--"},
		},

		// --version is a special multi-character flag.
		{[]string{"--version"},
			CommandLine{
				Target: NoTarget{},
				Flags:  []CommandLineFlag{f("-version")},
			},
			[]string{"--version"},
		},
	}

	failures := []struct {
		args []string
		err  string
	}{
		{[]string{"-a", "-b", "-Q"}, "two-value flag missing second value"},
		{[]string{"-c"}, "missing second value"},
		{[]string{"-vvm"}, "missing second value"},
		{[]string{"-\x80"}, "invalid rune in flag"},
	}

	Convey(`Testing Python command-line parsing`, t, func() {
		for i, tc := range successes {
			Convey(fmt.Sprintf(`Success case #%d: %v`, i, tc.args), func() {
				cmd, err := ParseCommandLine(tc.args)
				So(err, ShouldBeNil)

				builtArgs := cmd.BuildArgs()
				So(cmd, ShouldResemble, &tc.cmd)
				So(builtArgs, ShouldResemble, tc.build)

				// Round-trip!
				roundTripBuiltArgs := cmd.BuildArgs()
				cmd, err = ParseCommandLine(builtArgs)
				So(err, ShouldBeNil)
				So(cmd, ShouldResemble, &tc.cmd)
				So(roundTripBuiltArgs, ShouldResemble, tc.build)
				So(roundTripBuiltArgs, ShouldResemble, builtArgs)
			})
		}

		for i, tc := range failures {
			Convey(fmt.Sprintf(`Error case #%d: %v`, i, tc.args), func() {
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
			[]CommandLineFlag{f("c"), f("OO"), f("O"), f("S")},
			func(cl *CommandLine) { cl.RemoveAllFlag("OO") },
			[]CommandLineFlag{f("c"), f("O"), f("S")},
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
				cl.AddSingleFlag("OO")
				cl.AddFlag(CommandLineFlag{"W", "bar"})
			},
			[]CommandLineFlag{f("v"), f("O"), f("W", "foo"), f("W"), f("OO"), f("W", "bar")},
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

		Convey(`Testing Clone`, func() {
			// Create a command-line with all fields populated.
			cmd := CommandLine{
				Target: ScriptTarget{"script", false},
				Flags:  []CommandLineFlag{f("OO"), f("v"), f("Q", "warnall")},
				Args:   []string{"foo"},
			}

			clone := cmd.Clone()
			So(clone, ShouldResemble, &cmd)

			clone.Flags = append(clone.Flags[:0], f("B"), f("d"), f("E"), f("H"))
			clone.Args = append(clone.Args[:0], "bar", "baz")
			So(clone, ShouldNotResemble, &cmd)
		})

		Convey(`Adding a flag with a '-' causes a panic.`, func() {
			var cl CommandLine
			So(func() { cl.AddSingleFlag("-") }, ShouldPanic)
			So(func() { cl.AddFlag(f("-W", "all")) }, ShouldPanic)
		})

		Convey(`Adding a flag to a command-line with a flag separator`, func() {
			cl, err := ParseCommandLine([]string{"--", "foo", "bar"})
			So(err, ShouldBeNil)

			cl.AddSingleFlag("B")
			cl.AddSingleFlag("E")

			So(cl.BuildArgs(), ShouldResemble, []string{"-B", "-E", "--", "foo", "bar"})
		})

		Convey(`Setting FlagSeparator with a flag target includes the flag in the proper section.`, func() {
			cl := CommandLine{
				Target:        ModuleTarget{Module: "<module>"},
				FlagSeparator: true,
				Args:          []string{"foo", "bar"},
			}
			So(cl.BuildArgs(), ShouldResemble, []string{"-m", "<module>", "--", "foo", "bar"})
		})
	})
}
