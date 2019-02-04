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

	"go.starlark.net/starlark"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/lucicfg/graph"
	"go.chromium.org/luci/lucicfg/vars"
	"go.chromium.org/luci/starlark/interpreter"
)

// State is mutated throughout execution of the script and at the end contains
// the final execution result.
//
// It is available in the implementation of native functions exposed to the
// Starlark side. Starlark code operates with the state exclusively through
// these functions.
type State struct {
	Inputs  Inputs    // all inputs, exactly as passed to Generate.
	Configs ConfigSet // all generated config files, populated at the end
	Meta    Meta      // lucicfg parameters, settable through Starlark

	vars       vars.Vars         // holds state of lucicfg.var() variables
	errors     errors.MultiError // all errors emitted during the generation (if any)
	seenErrs   stringset.Set     // set of all string backtraces in 'errors', for deduping
	failOnErrs bool              // if true, 'emit_error' aborts the execution

	generators generators  // callbacks that generate config files based on state
	graph      graph.Graph // the graph with config entities defined so far
}

// clear resets the state.
func (s *State) clear() {
	*s = State{Inputs: s.Inputs, vars: s.vars}
	s.vars.ClearValues()
}

// err adds errors to the list of errors and returns the list as MultiError,
// deduplicating errors with identical backtraces.
func (s *State) err(err ...error) error {
	if s.seenErrs == nil {
		s.seenErrs = stringset.New(len(err))
	}
	for _, e := range err {
		if bt, _ := e.(BacktracableError); bt == nil || s.seenErrs.Add(bt.Backtrace()) {
			s.errors = append(s.errors, e)
		}
	}
	return s.errors
}

var stateCtxKey = "lucicfg.State"

// withState puts *State into the context, to be accessed by native functions.
func withState(ctx context.Context, s *State) context.Context {
	return context.WithValue(ctx, &stateCtxKey, s)
}

// ctxState pulls out *State from the context, as put there by withState.
//
// Panics if not there.
func ctxState(ctx context.Context) *State {
	return ctx.Value(&stateCtxKey).(*State)
}

func init() {
	// graph() returns a graph with config entities defines thus far.
	declNative("graph", func(call nativeCall) (starlark.Value, error) {
		if err := call.unpack(0); err != nil {
			return nil, err
		}
		return &call.State.graph, nil
	})

	// interpreter_context() returns either 'EXEC', 'LOAD', 'GEN' or 'UNKNOWN'.
	//
	// EXEC: inside a module that was 'exec'-ed.
	// LOAD: inside a module that was 'load'-ed.
	// GEN: inside a generator callback.
	// UNKNOWN: inside some other callback called from native code.
	declNative("interpreter_context", func(call nativeCall) (starlark.Value, error) {
		if err := call.unpack(0); err != nil {
			return nil, err
		}
		if call.State.generators.runningNow {
			return starlark.String("GEN"), nil
		}
		var val starlark.String
		switch interpreter.GetThreadKind(call.Thread) {
		case interpreter.ThreadLoading:
			val = "LOAD"
		case interpreter.ThreadExecing:
			val = "EXEC"
		default:
			val = "UNKNOWN"
		}
		return val, nil
	})

	// clear_state() wipes the state of the generator, for tests.
	declNative("clear_state", func(call nativeCall) (starlark.Value, error) {
		if err := call.unpack(0); err != nil {
			return nil, err
		}
		call.State.clear()
		return starlark.None, nil
	})

	// declare_var() allocates a new variable, returning its identifier.
	declNative("declare_var", func(call nativeCall) (starlark.Value, error) {
		if err := call.unpack(0); err != nil {
			return nil, err
		}
		return call.State.vars.Declare(), nil
	})

	// set_var(var_id, val) sets the value of a variable.
	declNative("set_var", func(call nativeCall) (starlark.Value, error) {
		var id vars.ID
		var val starlark.Value
		if err := call.unpack(2, &id, &val); err != nil {
			return nil, err
		}
		if err := call.State.vars.Set(call.Thread, id, val); err != nil {
			return nil, err
		}
		return starlark.None, nil
	})

	// get_var(var_id, default) returns variable's value.
	declNative("get_var", func(call nativeCall) (starlark.Value, error) {
		var id vars.ID
		var def starlark.Value
		if err := call.unpack(2, &id, &def); err != nil {
			return nil, err
		}
		return call.State.vars.Get(call.Thread, id, def)
	})
}
