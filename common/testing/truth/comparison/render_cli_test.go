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
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/internal/typed"
	"go.chromium.org/luci/common/testing/truth/failure"
)

func TestRenderFinding(t *testing.T) {
	t.Parallel()

	type caseT struct {
		prefix   string
		render   RenderCLI
		input    *failure.Finding
		expected []string
	}
	testCase := func(tt caseT) func(t *testing.T) {
		return func(t *testing.T) {
			t.Parallel()
			t.Helper()
			actual := tt.render.Finding(tt.prefix, tt.input)

			if diff := typed.Diff(strings.Join(tt.expected, "\n"), actual); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		}
	}

	t.Run("no lines", testCase(caseT{
		prefix: "  ",
		input: &failure.Finding{
			Name: "Because",
		},
		expected: []string{
			`  Because [no value]`,
		},
	}))

	t.Run("empty oneline", testCase(caseT{
		prefix: "  ",
		input: &failure.Finding{
			Name:  "Because",
			Value: []string{""},
		},
		expected: []string{
			`  Because [blank one-line value]`,
		},
	}))

	t.Run("oneline", testCase(caseT{
		prefix: "  ",
		input: &failure.Finding{
			Name: "Because",
			Value: []string{
				`meepmorp`,
			},
		},
		expected: []string{
			`  Because: meepmorp`,
		},
	}))

	t.Run("multiline", testCase(caseT{
		prefix: "  ",
		input: &failure.Finding{
			Name: "Because",
			Value: []string{
				`here`,
				`are`,
				`some`,
				`lines`,
			},
		},
		expected: []string{
			`  Because: \`,
			`    here`,
			`    are`,
			`    some`,
			`    lines`,
		},
	}))
}

func TestRenderCLIFinding(t *testing.T) {
	t.Parallel()

	type caseT struct {
		render   RenderCLI
		input    *failure.Finding
		expected []string
	}
	testCase := func(tt caseT) func(t *testing.T) {
		return func(t *testing.T) {
			t.Parallel()
			t.Helper()
			actual := tt.render.Finding("", tt.input)

			if diff := typed.Diff(strings.Join(tt.expected, "\n"), actual); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		}
	}

	t.Run("basic", testCase(caseT{
		input: &failure.Finding{
			Name: "Because",
		},
		expected: []string{
			`Because [no value]`,
		},
	}))

	t.Run("long value", testCase(caseT{
		input: &failure.Finding{
			Name: "Because",
			Value: []string{
				strings.Repeat("hi", 30),
			},
			Level: failure.FindingLogLevel_Warn,
		},
		expected: []string{
			`Because [verbose value len=60 (pass -v to see)]`,
		},
	}))

	t.Run("long value", testCase(caseT{
		render: RenderCLI{
			Verbose: true,
		},
		input: &failure.Finding{
			Name: "Because",
			Value: []string{
				strings.Repeat("hi", 30),
			},
		},
		expected: []string{
			`Because: hihihihihihihihihihihihihihihihihihihihihihihihihihihihihihi`,
		},
	}))

	t.Run("color cmp.Diff", testCase(caseT{
		render: RenderCLI{
			Colorize: true,
		},
		input: &failure.Finding{
			Name: "Because",
			Value: []string{
				"strings.Join({",
				"  \t... // 872 identical bytes",
				"  \t\"XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX\",",
				"  \t\"XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX\",",
				"- \t\"arg\",",
				"+ \t\"B\",",
				"  \t\"XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX\",",
				"  \t\"XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX\",",
				"  \t... // 872 identical bytes",
				"}, \"\")",
			},
			Type: failure.FindingTypeHint_CmpDiff,
		},
		expected: []string{
			"Because: \\",
			"    strings.Join({",
			"      \t... // 872 identical bytes",
			"      \t\"XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX\",",
			"      \t\"XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX\",",
			"    \x1b[0;32m- \t\"arg\",\x1b[0m",
			"    \x1b[0;31m+ \t\"B\",\x1b[0m",
			"      \t\"XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX\",",
			"      \t\"XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX\",",
			"      \t... // 872 identical bytes",
			"    }, \"\")",
		},
	}))

	t.Run("color unified diff", testCase(caseT{
		render: RenderCLI{
			Colorize: true,
		},
		input: &failure.Finding{
			Name: "Because",
			Value: []string{
				"--- lao\t2002-02-21 23:30:39.942229878 -0800",
				"+++ tzu\t2002-02-21 23:30:50.442260588 -0800",
				"@@ -1,7 +1,6 @@",
				"-The Way that can be told of is not the eternal Way;",
				"-The name that can be named is not the eternal name.",
				" The Nameless is the origin of Heaven and Earth;",
				"-The Named is the mother of all things.",
				"+The named is the mother of all things.",
				"+",
			},
			Type: failure.FindingTypeHint_UnifiedDiff,
		},
		expected: []string{
			"Because: \\",
			"    \x1b[0;92m--- lao\t2002-02-21 23:30:39.942229878 -0800\x1b[0m",
			"    \x1b[0;91m+++ tzu\t2002-02-21 23:30:50.442260588 -0800\x1b[0m",
			"    \x1b[0;31m@@ -1,7 +1,6 @@\x1b[0m",
			"    \x1b[0;32m-The Way that can be told of is not the eternal Way;\x1b[0m",
			"    \x1b[0;32m-The name that can be named is not the eternal name.\x1b[0m",
			"     The Nameless is the origin of Heaven and Earth;",
			"    \x1b[0;32m-The Named is the mother of all things.\x1b[0m",
			"    \x1b[0;31m+The named is the mother of all things.\x1b[0m",
			"    \x1b[0;31m+\x1b[0m",
		},
	}))
}

func TestRenderCLIFailureComparison(t *testing.T) {
	type caseT struct {
		render   RenderCLI
		input    *failure.Comparison
		expected []string
	}
	testCase := func(tt caseT) func(t *testing.T) {
		return func(t *testing.T) {
			t.Parallel()
			t.Helper()
			actual := tt.render.Comparison("", tt.input)

			if diff := typed.Diff(strings.Join(tt.expected, "\n"), actual); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		}
	}

	t.Run("basic", testCase(caseT{
		input: &failure.Comparison{
			Name: "basic",
		},
		expected: []string{`basic`},
	}))

	t.Run("basic (type args)", testCase(caseT{
		input: &failure.Comparison{
			Name:          "basic",
			TypeArguments: []string{"string", "int"},
		},
		expected: []string{`basic[string, int]`},
	}))

	t.Run("basic (args)", testCase(caseT{
		input: &failure.Comparison{
			Name:      "basic",
			Arguments: []string{"1", `"yo"`},
		},
		expected: []string{`basic(1, "yo")`},
	}))

	t.Run("basic (type args, args)", testCase(caseT{
		input: &failure.Comparison{
			Name:      "basic",
			Arguments: []string{"1", `"yo"`},
		},
		expected: []string{`basic(1, "yo")`},
	}))
}

func TestRenderCLIStack(t *testing.T) {
	type caseT struct {
		render   RenderCLI
		input    *failure.Stack
		expected []string
	}
	testCase := func(tt caseT) func(t *testing.T) {
		return func(t *testing.T) {
			t.Parallel()
			t.Helper()
			actual := tt.render.Stack("", tt.input)
			if diff := typed.Diff(strings.Join(tt.expected, "\n"), actual); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		}
	}

	t.Run("basic (no frames)", testCase(caseT{
		input: &failure.Stack{
			Name: "at",
		},
	}))

	t.Run("basic (one frame)", testCase(caseT{
		input: &failure.Stack{
			Name: "at",
			Frames: []*failure.Stack_Frame{
				{Filename: "some/path.go", Lineno: 100},
			},
		},
		expected: []string{
			"(at path.go:100)",
		},
	}))

	t.Run("basic (multi-frame)", testCase(caseT{
		input: &failure.Stack{
			Name: "at",
			Frames: []*failure.Stack_Frame{
				{Filename: "some/path.go", Lineno: 100},
				{Filename: "other/place.go", Lineno: 77},
			},
		},
		expected: []string{
			"at:",
			"    path.go:100",
			"    place.go:77",
		},
	}))

	t.Run("basic (multi-frame, fullpath)", testCase(caseT{
		render: RenderCLI{FullFilenames: true},
		input: &failure.Stack{
			Name: "at",
			Frames: []*failure.Stack_Frame{
				{Filename: "some/path.go", Lineno: 100},
				{Filename: "other/place.go", Lineno: 77},
			},
		},
		expected: []string{
			"at:",
			"    some/path.go:100",
			"    other/place.go:77",
		},
	}))
}

func TestRenderCLISummary(t *testing.T) {
	type caseT struct {
		render   RenderCLI
		input    *failure.Summary
		expected []string
	}
	testCase := func(tt caseT) func(t *testing.T) {
		return func(t *testing.T) {
			t.Parallel()
			t.Helper()
			actual := tt.render.Summary("", tt.input)
			if diff := typed.Diff(strings.Join(tt.expected, "\n"), actual); diff != "" {
				t.Errorf("unexpected diff (-want +got): %s", diff)
			}
		}
	}

	t.Run("basic", testCase(caseT{
		input: &failure.Summary{
			Comparison: &failure.Comparison{Name: "test", TypeArguments: []string{"int"}, Arguments: []string{"10"}},
			SourceContext: []*failure.Stack{
				{Name: "at", Frames: []*failure.Stack_Frame{{Filename: "source.go", Lineno: 13}}},
			},
			Findings: []*failure.Finding{
				{Name: "Because", Value: []string{"reasons"}},
			},
		},
		expected: []string{
			`test[int](10) FAILED`,
			`(at source.go:13)`,
			`Because: reasons`,
		},
	}))
}
