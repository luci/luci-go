// Copyright 2019 The LUCI Authors.
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
	"fmt"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/starlark/builtins"
)

// generators is a list of registered generator callbacks.
//
// It lives in State. Generators are executed sequentially after all Starlark
// code is loaded. They examine the state and generate configs based on it.
type generators struct {
	gen        []starlark.Callable
	runningNow bool // true while inside 'call'
}

// add registers a new generator callback.
func (g *generators) add(cb starlark.Callable) error {
	if g.runningNow {
		return fmt.Errorf("can't add a generator while already running them")
	}
	g.gen = append(g.gen, cb)
	return nil
}

// call calls all registered callbacks sequentially, collecting all errors.
func (g *generators) call(th *starlark.Thread, ctx *genCtx) (errs errors.MultiError) {
	if g.runningNow {
		return errors.MultiError{
			fmt.Errorf("can't call generators while they are already running"),
		}
	}
	g.runningNow = true
	defer func() { g.runningNow = false }()

	fc := builtins.GetFailureCollector(th)

	for _, cb := range g.gen {
		if fc != nil {
			fc.Clear()
		}
		if _, err := starlark.Call(th, cb, starlark.Tuple{ctx}, nil); err != nil {
			if fc != nil && fc.LatestFailure() != nil {
				// Prefer this error, it has custom stack trace.
				errs = append(errs, fc.LatestFailure())
			} else {
				errs = append(errs, err)
			}
		}
	}
	return
}

func init() {
	// add_generator(cb) registers a callback that is called at the end of the
	// execution to generate or mutate produced configs.
	declNative("add_generator", func(call nativeCall) (starlark.Value, error) {
		var cb starlark.Callable
		if err := call.unpack(1, &cb); err != nil {
			return nil, err
		}
		return starlark.None, call.State.generators.add(cb)
	})

	// call_generators(ctx) calls all registered generators, useful in tests.
	declNative("call_generators", func(call nativeCall) (starlark.Value, error) {
		var ctx *genCtx
		if err := call.unpack(1, &ctx); err != nil {
			return nil, err
		}
		switch errs := call.State.generators.call(call.Thread, ctx); {
		case len(errs) == 0:
			return starlark.None, nil
		case len(errs) == 1:
			return starlark.None, errs[0]
		default:
			return starlark.None, errs
		}
	})
}
