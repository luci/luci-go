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
	"go.starlark.net/starlark"

	"go.chromium.org/luci/starlark/builtins"
)

// BacktracableError is an error that has a starlark backtrace attached to it.
//
// Implemented by Error here and by starlark.EvalError.
type BacktracableError interface {
	error

	// Backtrace returns a user-friendly error message describing the stack
	// of calls that led to this error, along with the error message itself.
	Backtrace() string
}

var _ BacktracableError = (*starlark.EvalError)(nil)
var _ BacktracableError = (*Error)(nil)

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

// Backtrace is part of BacktracableError interface.
func (e *Error) Backtrace() string {
	if e.Stack == nil {
		return e.Msg
	}
	return e.Stack.String() + "Error: " + e.Msg
}

func init() {
	// emit_error(msg, stack) adds the given error to the list of errors in the
	// state, to be returned at the end of generation.
	declNative("emit_error", func(call nativeCall) (starlark.Value, error) {
		var msg starlark.String
		var stack *builtins.CapturedStacktrace
		if err := call.unpack(2, &msg, &stack); err != nil {
			return nil, err
		}
		call.State.err(&Error{
			Msg:   msg.GoString(),
			Stack: stack,
		})
		return starlark.None, nil
	})
}
