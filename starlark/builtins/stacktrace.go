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
	"bytes"
	"errors"
	"fmt"

	"go.starlark.net/starlark"
)

// CapturedStacktrace represents a stack trace returned by stacktrace(...).
//
// At the present time it can only be stringified (via str(...) in Starlark or
// via .String() in Go).
type CapturedStacktrace struct {
	// Note that starlark.Frame is immutable and has NO reference to the local
	// dict of a module/function, so its fine to retain the pointer as is.
	frame *starlark.Frame
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
func (s *CapturedStacktrace) String() string {
	buf := bytes.Buffer{}
	s.frame.WriteBacktrace(&buf)
	return buf.String()
}

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

	lvl, ok := skip.Uint64()
	if !ok {
		return nil, fmt.Errorf("stacktrace: bad 'skip' value %s", skip)
	}

	f := th.Caller()
	skipped := uint64(0)
	for skipped < lvl && f != nil {
		f = f.Parent()
		skipped++
	}
	if f == nil {
		return nil, fmt.Errorf("stacktrace: the stack is not deep enough to skip %s levels, has only %d frames", skip, skipped)
	}

	return &CapturedStacktrace{f}, nil
})
