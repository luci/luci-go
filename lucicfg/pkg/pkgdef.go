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

	"go.chromium.org/luci/common/data/stringset"
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
	// LintChecks is what was passed to pkg.options.lint_checks(...).
	LintChecks []string
	// FmtRules are all registered format rule sets.
	FmtRules []*FmtRule
	// Resources are all declared resource patterns (positive and negative).
	Resources []string
	// Deps are declared dependencies (validated only syntactically).
	Deps []*DepDecl
}

// FmtRule represents a registered pkg.options.fmt_rule(...), see its doc.
type FmtRule struct {
	// Stack is where this rule was declared, if known.
	Stack *builtins.CapturedStacktrace
	// Paths this rule applies to.
	Paths []string
	// SortFunctionArgs is true if need to order function call args.
	SortFunctionArgs bool
	// SortFunctionArgsOrder defines ordering of known arguments.
	SortFunctionArgsOrder []string
}

// Equal is true if `a` equals `b`, ignoring Stack.
func (a *FmtRule) Equal(b *FmtRule) bool {
	return slices.Equal(a.Paths, b.Paths) &&
		a.SortFunctionArgs == b.SortFunctionArgs &&
		slices.Equal(a.SortFunctionArgsOrder, b.SortFunctionArgsOrder)
}

// DepDecl represents a declared dependency on another module.
type DepDecl struct {
	// Stack is where this dependency was declared, if known.
	Stack *builtins.CapturedStacktrace

	// Name is the name of the package being dependent on.
	Name string

	// LocalPath is set to a slash-separated relative path (perhaps with "..")
	// if this is a local dependency: a dependency on a package within the same
	// repository as the current PACKAGE.star.
	//
	// Unset for remote dependencies.
	LocalPath string

	// Host is a googlesource host of the remote dependency.
	//
	// Unset for local dependencies.
	Host string

	// Repo is the repository name on the host.
	//
	// Unset for local dependencies.
	Repo string

	// Ref is a git ref (e.g. "refs/heads/main") in the repository.
	//
	// Unset for local dependencies.
	Ref string

	// Path is a path within the repository.
	//
	// Unset for local dependencies.
	Path string

	// Revision is a full git commit specifying a constraint on the required
	// remote version.
	//
	// In the final resolved version set, this dependency will be at this or
	// newer (based on the git log for Ref) revision.
	//
	// Unset for local dependencies.
	Revision string
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
//
// The returned error may be backtracable.
func LoadDefinition(ctx context.Context, body []byte, val LoaderValidator) (*Definition, error) {
	// State is mutated while executing PACKAGE.star file.
	state := &state{val: val}

	// All native symbols exposed to PACKAGE.star file.
	native := starlark.StringDict{
		"stacktrace": builtins.Stacktrace,
		"ctor":       builtins.Ctor,
		"genstruct":  builtins.GenStruct,
	}
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
			"pkgmain": interpreter.MemoryLoader(map[string]string{
				PackageScript: string(body),
			}),
			// Minimal library implementing APIs accessible to PACKAGE.star.
			"pkglib": interpreter.FSLoader(pkgFS),
		},
		MainPackage: "pkgmain",

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
	if err = intr.LoadBuiltins(ctx, "pkglib", "builtins.star"); err == nil {
		_, err = intr.ExecModule(ctx, "pkgmain", PackageScript)
	}
	if err != nil {
		return nil, err
	}

	if !state.declareCalled {
		return nil, errors.Reason("PACKAGE.star must call pkg.declare(...)").Err()
	}

	return &state.def, nil
}

