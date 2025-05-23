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
	"context"
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"go.chromium.org/luci/starlark/interpreter"
)

// nativeCall carries arguments for nativeFunc to avoid ridiculously long
// func declarations.
type nativeCall struct {
	Ctx    context.Context
	State  *State
	Thread *starlark.Thread
	Fn     *starlark.Builtin
	Args   starlark.Tuple
	Kwargs []starlark.Tuple
}

// unpack unpacks the positional arguments into corresponding variables.
//
// See starlark.UnpackPositionalArgs for more info.
func (c *nativeCall) unpack(min int, vars ...any) error {
	return starlark.UnpackPositionalArgs(c.Fn.Name(), c.Args, c.Kwargs, min, vars...)
}

// nativeFunc is callable from Starlark side via __native__.<name>(...).
//
// It gets the state of the generator and can potentially examine or mutate it.
// It must not depend on any other global state.
type nativeFunc func(call nativeCall) (starlark.Value, error)

// All native functions exposed to the starlark code.
//
// Populated during init() time via declNative(...) calls.
var nativeFuncs = map[string]nativeFunc{}

// declNative registers a native function during init() time.
//
// Panics if such function has already been registered.
func declNative(name string, f nativeFunc) {
	if nativeFuncs[name] != nil {
		panic(fmt.Sprintf("native function %q is already registered", name))
	}
	nativeFuncs[name] = f
}

// native returns a starlark struct with all registered native functions.
//
// This struct is put into the global starlark namespace as '__native__'. Native
// functions can access the generator state through the context associated with
// the starlark thread that executes them.
func native(extra starlark.StringDict) starlark.Value {
	dict := make(starlark.StringDict, len(nativeFuncs)+len(extra))
	for name, cb := range nativeFuncs {
		dict[name] = starlark.NewBuiltin(name, func(th *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			ctx := interpreter.Context(th)
			return cb(nativeCall{
				Ctx:    ctx,
				State:  ctxState(ctx),
				Thread: th,
				Fn:     fn,
				Args:   args,
				Kwargs: kwargs,
			})
		})
	}
	for k, v := range extra {
		dict[k] = v
	}
	return starlarkstruct.FromStringDict(starlark.String("__native__"), dict)
}
