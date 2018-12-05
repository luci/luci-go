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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/lucicfg/graph"
)

// State is mutated throughout execution of the script and at the end contains
// the final execution result.
//
// It is available in the implementation of native functions exposed to the
// Starlark side. Starlark code operates with the state exclusively through
// these functions.
type State struct {
	Inputs  Inputs            // all inputs, exactly as passed to Generate.
	Configs map[string]string // all generated config files, populated at the end

	errors     errors.MultiError // all errors emitted during the generation (if any)
	generators generators        // callbacks that generate config files based on state
	graph      graph.Graph       // the graph with config entities defined so far
}

// clear resets the state.
func (s *State) clear() {
	*s = State{Inputs: s.Inputs}
}

// err adds errors to the list of errors and returns the list as MultiError.
func (s *State) err(err ...error) error {
	s.errors = append(s.errors, err...)
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

	// clear_state() wipes the state of the generator, for tests.
	declNative("clear_state", func(call nativeCall) (starlark.Value, error) {
		if err := call.unpack(0); err != nil {
			return nil, err
		}
		call.State.clear()
		return starlark.None, nil
	})
}
