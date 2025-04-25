// Copyright 2025 The LUCI Authors.
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

// Package errs contains helpers for working with errors with backtraces.
package errs

import (
	"fmt"
	"strings"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/starlark/builtins"

	"go.chromium.org/luci/lucicfg/graph"
)

// Backtracable is an error that has a Starlark backtrace attached to it.
//
// Implemented by Error here, by starlark.EvalError and graph errors.
type Backtracable interface {
	error

	// Backtrace returns a user-friendly error message describing the stack
	// of calls that led to this error, along with the error message itself.
	Backtrace() string
}

// WithExtraContext is an error that has some extra context attached to it.
//
// This context is rendered after the original error message.
type WithExtraContext interface {
	error

	// ExtraContext will be rendered after the error message.
	ExtraContext() string
}

var (
	_ Backtracable = (*starlark.EvalError)(nil)
	_ Backtracable = (*builtins.Failure)(nil)
	_ Backtracable = (*Error)(nil)
	_ Backtracable = (*graph.NodeRedeclarationError)(nil)
	_ Backtracable = (*graph.CycleError)(nil)
	_ Backtracable = (*graph.DanglingEdgeError)(nil)
)

// Error is a single error message emitted by the config generator.
//
// It holds a stack trace responsible for the error.
type Error struct {
	Msg   string
	Stack *builtins.CapturedStacktrace
}

// Error is part of 'error' interface.
func (e *Error) Error() string {
	return e.Msg
}

// Backtrace is part of Backtracable interface.
func (e *Error) Backtrace() string {
	if e.Stack == nil {
		return e.Msg
	}
	return fmt.Sprintf("%sError: %s", e.Stack, e.Msg)
}

// Collect traverses err (which can be a MultiError, recursively), and appends
// all error messages there to 'in', returning the resulting slice.
//
// They are eventually used in JSON output and printed to stderr.
func Collect(err error, in []string) []string {
	switch {
	case err == nil:
		return in
	case errors.Is(err, auth.ErrLoginRequired):
		return append(in, "Need to login first by running:\nlucicfg auth-login")
	}
	return collect(err, in)
}

// collect traverses error tree, recognizing Backtracable errors.
//
// Assumes particular format of rendered annotated errors to extract annotations
// from them (aka does hacks).
func collect(err error, in []string) (out []string) {
	out = in

	var merr interface{ Unwrap() []error }
	if errors.As(err, &merr) {
		// Note: this totally skips any annotations attached to the MultiError
		// itself. Preserving them is quite difficult, since wrapped errors do not
		// expose an API to collect only "wrappers".
		for _, inner := range merr.Unwrap() {
			out = collect(inner, out)
		}
		return
	}

	var errMsg string
	var bterr Backtracable
	if errors.As(err, &bterr) {
		// E.g. "boom".
		rootCause := bterr.Error()
		// Either "<stack>\nError: boom" or just "boom".
		withTrace := bterr.Backtrace()
		// E.g. "context1: context2: boom".
		fullErr := err.Error()

		if withTrace == rootCause {
			// If there's no backtrace, just use the full annotated error message.
			errMsg = fmt.Sprintf("Error: %s", fullErr)
		} else {
			// If there's a backtrace, use it report the root cause error. Chop off
			// the root cause error from the `fullErr`. What remains (if anything) is
			// the context string "context1: context2".
			errCtx, cut := strings.CutSuffix(fullErr, ": "+rootCause)
			if cut && errCtx != "" {
				errMsg = fmt.Sprintf("Error: %s:\n%s", errCtx, withTrace)
			} else {
				errMsg = withTrace
			}
		}
	} else {
		errMsg = fmt.Sprintf("Error: %s", err)
	}

	// Fish out an extra context from anywhere under `err` (not necessary the
	// same error as Backtracable).
	extraCtx := ""
	var extraCtxErr WithExtraContext
	if errors.As(err, &extraCtxErr) {
		extraCtx = extraCtxErr.ExtraContext()
	}

	if extraCtx == "" {
		out = append(out, errMsg)
	} else {
		out = append(out, fmt.Sprintf("%s\n%s", errMsg, extraCtx))
	}
	return
}
