// Copyright 2018 The LUCI Authors.
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

package lucicfg

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/sergi/go-diff/diffmatchpatch"

	"go.starlark.net/resolve"
	"go.starlark.net/starlark"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/starlark/interpreter"
	"go.chromium.org/luci/starlark/starlarktest"
)

func init() {
	// Enable not-yet-standard features.
	resolve.AllowLambda = true
	resolve.AllowNestedDef = true
	resolve.AllowFloat = true
	resolve.AllowSet = true
}

// TestAllStarlark loads and executes all test scripts (testdata/*.star).
func TestAllStarlark(t *testing.T) {
	t.Parallel()

	starlarktest.RunTests(t, starlarktest.Options{
		TestsDir: "testdata",
		Skip:     "support",

		Executor: func(t *testing.T, path string, predeclared starlark.StringDict) error {
			blob, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}
			body := string(blob)

			expectErrExct := readCommentBlock(body, "Expect errors:")
			expectErrLike := readCommentBlock(body, "Expect errors like:")
			expectCfg := readCommentBlock(body, "Expect configs:")
			if expectErrExct != "" && expectErrLike != "" {
				t.Errorf("Cannot use 'Expect errors' and 'Expect errors like' at the same time")
				return nil
			}

			// We treat tests that compare the generator output to some expected
			// output as "integration tests", and everything else is a unit tests.
			// See below for why this is important.
			integrationTest := expectErrExct != "" || expectErrLike != "" || expectCfg != ""

			state, err := Generate(context.Background(), Inputs{
				// Use file system loader so test scripts can load supporting scripts
				// (from '**/support/*' which is skipped by the test runner). This also
				// makes error messages have the original scripts full name. Note that
				// 'go test' executes tests with cwd set to corresponding package
				// directories, regardless of what cwd was when 'go test' was called.
				Code:  interpreter.FileSystemLoader("."),
				Entry: filepath.ToSlash(path),

				// Expose 'assert' module, hook up error reporting to 't'.
				testPredeclared: predeclared,
				testThreadModifier: func(th *starlark.Thread) {
					starlarktest.HookThread(th, t)
				},

				// Failure collector interferes with assert.fails() in a bad way.
				// assert.fails() captures errors, but it doesn't clear the failure
				// collector state, so we may end up in a situation when the script
				// fails with one error (some native starlark error, e.g. invalid
				// function call, not 'fail'), but the failure collector remembers
				// another (stale!) error, emitted by 'fail' before and caught by
				// assert.fails(). This results in invalid error message at the end
				// of the script execution.
				//
				// Unfortunately, it is not easy to modify assert.fails() without
				// forking it. So instead we do a cheesy thing and disable the failure
				// collector if the file under test appears to be unit-testy (rather
				// than integration-testy). We define integration tests to be tests
				// that examine the output of the generator using "Expect ..." blocks
				// (see above), and unit tests are tests that use asserts.
				//
				// Disabling the failure collector results in fail(..., trace=t)
				// ignoring the custom stack trace 't'. But unit tests don't generally
				// check the stack trace (only the error message), so it's not a big
				// deal for them.
				testDisableFailureCollector: !integrationTest,
			})

			// If test was expected to fail on Starlark side, make sure it did, in
			// an expected way.
			if expectErrExct != "" || expectErrLike != "" {
				allErrs := strings.Builder{}
				errors.WalkLeaves(err, func(err error) bool {
					if bt, ok := err.(BacktracableError); ok {
						allErrs.WriteString(bt.Backtrace())
					} else {
						allErrs.WriteString(err.Error())
					}
					allErrs.WriteString("\n\n")
					return true
				})

				if expectErrExct != "" {
					errorOnDiff(t, allErrs.String(), expectErrExct)
				} else {
					errorOnPatternMismatch(t, allErrs.String(), expectErrLike)
				}
				return nil
			}

			// Otherwise just report all errors to Mr. T.
			errors.WalkLeaves(err, func(err error) bool {
				if bt, ok := err.(BacktracableError); ok {
					t.Errorf("%s\n", bt.Backtrace())
				} else {
					t.Errorf("%s\n", err)
				}
				return true
			})
			if err != nil {
				return nil // the error has been reported already
			}

			// If was expecting to see some configs, assert we did see them.
			if expectCfg != "" {
				files := make([]string, 0, len(state.Configs))
				for f := range state.Configs {
					files = append(files, f)
				}
				sort.Strings(files)
				got := bytes.Buffer{}
				for _, f := range files {
					fmt.Fprintf(&got, "=== %s\n", f)
					fmt.Fprintf(&got, string(state.Configs[f]))
					fmt.Fprintf(&got, "===\n\n")
				}
				errorOnDiff(t, got.String(), expectCfg)
			}

			return nil
		},
	})
}

// readCommentBlock reads a comment block that start with "# <hdr>\n".
//
// Return empty string if there's no such block.
func readCommentBlock(script, hdr string) string {
	scanner := bufio.NewScanner(strings.NewReader(script))
	for scanner.Scan() && scanner.Text() != "# "+hdr {
		continue
	}
	sb := strings.Builder{}
	for scanner.Scan() {
		if line := scanner.Text(); strings.HasPrefix(line, "#") {
			sb.WriteString(strings.TrimPrefix(line[1:], " "))
			sb.WriteRune('\n')
		}
	}
	return sb.String()
}

// errorOnDiff emits an error to T if got != exp.
func errorOnDiff(t *testing.T, got, exp string) {
	t.Helper()

	got = strings.TrimSpace(got)
	exp = strings.TrimSpace(exp)
	switch {
	case got == "":
		t.Errorf("Got nothing, but was expecting:\n\n%s\n", exp)
	case got != exp:
		dmp := diffmatchpatch.New()
		diffs := dmp.DiffMain(exp, got, false)
		t.Errorf(
			"Got:\n\n%s\n\nWas expecting:\n\n%s\n\nDiff:\n\n%s\n",
			got, exp, dmp.DiffPrettyText(diffs))
	}
}

// errorOnMismatch emits an error to T if got doesn't match a pattern pat.
//
// The pattern is syntax is:
//   * A line "[space]...[space]" matches zero or more arbitrary lines.
//   * Trigram "???" matches [0-9a-zA-Z]+.
//   * The rest should match as is.
func errorOnPatternMismatch(t *testing.T, got, pat string) {
	t.Helper()

	got = strings.TrimSpace(got)
	pat = strings.TrimSpace(pat)

	re := strings.Builder{}
	re.WriteRune('^')
	for _, line := range strings.Split(pat, "\n") {
		if strings.TrimSpace(line) == "..." {
			re.WriteString(`(.*\n)*`)
		} else {
			for line != "" {
				idx := strings.Index(line, "???")
				if idx == -1 {
					re.WriteString(regexp.QuoteMeta(line))
					break
				}
				re.WriteString(regexp.QuoteMeta(line[:idx]))
				re.WriteString(`[0-9a-zA-Z]+`)
				line = line[idx+3:]
			}
			re.WriteString(`\n`)
		}
	}
	re.WriteRune('$')

	if exp := regexp.MustCompile(re.String()); !exp.MatchString(got + "\n") {
		t.Errorf("Got:\n\n%s\n\nWas expecting pattern:\n\n%s\n\n", got, pat)
		t.Errorf("Regexp: %s", re.String())
	}
}
