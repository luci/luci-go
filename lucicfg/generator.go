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

// Package lucicfg contains LUCI config generator.
package lucicfg

import (
	"context"
	"fmt"
	"strings"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/starlark/builtins"
	"go.chromium.org/luci/starlark/interpreter"
	"go.chromium.org/luci/starlark/starlarkproto"

	generated "go.chromium.org/luci/lucicfg/starlark"
)

// Inputs define all inputs for the config generator.
type Inputs struct {
	Main interpreter.Loader // a package with the entry LUCI.star file

	// Used to setup additional facilities for unit tests.
	testPredeclared    starlark.StringDict
	testThreadModified func(th *starlark.Thread)
}

// State is mutated throughout execution of the script and at the end contains
// the final execution result.
//
// It is available in the implementation of native functions exposed to the
// Starlark side. Starlark code operates with the state exclusively through
// these functions.
type State struct {
	Inputs Inputs // all inputs, exactly as passed to Generate.

	Greetings []string // this is just for demo purposes
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

// Generate interprets the high-level config.
func Generate(ctx context.Context, in Inputs) (*State, error) {
	state := &State{Inputs: in}
	ctx = withState(ctx, state)

	// All available functions implemented in go.
	predeclared := starlark.StringDict{
		// Part of public API of the generator.
		"fail":    builtins.Fail,
		"proto":   starlarkproto.ProtoLib()["proto"],
		"struct":  builtins.Struct,
		"to_json": builtins.ToJSON,

		// '__native__' is NOT public API. It should be used only through public
		// @stdlib functions.
		"__native__": native(),
	}
	for k, v := range in.testPredeclared {
		predeclared[k] = v
	}

	// Expose @stdlib, @proto and __main__ package. All have no externally
	// observable state of their own, but they call low-level __native__.*
	// functions that manipulate 'state' by getting it through the context.
	pkgs := embeddedPackages()
	pkgs[interpreter.MainPkg] = in.Main
	pkgs["proto"] = protoLoader() // see protos.go

	// Execute LUCI.star in this environment. Return errors unwrapped so that
	// callers can sniff out various sorts of Starlark errors.
	intr := interpreter.Interpreter{
		Predeclared:    predeclared,
		Packages:       pkgs,
		ThreadModifier: in.testThreadModified,
	}
	if err := intr.Init(ctx); err != nil {
		return nil, err
	}
	if _, err := intr.LoadModule(ctx, interpreter.MainPkg, "LUCI.star"); err != nil {
		return nil, err
	}

	// TODO(vadimsh): There'll likely be more stages of the execution. LoadModule
	// above only loads all starlark code, we may want to call some of callbacks
	// it has registered.

	return state, nil
}

// embeddedPackages makes a map of loaders for embedded Starlark packages.
//
// Each directory directly under go.chromium.org/luci/lucicfg/starlark/...
// represents a corresponding starlark package. E.g. files in 'stdlib' directory
// are loadable via load("@stdlib//<path>", ...).
func embeddedPackages() map[string]interpreter.Loader {
	perRoot := map[string]map[string]string{}

	for path, data := range generated.Assets() {
		chunks := strings.SplitN(path, "/", 2)
		if len(chunks) != 2 {
			panic(fmt.Sprintf("forbidden *.star outside the package dir: %s", path))
		}
		root, rel := chunks[0], chunks[1]
		m := perRoot[root]
		if m == nil {
			m = make(map[string]string, 1)
			perRoot[root] = m
		}
		m[rel] = data
	}

	loaders := make(map[string]interpreter.Loader, len(perRoot))
	for pkg, files := range perRoot {
		loaders[pkg] = interpreter.MemoryLoader(files)
	}
	return loaders
}
