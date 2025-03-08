// Copyright 2020 The LUCI Authors.
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

// Package buildifier implements processing of Starlark files via buildifier.
//
// Buildifier is primarily intended for Bazel files. We try to disable as much
// of Bazel-specific logic as possible, keeping only generally useful
// Starlark rules.
package buildifier

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/bazelbuild/buildtools/build"
	"github.com/bazelbuild/buildtools/warn"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/starlark/interpreter"

	"go.chromium.org/luci/lucicfg/vars"
)

var (
	// ErrActionableFindings is returned by Lint if there are actionable findings.
	ErrActionableFindings = errors.New("some *.star files have linter warnings, please fix them")
)

// formattingCategory is linter check to represent `lucicfg fmt` checks.
//
// It's not a real buildifier category, we should be careful not to pass it to
// warn.FileWarnings.
const formattingCategory = "formatting"

// Finding is information about one linting or formatting error.
//
// Implements error interface. Non-actionable findings are assumed to be
// non-blocking errors.
type Finding struct {
	Path       string    `json:"path"`
	Start      *Position `json:"start,omitempty"`
	End        *Position `json:"end,omitempty"`
	Category   string    `json:"string,omitempty"`
	Message    string    `json:"message,omitempty"`
	Actionable bool      `json:"actionable,omitempty"`
}

// Position indicates a position within a file.
type Position struct {
	Line   int `json:"line"`   // starting from 1
	Column int `json:"column"` // in runes, starting from 1
	Offset int `json:"offset"` // absolute offset in bytes
}

// Error returns a short summary of the finding.
func (f *Finding) Error() string {
	switch {
	case f.Path == "":
		return f.Category
	case f.Start == nil:
		return fmt.Sprintf("%s: %s", f.Path, f.Category)
	default:
		return fmt.Sprintf("%s:%d: %s", f.Path, f.Start.Line, f.Category)
	}
}

// Format returns a detailed reported that can be printed to stderr.
func (f *Finding) Format() string {
	if strings.ContainsRune(f.Message, '\n') {
		return fmt.Sprintf("%s: %s\n\n", f.Error(), f.Message)
	} else {
		return fmt.Sprintf("%s: %s\n", f.Error(), f.Message)
	}
}

// Lint applies linting and formatting checks to the given files.
//
// getRewriterForPath should return a Rewriter, given the path which
// needs linting. This will be used to check the 'format' lint check.
// If getRewriterForPath is nil, we will use vars.GetDefaultRewriter for
// this.
//
// Returns all findings and a non-nil error (usually a MultiError) if some
// findings are blocking.
func Lint(ctx context.Context, loader interpreter.Loader, paths []string, lintChecks []string, getRewriterForPath func(path string) (*build.Rewriter, error)) (findings []*Finding, err error) {
	checks, err := normalizeLintChecks(lintChecks)
	if err != nil {
		return nil, err
	}

	if getRewriterForPath == nil {
		getRewriterForPath = func(path string) (*build.Rewriter, error) {
			return vars.GetDefaultRewriter(), nil
		}
	}

	// Transform unrecognized linter checks into warning-level findings.
	allPossible := allChecks()
	buildifierWarns := make([]string, 0, checks.Len())
	checkFmt := false
	for _, check := range checks.ToSortedSlice() {
		switch {
		case !allPossible.Has(check):
			findings = append(findings, &Finding{
				Category: "linter",
				Message:  fmt.Sprintf("Unknown linter check %q", check),
			})
		case check == formattingCategory:
			checkFmt = true
		default:
			buildifierWarns = append(buildifierWarns, check)
		}
	}

	if len(paths) == 0 || (!checkFmt && len(buildifierWarns) == 0) {
		return findings, nil
	}

	errs := Visit(ctx, loader, paths, func(path string, body []byte, f *build.File) (merr errors.MultiError) {
		if len(buildifierWarns) != 0 {
			findings := warn.FileWarnings(f, buildifierWarns, nil, warn.ModeWarn, newFileReader(ctx, loader))
			for _, f := range findings {
				merr = append(merr, &Finding{
					Path: path,
					Start: &Position{
						Line:   f.Start.Line,
						Column: f.Start.LineRune,
						Offset: f.Start.Byte,
					},
					End: &Position{
						Line:   f.End.Line,
						Column: f.End.LineRune,
						Offset: f.End.Byte,
					},
					Category:   f.Category,
					Message:    f.Message,
					Actionable: f.Actionable,
				})
			}
		}

		rewriter, err := getRewriterForPath(f.Path)
		if err != nil {
			return errors.MultiError{err}
		}

		if checkFmt && !bytes.Equal(build.FormatWithRewriter(rewriter, f), body) {
			merr = append(merr, &Finding{
				Path:       path,
				Category:   formattingCategory,
				Message:    `The file is not properly formatted, use 'lucicfg fmt' to format it.`,
				Actionable: true,
			})
		}
		return merr
	})
	if len(errs) == 0 {
		return findings, nil
	}

	// Extract findings into a dedicated slice. Return an overall error if there
	// are actionable findings.
	filtered := errs[:0]
	actionable := false
	for _, err := range errs {
		if f, ok := err.(*Finding); ok {
			findings = append(findings, f)
			if f.Actionable {
				actionable = true
			}
		} else {
			filtered = append(filtered, err)
		}
	}
	if actionable {
		filtered = append(filtered, ErrActionableFindings)
	}

	if len(filtered) == 0 {
		return findings, nil
	}
	return findings, filtered
}

