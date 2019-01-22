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
	Code  interpreter.Loader // a package with the user supplied code
	Entry string             // a name of the entry point script in this package

	TextPBHeader string // a header to put into generated textpb files

	// Used to setup additional facilities for unit tests.
	testPredeclared             starlark.StringDict
	testThreadModifier          func(th *starlark.Thread)
	testDisableFailureCollector bool
}

// Generate interprets the high-level config.
//
// Returns a multi-error with all captured errors. Some of them may implement
// BacktracableError interface.
func Generate(ctx context.Context, in Inputs) (*State, error) {
	state := &State{Inputs: in}
	ctx = withState(ctx, state)

	// All available functions implemented in go.
	predeclared := starlark.StringDict{
		// Part of public API of the generator.
		"fail":       builtins.Fail,
		"proto":      starlarkproto.ProtoLib()["proto"],
		"stacktrace": builtins.Stacktrace,
		"struct":     builtins.Struct,
		"to_json":    builtins.ToJSON,

		// '__native__' is NOT public API. It should be used only through public
		// @stdlib functions.
		"__native__": native(starlark.StringDict{
			"ctor":          builtins.Ctor,
			"genstruct":     builtins.GenStruct,
			"re_submatches": builtins.RegexpMatcher("submatches"),
		}),
	}
	for k, v := range in.testPredeclared {
		predeclared[k] = v
	}

	// Expose @stdlib, @proto and __main__ package. All have no externally
	// observable state of their own, but they call low-level __native__.*
	// functions that manipulate 'state' by getting it through the context.
	pkgs := embeddedPackages()
	pkgs[interpreter.MainPkg] = in.Code
	pkgs["proto"] = protoLoader() // see protos.go

	// Capture details of fail(...) calls happening inside Starlark code.
	failures := builtins.FailureCollector{}

	// Execute the config script in this environment. Return errors unwrapped so
	// that callers can sniff out various sorts of Starlark errors.
	intr := interpreter.Interpreter{
		Predeclared: predeclared,
		Packages:    pkgs,
		ThreadModifier: func(th *starlark.Thread) {
			if !in.testDisableFailureCollector {
				failures.Install(th)
			}
			if in.testThreadModifier != nil {
				in.testThreadModifier(th)
			}
		},
	}

	// Load builtins.star, and then execute the user-supplied script.
	var err error
	if err = intr.Init(ctx); err == nil {
		_, err = intr.LoadModule(ctx, interpreter.MainPkg, in.Entry)
	}
	if err != nil {
		if f := failures.LatestFailure(); f != nil {
			err = f // prefer this error, it has custom stack trace
		}
		return nil, state.err(err)
	}

	// Executing the script (with all its dependencies) populated the graph.
	// Finalize it. This checks there are no dangling edges, freezes the graph,
	// and makes it queryable, so generator callbacks can traverse it.
	if errs := state.graph.Finalize(); len(errs) != 0 {
		return nil, state.err(errs...)
	}

	// The script registered a bunch of callbacks that take the graph and
	// transform it into actual config files (living in a config set). Run these
	// callbacks now.
	genCtx := newGenCtx()
	if errs := state.generators.call(intr.Thread(ctx), genCtx); len(errs) != 0 {
		return nil, state.err(errs...)
	}
	cfgs, err := genCtx.configSet.renderWithTextProto(in.TextPBHeader)
	if err != nil {
		return nil, state.err(err)
	}
	state.Configs = cfgs

	if len(state.errors) != 0 {
		return nil, state.errors
	}
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
