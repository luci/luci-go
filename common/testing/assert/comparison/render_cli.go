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
)

// if lines has zero lines or one line with an empty value, returns prefix+title.
// if lines has one line with a value, returns just that line prefixed with prefix and
// title.
// if lines has many lines, returns a heredoc style set of lines with the first
// line as `prefix+title: EOF`, the lines, and then `EOF`.
func heredocLines(prefix, title string, lines []string) (ret []string) {
	if len(lines) == 0 || len(strings.TrimSpace(lines[0])) == 0 {
		return []string{fmt.Sprintf("%s%s", prefix, title)}
	}
	if len(lines) == 1 {
		return []string{fmt.Sprintf("%s%s: %s", prefix, title, lines[0])}
	}

	ret = make([]string, 0, len(lines)+2)
	ret = append(ret, fmt.Sprintf("%s%s: <<EOF", prefix, title))
	ret = append(ret, lines...)
	ret = append(ret, "EOF")

	return ret
}

// RenderCLI pretty-prints the result as a list of lines for display via the `go
// test` CLI output.
func RenderCLI(r *Failure) []string {
	if r == nil {
		return nil
	}
	var lines []string
	testName := r.GetComparison().GetName()
	if testName == "" {
		testName = "UNKNOWN COMPARISON"
	}

	var testTypeArgs string
	if args := r.GetComparison().GetTypeArguments(); len(args) > 0 {
		testTypeArgs = fmt.Sprintf("[%s]", strings.Join(args, ", "))
	}

	lines = append(lines, fmt.Sprintf("%s%s FAILED", testName, testTypeArgs))
	for _, f := range r.Findings {
		lines = append(lines, heredocLines("  ", f.Name, strings.Split(f.Value, "\n"))...)
	}

	return lines
}
