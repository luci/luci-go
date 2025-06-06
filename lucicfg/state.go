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
	"sort"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/starlark/interpreter"

	"go.chromium.org/luci/lucicfg/errs"
	"go.chromium.org/luci/lucicfg/graph"
	"go.chromium.org/luci/lucicfg/pkg"
	"go.chromium.org/luci/lucicfg/vars"
)

// State is mutated throughout execution of the script and at the end contains
// the final execution result.
//
// It is available in the implementation of native functions exposed to the
// Starlark side. Starlark code operates with the state exclusively through
// these functions.
//
// Use NewState to construct it.
//
// All Starlark code is executed sequentially in a single goroutine, thus the
// state is not protected by any mutexes.
type State struct {
	Inputs Inputs // all inputs, exactly as passed to Generate.
	Output Output // all generated config files, populated at the end
	Meta   Meta   // lucicfg parameters, settable through Starlark

	visited     []string          // visited Starlark modules from the main package in Inputs
	vars        vars.Vars         // holds state of lucicfg.var() variables
	seq         sequences         // holds state for __native__.sequence_next()
	experiments experiments       // holds the set of registered/enabled experiments
	errors      errors.MultiError // all errors emitted during the generation (if any)
	seenErrs    stringset.Set     // set of all string backtraces in 'errors', for deduping
	failOnErrs  bool              // if true, 'emit_error' aborts the execution

	generators generators     // callbacks that generate config files based on state
	graph      graph.Graph    // the graph with config entities defined so far
	templates  templateCache  // cached parsed text templates, see templates.go
	interner   stringInterner // a general purpose string interner used by other guts
	files      fileCache      // cache of files read from disk, see io.go
	protos     protoCache     // cache of deserialized protos, see protos.go
}

// SourcesToLint is a list of paths in the main package to run the linter on.
//
// Returns an error if executing a remote package. Linting is a purely local
// operation.
func (s *State) SourcesToLint() ([]string, error) {
	if s.Inputs.Entry.Local == nil {
		return nil, errors.New("cannot lint remote packages")
	}
	if s.Inputs.Entry.Package == pkg.LegacyPackageNamePlaceholder {
		// Legacy mode lints only files that were executed for backward
		// compatibility with how it used to work.
		return s.visited, nil
	}
	return s.Inputs.Entry.Local.Sources()
}

// NewState initializes the state.
func NewState(inputs Inputs) *State {
	interner := stringInterner{}
	return &State{
		Inputs:   inputs,
		Meta:     inputs.Meta.Copy(),
		interner: interner,
		files:    newFileCache(interner),
		protos:   newProtoCache(interner),
	}
}

// checkUnconsumedVars returns an error per a provided (via Inputs.Vars), but
// unused (by lucicfg.var(expose_as=...)) variable.
//
// All supplied inputs must be consumed.
func (s *State) checkUnconsumedVars() (errs []error) {
	keys := make([]string, 0, len(s.Inputs.Vars))
	for k := range s.Inputs.Vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if !s.vars.DeclaredExposeAsAliases.Has(k) {
			errs = append(errs, fmt.Errorf("value set by \"-var %s=...\" was not used by Starlark code", k))
		}
	}
	return
}

// clear resets the state, preserving registered vars (but not their values).
//
// Used in tests.
func (s *State) clear() {
	*s = State{
		Inputs:   s.Inputs,
		Meta:     s.Inputs.Meta.Copy(),
		vars:     s.vars,
		interner: s.interner,
		files:    s.files,
		protos:   s.protos,
	}
	s.vars.ClearValues()
}

// err adds errors to the list of errors and returns the list as MultiError,
// deduplicating errors with identical backtraces.
func (s *State) err(err ...error) error {
	if s.seenErrs == nil {
		s.seenErrs = stringset.New(len(err))
	}
	for _, e := range err {
		var bterr errs.Backtracable
		if errors.As(e, &bterr) {
			if s.seenErrs.Add(bterr.Backtrace()) {
				s.errors = append(s.errors, e)
			}
		} else {
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

	// current_module returns a tuple (package, path) with current module info
	// or fails if unknown (i.e. the thread is running a callback called from
	// native code).
	declNative("current_module", func(call nativeCall) (starlark.Value, error) {
		if err := call.unpack(0); err != nil {
			return nil, err
		}
		mod := interpreter.GetThreadModuleKey(call.Thread)
		if mod == nil {
			return nil, fmt.Errorf(
				"current_module: no information about the current module in the thread locals")
		}
		return starlark.Tuple{starlark.String("@" + mod.Package), starlark.String(mod.Path)}, nil
	})

	// clear_state() wipes the state of the generator, for tests.
	declNative("clear_state", func(call nativeCall) (starlark.Value, error) {
		if err := call.unpack(0); err != nil {
			return nil, err
		}
		call.State.clear()
		return starlark.None, nil
	})

	// declare_var(expose_as, preset_value) allocates a new variable, returning
	// its identifier.
	declNative("declare_var", func(call nativeCall) (starlark.Value, error) {
		var exposeAs starlark.String
		var presetValue starlark.Value
		if err := call.unpack(2, &exposeAs, &presetValue); err != nil {
			return nil, err
		}
		return call.State.vars.Declare(call.Thread, exposeAs.GoString(), presetValue)
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