// Visitor processes a parsed Starlark file, returning all errors encountered
// when processing it.
type Visitor func(path string, body []byte, f *build.File) errors.MultiError

// Visit parses Starlark files using Buildifier and calls the callback for each
// parsed file, in parallel.
//
// Collects all errors from all callbacks in a single joint multi-error.
func Visit(ctx context.Context, loader interpreter.Loader, paths []string, v Visitor) errors.MultiError {

	m := sync.Mutex{}
	perPath := make(map[string]errors.MultiError, len(paths))

	parallel.WorkPool(runtime.NumCPU(), func(tasks chan<- func() error) {
		for _, path := range paths {
			path := path
			tasks <- func() error {
				var errs []error
				switch body, f, err := parseFile(ctx, loader, path); {
				case err != nil:
					errs = []error{err}
				case f != nil:
					errs = v(path, body, f)
				}
				m.Lock()
				perPath[path] = errs
				m.Unlock()
				return nil
			}
		}
	})

	// Assemble errors in original order.
	var errs errors.MultiError
	for _, path := range paths {
		errs = append(errs, perPath[path]...)
	}
	return errs
}

// parseFile parses a Starlark module using the buildifier parser.
//
// Returns (nil, nil, nil) if the module is a native Go module.
func parseFile(ctx context.Context, loader interpreter.Loader, path string) ([]byte, *build.File, error) {
	switch dict, src, err := loader(ctx, path); {
	case err != nil:
		return nil, nil, err
	case dict != nil:
		return nil, nil, nil
	default:
		body := []byte(src)
		f, err := build.ParseDefault(path, body)
		if f != nil {
			f.Type = build.TypeDefault // always generic Starlark file, not a BUILD
			f.Label = path             // lucicfg loader paths ~= map to Bazel labels
		}
		return body, f, err
	}
}

// newFileReader returns a warn.FileReader based on the loader.
//
// Note: *warn.FileReader doesn't protect its caching guts with any locks so we
// can't share a single copy across multiple goroutines.
func newFileReader(ctx context.Context, loader interpreter.Loader) *warn.FileReader {
	return warn.NewFileReader(func(path string) ([]byte, error) {
		switch dict, src, err := loader(ctx, path); {
		case err != nil:
			return nil, err
		case dict != nil:
			return nil, nil // skip native modules
		default:
			return []byte(src), nil
		}
	})
}

// normalizeLintChecks replaces `all` with an explicit list of checks and does
// other similar transformations.
//
// Checks has a form ["<optional initial category>", "+warn", "-warn", ...].
// Where <optional initial category> can be `none`, `default` or `all`.
//
// Doesn't check all added checks are actually defined.
func normalizeLintChecks(checks []string) (stringset.Set, error) {
	if len(checks) == 0 {
		checks = []string{"default"}
	}

	var set stringset.Set
	if cat := checks[0]; !strings.HasPrefix(cat, "+") && !strings.HasPrefix(cat, "-") {
		switch cat {
		case "none":
			set = stringset.New(0)
		case "all":
			set = allChecks()
		case "default":
			set = defaultChecks()
		default:
			return nil, fmt.Errorf(
				`unrecognized linter checks category %q: must be one of "none", "all", "default" `+
					`(if you want to enable individual checks, use "+name" syntax)`, cat)
		}
		checks = checks[1:]
	} else {
		set = defaultChecks()
	}

	for _, check := range checks {
		switch {
		case strings.HasPrefix(check, "+"):
			set.Add(check[1:])
		case strings.HasPrefix(check, "-"):
			set.Del(check[1:])
		default:
			return nil, fmt.Errorf(`use "+name" to enable a check or "-name" to disable it, got %q instead`, check)
		}
	}

	return set, nil
}

func allChecks() stringset.Set {
	s := stringset.NewFromSlice(warn.AllWarnings...)
	s.Add(formattingCategory)
	return s
}

func defaultChecks() stringset.Set {
	s := stringset.NewFromSlice(warn.DefaultWarnings...)
	s.Add(formattingCategory)
	s.Del("load-on-top")   // order of loads may matter in lucicfg
	s.Del("uninitialized") // this check doesn't work well with lambdas and inner functions
	return s
}
