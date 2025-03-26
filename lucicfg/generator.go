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
//
// All Starlark code is executed sequentially in a single goroutine from inside
// Generate function, thus this package doesn't used any mutexes or other
// synchronization primitives. It is safe to call Generate concurrently though,
// since there's no global shared state, each Generate call operates on its
// own state.
package lucicfg

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"go.starlark.net/lib/json"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/starlark/builtins"
	"go.chromium.org/luci/starlark/interpreter"
	"go.chromium.org/luci/starlark/starlarkproto"

	"go.chromium.org/luci/lucicfg/internal"
	"go.chromium.org/luci/lucicfg/pkg"
	embedded "go.chromium.org/luci/lucicfg/starlark"
)

// stdlibPkg is the lucicfg package with the hardcoded stdlib.
const stdlibPkg = "stdlib"

// Inputs define all inputs for the config generator.
type Inputs struct {
	Entry *pkg.Entry        // the main package and the entry point script
	Meta  *Meta             // defaults for lucicfg own parameters
	Vars  map[string]string // var values passed via `-var key=value` flags

	// Used to setup additional facilities for unit tests.
	testOmitHeader              bool
	testPredeclared             starlark.StringDict
	testThreadModifier          func(th *starlark.Thread)
	testDisableFailureCollector bool
	testVersion                 string
}

