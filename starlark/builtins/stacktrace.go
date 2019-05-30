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
	"regexp"
	"strings"

	"go.starlark.net/starlark"
)

// CapturedStacktrace represents a stack trace returned by stacktrace(...).
//
// At the present time it can only be stringified (via str(...) in Starlark or
// via .String() in Go).
type CapturedStacktrace struct {
	backtrace string
}

// CaptureStacktrace captures thread's stack trace, skipping some number of
// innermost frames.
//
// Returns an error if the stack is not deep enough to skip the requested number
// of frames.
func CaptureStacktrace(th *starlark.Thread, skip int) (*CapturedStacktrace, error) {
	if th.CallStackDepth() <= skip {
		return nil, fmt.Errorf("stacktrace: the stack is not deep enough to skip %d levels, has only %d frames", skip, th.CallStackDepth())
	}

	stack := th.CallStack()[:th.CallStackDepth()-skip]
	return &CapturedStacktrace{stack.String()}, nil
}

// Type is part of starlark.Value interface.
func (*CapturedStacktrace) Type() string { return "stacktrace" }

// Freeze is part of starlark.Value interface.
func (*CapturedStacktrace) Freeze() {}

// Truth is part of starlark.Value interface.
func (*CapturedStacktrace) Truth() starlark.Bool { return starlark.True }

// Hash is part of starlark.Value interface.
func (*CapturedStacktrace) Hash() (uint32, error) { return 0, errors.New("stacktrace is unhashable") }

// String is part of starlark.Value interface.
//
// Renders the stack trace as string.
func (s *CapturedStacktrace) String() string { return s.backtrace }

// Stacktrace is stacktrace(...) builtin.
//
//  def stacktrace(skip=0):
//    """Capture and returns a stack trace of the caller.
//
//    A captured stacktrace is an opaque object that can be stringified to get a
//    nice looking trace (e.g. for error messages).
//
//    Args:
//      skip: how many inner most frames to skip.
//    """
var Stacktrace = starlark.NewBuiltin("stacktrace", func(th *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	skip := starlark.MakeInt(0)
	if err := starlark.UnpackArgs("stacktrace", args, kwargs, "skip?", &skip); err != nil {
		return nil, err
	}
	switch lvl, err := starlark.AsInt32(skip); {
	case err != nil:
		return nil, fmt.Errorf("stacktrace: bad 'skip' value %s - %s", skip, err)
	case lvl < 0:
		return nil, fmt.Errorf("stacktrace: bad 'skip' value %d - must be non-negative", lvl)
	default:
		return CaptureStacktrace(th, lvl)
	}
})

// This matches "<module>:<line>:<col>: in <func>" where <line>:<col> is
// optional.
var stackLineRe = regexp.MustCompile(`^(\s*)(.*?): in (.*)$`)

// NormalizeStacktrace removes mentions of line and column numbers from a
// rendered stack trace: "main:1:5: in <toplevel>" => "main: in <toplevel>".
//
// Useful when comparing stack traces in tests to make the comparison less
// brittle.
func NormalizeStacktrace(trace string) string {
	lines := strings.Split(trace, "\n")
	for i := range lines {
		if m := stackLineRe.FindStringSubmatch(lines[i]); m != nil {
			space, module, funcname := m[1], m[2], m[3]
			if idx := strings.IndexRune(module, ':'); idx != -1 {
				module = module[:idx]
			}
			lines[i] = fmt.Sprintf("%s%s: in %s", space, module, funcname)
		}
	}
	return strings.Join(lines, "\n")
}
