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

package interpreter

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"
)

// failImpl implements fail() builtin.
//
// It aborts the script with a fatal error.
func failImpl(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(kwargs) != 0 {
		return nil, fmt.Errorf("'fail' doesn't accept kwargs")
	}
	if len(args) != 1 {
		return nil, fmt.Errorf("'fail' got %d arguments, wants 1", len(args))
	}
	msg, ok := starlark.AsString(args[0])
	if !ok {
		msg = args[0].String()
	}
	// Note: starlark.ExecFile returns *EvalError on errors which have the
	// original error flattened into a string, so returning a structured error
	// here makes no difference.
	return nil, errors.New(msg)
}
