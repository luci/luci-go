// Copyright 2025 The LUCI Authors.
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

package pkg

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/starlark/builtins"
	"go.chromium.org/luci/starlark/interpreter"

	embedded "go.chromium.org/luci/lucicfg/starlark"
)

// LucicfgVersion is "<major>.<minor>.<patch>" with lucicfg version.
type LucicfgVersion [3]int

// IsZero is true if the version is unpopulated.
func (v LucicfgVersion) IsZero() bool {
	return v[0] == 0 && v[1] == 0 && v[2] == 0
}

// String returns "<major>.<minor>.<patch>" representation.
func (v LucicfgVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2])
}

// Older returns true if `v` is older than `other`.
func (v LucicfgVersion) Older(other LucicfgVersion) bool {
	return slices.Compare(v[:], other[:]) < 0
}

// Definition represents a loaded PACKAGE.star file.
type Definition struct {
	// Name is the declare package name.
	Name string

	// MinLucicfgVersion is the declared minimum required lucicfg version.
	MinLucicfgVersion LucicfgVersion

	// Entrypoints is the declared list of entry points as slash-separated paths
	// relative to the package root.
	Entrypoints []string
}

// LoaderValidator can do extra validation checks right when loading
// PACKAGE.star.
//
// This is useful for attaching Starlark stack traces to validation errors.
type LoaderValidator interface {
	// ValidateEntrypoint returns an error if the given entrypoint is missing.
	ValidateEntrypoint(ctx context.Context, entrypoint string) error
}

// NoopLoaderValidator implements LoaderValidator by doing nothing.
//
// Useful in cases when PACKAGE.star is already known to be valid.
type NoopLoaderValidator struct{}

// ValidateEntrypoint implements LoaderValidator interface.
func (NoopLoaderValidator) ValidateEntrypoint(ctx context.Context, entrypoint string) error {
	return nil
}

// LoadDefinition interprets PACKAGE.star file.
//
// Does initial validation by checking syntax of all values and by delegating
// some checks to the given validator.
//
// Doesn't validate dependencies are up-to-date or make sense at all.
func LoadDefinition(ctx context.Context, body []byte, val LoaderValidator) (*Definition, error) {
	// State is mutated while executing PACKAGE.star file.
	state := &state{val: val}

	// All native symbols exposed to PACKAGE.star file.
	native := starlark.StringDict{}
	state.declNative(native)

	// Code with Starlark API available to PACKAGE.star.
	pkgFS, err := fs.Sub(embedded.Content, "pkg")
	if err != nil {
		panic("pkg is not embedded, inconceivable")
	}

	intr := interpreter.Interpreter{
		Predeclared: starlark.StringDict{
			// Publicly available symbols.
			"fail":   builtins.Fail,
			"struct": builtins.Struct,
			// Part of the guts, not a public API. Used by pkg/builtins.star.
			"__native__": starlarkstruct.FromStringDict(starlark.String("__native__"), native),
		},

		Packages: map[string]interpreter.Loader{
			// Loader with PACKAGE.star itself (and only it!).
			interpreter.MainPkg: interpreter.MemoryLoader(map[string]string{
				PackageScript: string(body),
			}),
			// Minimal stdlib implementing APIs accessible to PACKAGE.star.
			interpreter.StdlibPkg: interpreter.FSLoader(pkgFS),
		},

		// Allow printf-debugging.
		Logger: func(file string, line int, message string) {
			logging.Infof(ctx, "[%s:%d] %s", file, line, message)
		},

		// Even though there's nothing for load(...) and exec(...) to load (since
		// we populated only PACKAGE.star in the loader), forbid calling them with
		// a nicer error message.
		ForbidLoad: "load(...) is not allowed in PACKAGE.star file",
		ForbidExec: "exec(...) is not allowed in PACKAGE.star file",
	}

	// Load builtins.star, and then execute the PACKAGE.star script.
	if err = intr.Init(ctx); err == nil {
		_, err = intr.ExecModule(ctx, interpreter.MainPkg, PackageScript)
	}
	if err != nil {
		return nil, err
	}

	if !state.declareCalled {
		return nil, errors.Reason("PACKAGE.star must call pkg.declare(...)").Err()
	}

	return &state.def, nil
}

////////////////////////////////////////////////////////////////////////////////

type state struct {
	val           LoaderValidator
	declareCalled bool
	def           Definition
}

type nativeCall struct {
	thread *starlark.Thread
	fn     *starlark.Builtin
	args   starlark.Tuple
	kwargs []starlark.Tuple
}

// unpack unpacks the positional arguments into corresponding variables.
func (c *nativeCall) unpack(min int, vars ...any) error {
	return starlark.UnpackPositionalArgs(c.fn.Name(), c.args, c.kwargs, min, vars...)
}

// declNative registers all __native__.<name> functions.
func (s *state) declNative(native starlark.StringDict) {
	decl := func(name string, requiredDeclareFirst bool, nativeFn func(ctx context.Context, call nativeCall) (starlark.Value, error)) {
		native[name] = starlark.NewBuiltin(name, func(th *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			if requiredDeclareFirst && !s.declareCalled {
				return nil, errors.New("pkg.declare(...) must be the first statement in PACKAGE.star")
			}
			return nativeFn(interpreter.Context(th), nativeCall{
				thread: th,
				fn:     fn,
				args:   args,
				kwargs: kwargs,
			})
		})
	}

	decl("declare", false, s.declare)
	decl("entrypoint", true, s.entrypoint)
}

func (s *state) declare(ctx context.Context, call nativeCall) (starlark.Value, error) {
	var name, lucicfg starlark.String
	if err := call.unpack(2, &name, &lucicfg); err != nil {
		return nil, err
	}

	if s.declareCalled {
		return nil, errors.New("pkg.declare(...) can be called at most once")
	}
	s.declareCalled = true

	s.def.Name = name.GoString()
	if err := ValidateName(s.def.Name); err != nil {
		return nil, errors.Annotate(err, "bad package name %q", s.def.Name).Err()
	}

	var err error
	if s.def.MinLucicfgVersion, err = ValidateVersion(lucicfg.GoString()); err != nil {
		return nil, errors.Annotate(err, "bad lucicfg version string %q", lucicfg.GoString()).Err()
	}

	return starlark.None, nil
}

func (s *state) entrypoint(ctx context.Context, call nativeCall) (starlark.Value, error) {
	var relPath starlark.String
	if err := call.unpack(1, &relPath); err != nil {
		return nil, err
	}
	clean := path.Clean(relPath.GoString())
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return nil, errors.Reason("entry point path must be within the package, got %q", relPath.GoString()).Err()
	}
	if clean != relPath.GoString() {
		return nil, errors.Reason("entry point path must be in normalized form (i.e. %q instead of %q)", clean, relPath.GoString()).Err()
	}
	for _, p := range s.def.Entrypoints {
		if p == clean {
			return nil, errors.Reason("entry point %q was already defined", p).Err()
		}
	}
	if err := s.val.ValidateEntrypoint(ctx, clean); err != nil {
		return nil, errors.Annotate(err, "entry point %q", clean).Err()
	}
	s.def.Entrypoints = append(s.def.Entrypoints, clean)
	return starlark.None, nil
}