// IsReservedPackageName returns true if the given package is reserved and not
// allowed to appear inside pkg.declare(...) definition.
func IsReservedPackageName(name string) bool {
	return name == LegacyPackageNamePlaceholder || name == "@stdlib" || name == "@proto"
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

// cleanPath normalizes a path.
func cleanPath(p string) string {
	return path.Clean(strings.Replace(p, "\\", "/", -1))
}

// declNative registers all __native__.<name> functions.
func (s *state) declNative(native starlark.StringDict) {
	decl := func(name string, requiredDeclareFirst bool, nativeFn func(ctx context.Context, call nativeCall) (starlark.Value, error)) {
		native[name] = starlark.NewBuiltin(name, func(th *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			// Forbid calling __native__.* directly from PACKAGE.star scripts, only
			// wrappers in pkg/builtins.star can use this API, since they implement
			// important extra validation checks that Go code assumes.
			if frame := th.CallFrame(1); frame.Pos.Filename() != "@pkglib//builtins.star" {
				return nil, errors.New("forbidden direct call to __native__ API")
			}
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

	// Helpers.
	decl("validate_path", false, s.validatePath)

	// Declarations.
	decl("declare", false, s.declare)
	decl("entrypoint", true, s.entrypoint)
	decl("lint_checks", true, s.lintChecks)
	decl("fmt_rules", true, s.fmtRules)
	decl("resources", true, s.resources)
	decl("depend", true, s.depend)
}

func (s *state) validatePath(ctx context.Context, call nativeCall) (starlark.Value, error) {
	var val starlark.String
	var allowDots starlark.Bool
	if err := call.unpack(2, &val, &allowDots); err != nil {
		return nil, err
	}
	clean := cleanPath(val.GoString())
	if clean != val.GoString() {
		return starlark.String(fmt.Sprintf("the path must be in normalized form (i.e. %q instead of %q)", clean, val.GoString())), nil
	}
	if allowDots != starlark.True && (clean == ".." || strings.HasPrefix(clean, "../")) {
		return starlark.String("the path must be within the package"), nil
	}
	return starlark.None, nil
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
	if IsReservedPackageName(s.def.Name) {
		return nil, errors.Reason("bad package name %q: reserved", s.def.Name).Err()
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
	entrypoint := relPath.GoString()
	for _, p := range s.def.Entrypoints {
		if p == entrypoint {
			return nil, errors.Reason("entry point %q was already defined", p).Err()
		}
	}
	if err := s.val.ValidateEntrypoint(ctx, entrypoint); err != nil {
		return nil, errors.Annotate(err, "entry point %q", entrypoint).Err()
	}
	s.def.Entrypoints = append(s.def.Entrypoints, entrypoint)
	return starlark.None, nil
}

func (s *state) lintChecks(ctx context.Context, call nativeCall) (starlark.Value, error) {
	var checks starlark.Tuple
	if err := call.unpack(1, &checks); err != nil {
		return nil, err
	}
	if len(s.def.LintChecks) != 0 {
		return nil, errors.Reason("pkg.options.lint_checks(...) can be called at most once").Err()
	}
	s.def.LintChecks = make([]string, len(checks))
	for i, val := range checks {
		s.def.LintChecks[i] = val.(starlark.String).GoString()
	}
	return starlark.None, nil
}

func (s *state) fmtRules(ctx context.Context, call nativeCall) (starlark.Value, error) {
	var stack *builtins.CapturedStacktrace
	var pathsTup starlark.Tuple
	var argsSortVal starlark.Value
	if err := call.unpack(3, &stack, &pathsTup, &argsSortVal); err != nil {
		return nil, err
	}

	rule := FmtRule{
		Stack:            stack,
		Paths:            make([]string, len(pathsTup)),
		SortFunctionArgs: argsSortVal != starlark.None,
	}

	// Validate there are no dup paths (other aspects are validated in Starlark).
	seenPaths := stringset.New(len(pathsTup))
	for i, v := range pathsTup {
		rel := v.(starlark.String).GoString()
		if !seenPaths.Add(rel) {
			return nil, errors.Reason("invalid paths: %q is specified more than once", rel).Err()
		}
		rule.Paths[i] = rel
	}

	// Validate function_args_sort (the type was already validated in starlark).
	if argsSortVal != starlark.None {
		rule.SortFunctionArgsOrder = make([]string, len(argsSortVal.(starlark.Tuple)))
		seenArgs := stringset.New(len(rule.SortFunctionArgsOrder))
		for i, v := range argsSortVal.(starlark.Tuple) {
			arg := v.(starlark.String).GoString()
			if !seenArgs.Add(arg) {
				return nil, errors.Reason("invalid function_args_sort: %q is specified more than once", arg).Err()
			}
			rule.SortFunctionArgsOrder[i] = arg
		}
	}

	// Check there are no rules that cover the exact same path.
	for _, r := range s.def.FmtRules {
		for _, p := range r.Paths {
			if seenPaths.Has(p) {
				return nil, errors.Reason("path %q is already covered by an existing rule defined at\n%s", p, r.Stack).Err()
			}
		}
	}
	s.def.FmtRules = append(s.def.FmtRules, &rule)

	return starlark.None, nil
}

func (s *state) resources(ctx context.Context, call nativeCall) (starlark.Value, error) {
	var patterns starlark.Tuple
	if err := call.unpack(1, &patterns); err != nil {
		return nil, err
	}
	for _, pat := range patterns {
		str := pat.(starlark.String).GoString()
		if slices.Contains(s.def.Resources, str) {
			return nil, errors.Reason("resource pattern %q is declared more than once", str).Err()
		}
		s.def.Resources = append(s.def.Resources, str)
	}
	return starlark.None, nil
}

func (s *state) depend(ctx context.Context, call nativeCall) (starlark.Value, error) {
	var stack *builtins.CapturedStacktrace
	var name starlark.String
	var source *starlarkstruct.Struct
	if err := call.unpack(3, &stack, &name, &source); err != nil {
		return nil, err
	}

	dep := DepDecl{
		Stack: stack,
		Name:  name.GoString(),
	}
	if err := ValidateName(dep.Name); err != nil {
		return nil, errors.Reason(`bad "name": %s`, err).Err()
	}

	// sourceField reads a string field of pkg.source.ref structs.
	sourceField := func(key string) string {
		val, err := source.Attr(key)
		if err != nil {
			panic(err) // unknown attr, should not happen
		}
		switch val := val.(type) {
		case starlark.NoneType:
			return ""
		case starlark.String:
			return val.GoString()
		default:
			panic(fmt.Sprintf("source.%s is not a string: %s", key, val))
		}
	}

	// Note: basic types and presence of fields were already validated.
	dep.LocalPath = sourceField("local_path")
	dep.Host = sourceField("host")
	dep.Repo = sourceField("repo")
	dep.Ref = sourceField("ref")
	dep.Path = sourceField("path")
	dep.Revision = sourceField("revision")

	dup := slices.IndexFunc(s.def.Deps, func(existing *DepDecl) bool {
		return existing.Name == dep.Name
	})
	if dup != -1 {
		return nil, errors.Reason("dependency on %q was already declared at\n%s", dep.Name, s.def.Deps[dup].Stack).Err()
	}

	s.def.Deps = append(s.def.Deps, &dep)
	return starlark.None, nil
}
