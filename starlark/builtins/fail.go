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

package builtins

import (
	"errors"
	"fmt"
	"strings"

	"go.starlark.net/starlark"
)

// Failure is an error emitted by fail(...) and captured by FailureCollector.
type Failure struct {
	Message   string              // the error message, as passed to fail(...)
	UserTrace *CapturedStacktrace // value of 'trace' passed to fail or nil
	FailTrace *CapturedStacktrace // where 'fail' itself was called
}

// Error is the short error message, as passed to fail(...).
func (f *Failure) Error() string {
	return f.Message
}

// Backtrace returns a user-friendly error message describing the stack of
// calls that led to this error.
//
// If fail(...) was called with a custom stack trace, this trace is shown here.
// Otherwise the trace of where fail(...) happened is used.
func (f *Failure) Backtrace() string {
	tr := f.UserTrace
	if tr == nil {
		tr = f.FailTrace
	}
	return tr.String() + "Error: " + f.Message
}

// Fail is fail(*args, sep=" ", trace=None) builtin.
//
//  def fail(*args, sep=" ", trace=None):
//    """Aborts the script execution with an error message."
//
//    Args:
//      args: values to print in the message.
//      sep: separator to use between values from `args`.
//      trace: a trace (as returned by stacktrace()) to attach to the error.
//    """
//
// Custom stack traces are recoverable through FailureCollector. This is due
// to Starlark's insistence on stringying all errors. If there's no
// FailureCollector in the thread locals, custom traces are silently ignored.
//
// Note that the assert.fails(...) check in the default starlark tests library
// doesn't clear the failure collector state when it "catches" an error, so
// tests that use assert.fails(...) should be careful with using the failure
// collector (or just don't use it at all).
var Fail = starlark.NewBuiltin("fail", func(th *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	sep := " "
	var trace starlark.Value
	err := starlark.UnpackArgs("fail", nil, kwargs,
		"sep?", &sep,
		"trace?", &trace)
	if err != nil {
		return nil, err
	}

	var userTrace *CapturedStacktrace
	if trace != nil && trace != starlark.None {
		if userTrace, _ = trace.(*CapturedStacktrace); userTrace == nil {
			return nil, fmt.Errorf("fail: bad 'trace' - got %s, expecting stacktrace", trace.Type())
		}
	}

	buf := strings.Builder{}
	for i, v := range args {
		if i > 0 {
			buf.WriteString(sep)
		}
		if s, ok := starlark.AsString(v); ok {
			buf.WriteString(s)
		} else {
			buf.WriteString(v.String())
		}
	}
	msg := buf.String()

	if fc := GetFailureCollector(th); fc != nil {
		failTrace, _ := CaptureStacktrace(th, 0)
		fc.failure = &Failure{
			Message:   msg,
			UserTrace: userTrace,
			FailTrace: failTrace,
		}
	}

	return nil, errors.New(msg)
})

// A key in thread.Locals to hold *FailureCollector.
const failSlotKey = "go.chromium.org/luci/starlark/builtins.FailureCollector"

// FailureCollector receives structured error messages from fail(...).
//
// It should be installed into Starlark thread locals (via Install) for
// fail(...) to be able to discover it. If it's not there, fail(...) will not
// return any additional information (like a custom stack trace) besides the
// information contained in *starlark.EvalError.
type FailureCollector struct {
	// failure is the error passed to fail(...).
	//
	// fail(...) aborts the execution of starlark scripts, so its fine to keep
	// only one error. There can't really be more.
	failure *Failure
}

// GetFailureCollector returns a failure collector installed in the thread.
func GetFailureCollector(th *starlark.Thread) *FailureCollector {
	fc, _ := th.Local(failSlotKey).(*FailureCollector)
	return fc
}

// Install installs this failure collector into the thread.
func (fc *FailureCollector) Install(t *starlark.Thread) {
	t.SetLocal(failSlotKey, fc)
}

// LatestFailure returns the latest captured failure or nil if there are none.
func (fc *FailureCollector) LatestFailure() *Failure {
	return fc.failure
}

// Clear resets the state.
//
// Useful if the same FailureCollector is reused between calls to Starlark.
func (fc *FailureCollector) Clear() {
	fc.failure = nil
}
