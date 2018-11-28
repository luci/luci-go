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

type stacktraceObj struct {
	// Note that starlark.Frame is immutable and has NO reference to the local
	// dict of a module/function, so its fine to retain the pointer as is.
	frame *starlark.Frame
}

func (*stacktraceObj) Type() string          { return "stacktrace" }
func (*stacktraceObj) Freeze()               {}
func (*stacktraceObj) Truth() starlark.Bool  { return starlark.True }
func (*stacktraceObj) Hash() (uint32, error) { return 0, errors.New("stacktrace is unhashable") }

func (s *stacktraceObj) String() string {
	buf := bytes.Buffer{}
	s.frame.WriteBacktrace(&buf)
	return buf.String()
}

// Stacktrace is stacktrace(level) builtin.
//
//  def stacktrace(skip=0):
//    """Capture and returns a stack trace of the caller.
//
//    Args:
//      skip: how many inner most frames to skip.
//    """
//
// A captured stacktrace is an opaque object that can be stringified to get a
// nice looking trace (e.g. for error messages).
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

	return &stacktraceObj{f}, nil
})
