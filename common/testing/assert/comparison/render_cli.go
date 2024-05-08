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
	"strings"

	"github.com/mgutz/ansi"
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
}

// Finding renders a Finding to a set of output lines which would be
// suitable for display as CLI output (e.g. to be logged with testing.T.Log
// calls).
func (r RenderCLI) Finding(prefix string, f *Failure_Finding) (ret string) {
	if len(f.Value) == 0 {
		return fmt.Sprintf("%s%s [no value]", prefix, f.Name)
	}
	if len(strings.TrimSpace(f.Value[0])) == 0 {
		return fmt.Sprintf("%s%s [blank one-line value]", prefix, f.Name)
	}

	if f.Level > FindingLogLevel_Error && !r.Verbose {
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
		case FindingTypeHint_CmpDiff, FindingTypeHint_UnifiedDiff:
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

// Failure pretty-prints the result as a list of lines for display via the `go
// test` CLI output.
//
// If verbose is true, will render all verbose Findings.
// If colorize is true, will attempt to add ANSI coloring (currently just very
// basic per-line colors for diffs).
func (r RenderCLI) Failure(prefix string, f *Failure) string {
	if f == nil {
		return ""
	}
	testName := f.GetComparison().GetName()
	if testName == "" {
		testName = "UNKNOWN COMPARISON"
	}

	var testTypeArgs string
	if args := f.GetComparison().GetTypeArguments(); len(args) > 0 {
		testTypeArgs = fmt.Sprintf("[%s]", strings.Join(args, ", "))
	}

	if len(f.Findings) == 0 {
		return fmt.Sprintf("%s%s FAILED", testName, testTypeArgs)
	}

	findingLines := make([]string, 0, len(f.Findings))
	for _, finding := range f.Findings {
		findingLines = append(findingLines, r.Finding(prefix, finding))
	}

	return fmt.Sprintf("%s%s FAILED\n%s", testName, testTypeArgs, strings.Join(findingLines, "\n"))
}
