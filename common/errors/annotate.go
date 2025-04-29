// Copyright 2016 The LUCI Authors.
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

package errors

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/errors/errtag/stacktag"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/runtime/debugstack"
)

// Annotator is a builder for annotating errors. Obtain one by calling Annotate
// on an existing error or using Reason.
//
// See the example test for Annotate to see how this is meant to be used.
type Annotator struct {
	inner    error
	wrappers []ErrorWrapper
}

// ErrorWrapper describes a method which can accept an error and return an
// optionally wrapped version of that error.
//
// Implementations should return `nil` if provided `nil`.
type ErrorWrapper interface {
	Apply(err error) (wrapped error)
}

// Tag wraps the final error with `wrappers`.
//
// This can be used with the go.chromium.org/luci/common/errors/errtag package
// to allow tagging errors built with Annotator.
func (a *Annotator) Tag(wrappers ...ErrorWrapper) *Annotator {
	if a == nil {
		return a
	}
	a.wrappers = append(a.wrappers, wrappers...)
	return a
}

// Err returns the finalized annotated error.
func (a *Annotator) Err() error {
	if a == nil {
		return nil
	}
	ret := a.inner
	for _, wrapper := range a.wrappers {
		ret = wrapper.Apply(ret)
	}
	// The 1 is because this is a helper function we don't want to see in the
	// captured trace.
	ret = stacktag.Capture(ret, 1)
	return ret
}

// Log logs the full error. If this is an Annotated error, it will log the full
// stack information as well.
//
// Does nothing if err is nil.
func Log(ctx context.Context, err error, excludePkgs ...string) {
	if err == nil {
		return
	}

	stack := stacktag.Tag.ValueOrDefault(err)

	if stack == "" {
		// No stack in error, just log it normally.
		if tags := errtag.Collect(err, stacktag.Tag); len(tags) > 0 {
			logging.Errorf(ctx, "errtags:\n%s", strings.Join(tags.Format("  "), "\n"))
		}
		logging.Errorf(ctx, "%s", err)
	} else {
		// This has a stacktrace. Report it with a stack trace attached.

		stack := logging.StackTrace{
			Standard: stack,
			Textual:  RenderStack(err, excludePkgs...),
		}
		logging.ErrorWithStackTrace(ctx, stack, "error: %s", err)
	}
}

func dropFrames(stack string, excludePkgs ...string) string {
	if len(excludePkgs) == 0 {
		return stack
	}
	filters := make([]*regexp.Regexp, len(excludePkgs))
	for i, pkg := range excludePkgs {
		filters[i] = regexp.MustCompile(fmt.Sprintf("^%s$", regexp.QuoteMeta(pkg)))
	}
	parsed := debugstack.ParseString(stack).Filter(debugstack.CompileRules(
		debugstack.Rule{
			ApplyTo:     debugstack.StackFrameKind,
			DropIfInPkg: filters,
		},
	), true)
	return parsed.String()
}

// RenderStack renders the error message and a stack to a list of lines.
//
// Uses a compact stack format.
func RenderStack(err error, excludePkgs ...string) string {
	if err == nil {
		return ""
	}
	ret := err.Error()
	if tags := errtag.Collect(err, stacktag.Tag); len(tags) > 0 {
		ret += fmt.Sprintf("\n\nerrtags:\n%s\n", strings.Join(tags.Format("  "), "\n"))
	}
	if stack := stacktag.Tag.ValueOrDefault(err); stack != "" {
		stack = dropFrames(stack, excludePkgs...)
		ret += fmt.Sprintf("\n%s", stack)
	}
	return ret
}

// RenderGoStack renders the error to a Go-style stacktrace.
//
// If `onlyInner` is true, this will only return the inner-most stack in case
// this error was annotated by multiple goroutines.
//
// If it's false, then all goroutines which annotated this error will have their
// stacks combined into a single trace.
//
// If `err` is not annotated, returns the empty string.
func RenderGoStack(err error, onlyInner bool, excludePkgs ...string) string {
	return dropFrames(stacktag.Tag.ValueOrDefault(err), excludePkgs...)
}

// Annotate captures the current stack frame and returns a new annotatable
// error, attaching the publicly readable `reason` format string to the error.
// You can add tags to this error with the 'Tag' method, and then obtain a real
// `error` with the Err() function.
//
// If this is passed nil, it will return a no-op Annotator whose .Err() function
// will also return nil.
//
// The original error may be recovered by using Wrapped.Unwrap on the
// returned error.
//
// Rendering the derived error with Error() will render a summary version of all
// the public `reason`s as well as the initial underlying error's Error() text.
// It is intended that the initial underlying error and all annotated reasons
// only contain user-visible information, so that the accumulated error may be
// returned to the user without worrying about leakage.
//
// You should assume that end-users (including unauthenticated end users) may
// see the text in the `reason` field here. To only attach an internal reason,
// leave the `reason` argument blank and don't pass any additional formatting
// arguments.
//
// The `reason` string is formatted with `args` and may contain Sprintf-style
// formatting directives.
func Annotate(err error, reason string, args ...any) *Annotator {
	if err == nil {
		return nil
	}

	args = slices.Clone(args)
	args = append(args, err)
	return &Annotator{fmt.Errorf(reason+": %w", args...), nil}
}

// Reason builds a new Annotator starting with reason. This allows you to use
// all the formatting directives you would normally use with Annotate, in case
// your originating error needs tags or an internal reason.
//
//	errors.Reason("something bad: %d", value).Tag(transient.Tag).Err()
//
// Prefer this form to errors.New(fmt.Sprintf("...")) or fmt.Errorf("...")
func Reason(reason string, args ...any) *Annotator {
	return &Annotator{fmt.Errorf(reason, args...), nil}
}

// New is an API-compatible version of the standard errors.New function, except
// that it also allows you to apply wrappers (like from the errtag library).
//
// This function does capture the stack, but ONLY if this is not used at
// init()-time. This is to avoid module level errors having a mostly-useless
// stack attached.
func New(msg string, wrappers ...ErrorWrapper) error {
	ret := errors.New(msg)
	for _, wrapper := range wrappers {
		ret = wrapper.Apply(ret)
	}
	// The 1 is because this is a helper function we don't want to see in the
	// captured trace.
	return stacktag.Capture(ret, 1)
}

// Fmt is an API-compatible version of the standard fmt.Errorf function, except
// that it captures the stack, but ONLY if this is not used at init()-time. This
// is to avoid module level errors having a mostly-useless stack attached.
func Fmt(format string, args ...any) error {
	// The 1 is because this is a helper function we don't want to see in the
	// captured trace.
	return stacktag.Capture(fmt.Errorf(format, args...), 1)
}

// WrapIf is the same as Fmt, except that:
//
//   - It explicitly checks that `err` is not nil. If it is, then this returns
//     `nil`.
//   - It adds ": %w" to format and err to args.
//
// This is meant to be used like:
//
//	return errors.WrapIf(someCall(i), "while doing someCall(%d)", i)
//
// Instead of:
//
//	if err := someCall(i); err != nil {
//	  return errors.Fmt("while doing someCall(%d): %w", i, err)
//	}
//	return nil
//
// These two code snippets have identical behavior.
//
// This is convenient, but is mostly an affordance to safely convert a previous
// `errors.Annotate` api which had this semantic.
func WrapIf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	// The 1 is because this is a helper function we don't want to see in the
	// captured trace.
	//
	// Clip is necessary to avoid mutating a passed-in `args`.
	return stacktag.Capture(fmt.Errorf(format+": %w", append(slices.Clip(args), err)...), 1)
}