// Generate interprets the high-level config.
//
// Returns either a singular error (if the call crashed before executing any
// Starlark) or a multi-error with all errors captured during the execution.
// Some of them may be backtracable.
func Generate(ctx context.Context, in Inputs) (*State, error) {
	state := NewState(in)
	ctx = withState(ctx, state)

	// Do not put frequently changing version string into test outputs.
	ver := Version
	if in.testVersion != "" {
		ver = in.testVersion
	}

	// Should satisfy version constraint of all dependencies.
	pkgMinLucicfg := starlark.Value(starlark.None)
	for _, constraint := range in.Entry.LucicfgVersionConstraints {
		if err := VerifyVersion(constraint, ver); err != nil {
			return nil, err
		}
		asTuple := starlark.Tuple{
			starlark.MakeInt(constraint.Min[0]),
			starlark.MakeInt(constraint.Min[1]),
			starlark.MakeInt(constraint.Min[2]),
		}
		if constraint.Main {
			pkgMinLucicfg = asTuple
			state.experiments.setMinVersion(asTuple)
		}
	}

	// Plumb lint_checks though only for local packages. They don't matter when
	// executing remote packages (linting is a purely local workflow).
	pkgLintChecks := starlark.Value(starlark.None)
	if in.Entry.Local != nil && len(in.Entry.Local.Definition.LintChecks) != 0 {
		var pkgLintChecksTup starlark.Tuple
		for _, check := range in.Entry.Local.Definition.LintChecks {
			pkgLintChecksTup = append(pkgLintChecksTup, starlark.String(check))
		}
		if err := state.Meta.setField("lint_checks", pkgLintChecksTup); err != nil {
			return nil, errors.Annotate(err, "presetting lint_checks").Err()
		}
		pkgLintChecks = pkgLintChecksTup
	}

	// All available symbols implemented in go.
	predeclared := starlark.StringDict{
		// Part of public API of the generator.
		"fail":       builtins.Fail,
		"proto":      starlarkproto.ProtoLib()["proto"],
		"stacktrace": builtins.Stacktrace,
		"struct":     builtins.Struct,
		"json":       json.Module,
		"to_json":    toSortedJSON, // see json.go, deprecated

		// '__native__' is NOT public API. It should be used only through public
		// @stdlib functions.
		"__native__": native(starlark.StringDict{
			// How the generator was launched.
			"version":        versionTuple(ver),
			"entry_point":    starlark.String(in.Entry.Script),
			"main_pkg_path":  starlark.String(in.Entry.Path),
			"main_pkg_name":  starlark.String(in.Entry.Package),
			"var_flags":      asFrozenDict(in.Vars),
			"running_tests":  starlark.Bool(in.testThreadModifier != nil),
			"testing_tweaks": internal.GetTestingTweaks(ctx).ToStruct(),
			// Data pulled from PACKAGE.star, if any.
			"pkg_min_lucicfg": pkgMinLucicfg,
			"pkg_lint_checks": pkgLintChecks,
			// Some built-in utilities implemented in `builtins` package.
			"ctor":          builtins.Ctor,
			"genstruct":     builtins.GenStruct,
			"re_submatches": builtins.RegexpMatcher("submatches"),
			// Built-in proto descriptors.
			"wellknown_descpb":   wellKnownDescSet,
			"googtypes_descpb":   googTypesDescSet,
			"annotations_descpb": annotationsDescSet,
			"validation_descpb":  validateDescSet,
			"lucitypes_descpb":   luciTypesDescSet,
		}),
	}
	for k, v := range in.testPredeclared {
		predeclared[k] = v
	}

	// Expose the hardcoded @stdlib.
	pkgs := embeddedPackages()

	// Create a proto loader, hook up load("@proto//<path>", ...) to load proto
	// modules through it. See ThreadModifier below where it is set as default in
	// the thread. This exposes it to Starlark code, so it can register descriptor
	// sets in it.
	ploader := starlarkproto.NewLoader()
	pkgs["proto"] = func(_ context.Context, path string) (dict starlark.StringDict, _ string, err error) {
		mod, err := ploader.Module(path)
		if err != nil {
			return nil, "", err
		}
		return starlark.StringDict{mod.Name: mod}, "", nil
	}

	// Convert "@main" (as used in PACKAGE.star) to "main" for Packages map.
	mainPkg, found := strings.CutPrefix(in.Entry.Package, "@")
	if !found {
		panic(fmt.Sprintf("unexpected main package name %q, missing @", in.Entry.Package))
	}

	// Add the main package and all its dependencies last to verify they do not
	// clobber predeclared packages.
	var depErr errors.MultiError
	addDep := func(dep string, loader interpreter.Loader) {
		if pkgs[dep] != nil {
			depErr = append(depErr, errors.Reason("dependency %q clashes with a predeclared dependency", dep).Err())
		} else {
			pkgs[dep] = loader
		}
	}
	addDep(mainPkg, in.Entry.Main)
	for dep, loader := range in.Entry.Deps {
		addDep(dep, loader)
	}
	if len(depErr) != 0 {
		return nil, depErr
	}

	// Capture details of fail(...) calls happening inside Starlark code.
	failures := builtins.FailureCollector{}

	// Execute the config script in this environment. Return errors unwrapped so
	// that callers can sniff out various sorts of Starlark errors.
	intr := interpreter.Interpreter{
		Predeclared: predeclared,
		Packages:    pkgs,
		MainPackage: mainPkg,

		PreExec:  func(th *starlark.Thread, _ interpreter.ModuleKey) { state.vars.OpenScope(th) },
		PostExec: func(th *starlark.Thread, _ interpreter.ModuleKey) { state.vars.CloseScope(th) },

		ThreadModifier: func(th *starlark.Thread) {
			starlarkproto.SetDefaultLoader(th, ploader)
			starlarkproto.SetMessageCache(th, &state.protos)
			if !in.testDisableFailureCollector {
				failures.Install(th)
			}
			if in.testThreadModifier != nil {
				in.testThreadModifier(th)
			}
		},

		Options: &syntax.FileOptions{
			Set: true, // allow set(...) datatype
		},
	}

	// Load builtins.star, and then execute the user-supplied script.
	var err error
	if err = intr.LoadBuiltins(ctx, stdlibPkg, "builtins.star"); err == nil {
		_, err = intr.ExecModule(ctx, mainPkg, in.Entry.Script)
	}
	if err != nil {
		if f := failures.LatestFailure(); f != nil {
			err = f // prefer this error, it has custom stack trace
		}
		return nil, state.err(err)
	}

	// Verify all var values provided via Inputs.Vars were actually used by
	// lucicfg.var(expose_as='...') definitions.
	if errs := state.checkUnconsumedVars(); len(errs) != 0 {
		return nil, state.err(errs...)
	}

	// Executing the script (with all its dependencies) populated the graph.
	// Finalize it. This checks there are no dangling edges, freezes the graph,
	// and makes it queryable, so generator callbacks can traverse it.
	if errs := state.graph.Finalize(); len(errs) != 0 {
		return nil, state.err(errs...)
	}

	// The script registered a bunch of callbacks that take the graph and
	// transform it into actual output config files. Run these callbacks now.
	genCtx := newGenCtx()
	if errs := state.generators.call(intr.Thread(ctx), genCtx); len(errs) != 0 {
		return nil, state.err(errs...)
	}
	output, err := genCtx.assembleOutput(!in.testOmitHeader)
	if err != nil {
		return nil, state.err(err)
	}
	state.Output = output

	if len(state.errors) != 0 {
		return nil, state.errors
	}

	// Discover what main package modules we actually executed.
	for _, key := range intr.Visited() {
		if key.Package == mainPkg {
			state.Visited = append(state.Visited, key.Path)
		}
	}

	return state, nil
}

// embeddedPackages makes a map of loaders for embedded Starlark packages.
//
// A directory directly under go.chromium.org/luci/lucicfg/starlark/...
// represents a corresponding starlark package. E.g. files in 'stdlib' directory
// are loadable via load("@stdlib//<path>", ...).
func embeddedPackages() map[string]interpreter.Loader {
	out := make(map[string]interpreter.Loader, 1)
	for _, pkg := range []string{stdlibPkg} {
		sub, err := fs.Sub(embedded.Content, pkg)
		if err != nil {
			panic(fmt.Sprintf("%q is not embedded", pkg))
		}
		out[pkg] = interpreter.FSLoader(sub)
	}
	return out
}

// asFrozenDict converts a map to a frozen Starlark dict.
func asFrozenDict(m map[string]string) *starlark.Dict {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	d := starlark.NewDict(len(m))
	for _, k := range keys {
		d.SetKey(starlark.String(k), starlark.String(m[k]))
	}
	d.Freeze()

	return d
}
