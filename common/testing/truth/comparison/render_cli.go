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
	"path/filepath"
	"strings"

	"github.com/mgutz/ansi"

	"go.chromium.org/luci/common/testing/truth/failure"
)

type RenderCLI struct {
	// If true, will render all Verbose findings.
	//
	// Otherwise this will print an omission message which describes how long the
	// omitted value is and to pass `-v` to the test to see them.
	Verbose bool

	// If true, will add ANSI color codes to Findings with appropriate types
	// (currently just simple +/- per-line colorization for unified and cmp.Diff
	// Findings).
	Colorize bool

	// If true, will not truncate filenames in any source_context stacks.
	FullFilenames bool
}

// Finding renders a Finding to a set of output lines which would be
// suitable for display as CLI output (e.g. to be logged with testing.T.Log
// calls).
func (r RenderCLI) Finding(prefix string, f *failure.Finding) string {
	if len(f.Value) == 0 {
		return fmt.Sprintf("%s%s [no value]", prefix, f.Name)
	}
	if len(strings.TrimSpace(f.Value[0])) == 0 {
		return fmt.Sprintf("%s%s [blank one-line value]", prefix, f.Name)
	}

	if f.Level > failure.FindingLogLevel_Error && !r.Verbose {
		valLen := len(f.Value) - 1 // one per newline
		for _, line := range f.Value {
			valLen += len(line)
		}
		return fmt.Sprintf("%s%s [verbose value len=%d (pass -v to see)]", prefix, f.Name, valLen)
	}

	if len(f.Value) == 1 {
		return fmt.Sprintf("%s%s: %s", prefix, f.Name, f.Value[0])
	}

	value := make([]string, len(f.Value))
	copy(value, f.Value)
	if r.Colorize {
		switch f.Type {
		case failure.FindingTypeHint_CmpDiff, failure.FindingTypeHint_UnifiedDiff:
			for i, line := range value {
				code := ""
				if strings.HasPrefix(line, "-") {
					code = ansi.Green
					if strings.HasPrefix(line, "--- ") {
						code = ansi.LightGreen
					}
				} else if strings.HasPrefix(line, "+") {
					code = ansi.Red
					if strings.HasPrefix(line, "+++ ") {
						code = ansi.LightRed
					}
				} else if strings.HasPrefix(line, "@@ ") {
					code = ansi.Red
				}
				if code != "" {
					value[i] = fmt.Sprintf("%s%s%s", code, line, ansi.Reset)
				} else {
					value[i] = line
				}
			}
		}
	}
	for i, line := range value {
		value[i] = "    " + line
	}
	return fmt.Sprintf("%s%s: \\\n%s", prefix, f.Name, strings.Join(value, "\n"))
}

// maybeShortenFilename will shorten `filename` with filepath.Base iff
// r.FullFilenames==false, otherwise returns `filename`.
func (r RenderCLI) maybeShortenFilename(filename string) string {
	if r.FullFilenames {
		return filename
	}
	return filepath.Base(filename)
}

// Stack pretty-prints a failure.Stack.
//
// If r.FullFilenames=false this will truncate filenames to just the base path,
// like `go test` does by default.
func (r RenderCLI) Stack(prefix string, s *failure.Stack) string {
	if len(s.Frames) == 0 {
		return ""
	}
	if len(s.Frames) == 1 {
		f := s.Frames[0]
		// e.g. `(at filename:NN)`
		return fmt.Sprintf("%s(%s %s:%d)", prefix, s.Name, r.maybeShortenFilename(f.Filename), f.Lineno)
	}
	lines := make([]string, 0, len(s.Frames))
	for _, f := range s.Frames {
		lines = append(lines, fmt.Sprintf("%s    %s:%d", prefix, r.maybeShortenFilename(f.Filename), f.Lineno))
	}
	return fmt.Sprintf("%s%s:\n%s", prefix, s.Name, strings.Join(lines, "\n"))
}

// Comparison pretty-prints a failure.Comparison.
//
// If `c` is nil, or is missing the Name field, this will use the name
// "UNKNOWN COMPARISON", which means that this function never returns an empty
// string.
func (r RenderCLI) Comparison(prefix string, c *failure.Comparison) string {
	testName := c.GetName()
	if testName == "" {
		testName = "UNKNOWN COMPARISON"
	}

	var testTypeArgs string
	if args := c.GetTypeArguments(); len(args) > 0 {
		testTypeArgs = fmt.Sprintf("[%s]", strings.Join(args, ", "))
	}
	var testArgs string
	if args := c.GetArguments(); len(args) > 0 {
		testArgs = fmt.Sprintf("(%s)", strings.Join(args, ", "))
	}

	return fmt.Sprintf("%s%s%s%s", prefix, testName, testTypeArgs, testArgs)
}

// Summary pretty-prints the result as a list of lines for display via the `go
// test` CLI output.
//
// If verbose is true, will render all verbose Findings.
// If colorize is true, will attempt to add ANSI coloring (currently just very
// basic per-line colors for diffs).
func (r RenderCLI) Summary(prefix string, f *failure.Summary) string {
	if f == nil {
		return ""
	}

	lines := make([]string, 0, 1+len(f.SourceContext)+len(f.Findings))
	lines = append(lines, r.Comparison(prefix, f.Comparison)+" FAILED")

	for _, context := range f.SourceContext {
		if toAdd := r.Stack(prefix, context); toAdd != "" {
			lines = append(lines, toAdd)
		}
	}
	for _, finding := range f.Findings {
		lines = append(lines, r.Finding(prefix, finding))
	}

	return strings.Join(lines, "\n")
}
