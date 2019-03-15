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

// CapturedStacktrace represents a stack trace returned by stacktrace(...).
//
// At the present time it can only be stringified (via str(...) in Starlark or
// via .String() in Go).
type CapturedStacktrace struct {
	// Note that *starlark.Frame is mutated after it is "captured". So we either
	// need to do a deep copy of the linked list of frames, or just render it to
	// the string right away. We opt for the later, since it is simpler and the
	// public interface supports only stringification anyway.
	backtrace string
}

// CaptureStacktrace captures thread's stack trace, skipping some number of
// innermost frames.
//
// Returns an error if the stack is not deep enough to skip the requested number
// of frames.
func CaptureStacktrace(th *starlark.Thread, skip int) (*CapturedStacktrace, error) {
	f := th.Caller()
	skipped := 0
	for skipped < skip && f != nil {
		f = f.Parent()
		skipped++
	}
	if f == nil {
		return nil, fmt.Errorf("stacktrace: the stack is not deep enough to skip %d levels, has only %d frames", skip, skipped)
	}
	buf := strings.Builder{}
	f.WriteBacktrace(&buf)
	return &CapturedStacktrace{buf.String()}, nil
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
