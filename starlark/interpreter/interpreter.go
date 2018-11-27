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

// Package interpreter contains Starlark interpreter.
//
// It supports loading Starlark programs that consist of many files that
// reference each other, thus allowing decomposing code into small logical
// chunks and enabling code reuse.
//
// Modules and packages
//
// Two main new concepts are modules and packages. A package is a collection
// of Starlark files living under the same root. A module is just one such
// Starlark file.
//
// Modules within a single package can load each other (via 'load' statement)
// using their root-relative paths that start with "//". Modules are executed
// exactly once when they are loaded for the first time. All subsequent
// load("//module/path", ...) calls just reuse the cached dict of the executed
// module.
//
// Packages also have identifiers, though they are local to the interpreter
// (e.g. not defined anywhere in the package itself). For that reason they are
// called "package aliases".
//
// Modules from one package may load modules from another package by using
// the following syntax:
//
//   load("@<package alias>//<path within the package>", ...)
//
// Presently the mapping between a package alias (e.g. 'stdlib') and package's
// source code (a collection of *.star files under a single root) is static and
// supplied by Go code that uses the interpreter. This may change in the future
// to allow loading packages dynamically, e.g. over the network.
//
// Special packages
//
// There are two special package aliases recognized natively by the interpreter:
// '__main__' and 'stdlib'.
//
// '__main__' is a conventional name of the package with top level user-supplied
// code. When printing module paths in stack traces and error message,
// "@__main__//<path>" are shown as simply "//<path>" to avoid confusing users
// who don't know what "@<alias>//" might mean. There's no other special
// handling.
//
// Module '@stdlib//builtins.star' (if present) is loaded before any other code.
// All its exported symbols that do not start with '_' are made available in the
// global namespace of all other modules (except ones loaded by
// '@stdlib//builtins.star' itself). This allows to implement in Starlark
// built-in global functions exposed by the interpreter.
//
//
// Built-in symbols
//
// Embedders of the interpreter can also supply arbitrary "predeclared" symbols
// they want to be available in the global scope of all loaded modules.
package interpreter

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/common/errors"
)

var (
	// ErrNoModule is returned by loaders when they can't find a source code of
	// the requested module. It is also returned by LoadModule when it can't find
	// a module within an existing package.
	ErrNoModule = errors.New("no such module")

	// ErrNoPackage is returned by LoadModule when it can't find a package.
	ErrNoPackage = errors.New("no such package")
)

const (
	// MainPkg is an alias of the package with user-supplied code.
	MainPkg = "__main__"
	// StdlibPkg is an alias of the package with the standard library.
	StdlibPkg = "stdlib"
)

// Key of context.Context inside starlark.Thread's local store.
const threadCtxKey = "interpreter.Context"

// Context returns a context of the thread created through Interpreter.
//
// Panics if the Starlark thread wasn't created through Interpreter.Thread().
func Context(th *starlark.Thread) context.Context {
	ctx := th.Local(threadCtxKey)
	if ctx == nil {
		panic("not an Interpreter thread, no context in it")
	}
	return ctx.(context.Context)
}

// Loader knows how to load modules of some concrete package.
//
// It takes a module path relative to the package and returns either modules's
// dict (e.g. for go native modules) or module's source code, to be interpreted.
//
// Returns ErrNoModule if there's no such module in the package.
//
// The source code is returned as 'string' to guarantee immutability and
// allowing efficient use of string Go constants.
type Loader func(path string) (dict starlark.StringDict, src string, err error)

// Interpreter knows how to load starlark modules that can load other starlark
// modules.
type Interpreter struct {
	// Predeclared is a dict with predeclared symbols that are available globally.
	//
	// They are available when loading stdlib and when executing user modules. Can
	// be used to extend the interpreter with native Go functions.
	Predeclared starlark.StringDict

	// Packages is a mapping from a package alias to a loader with package code.
	//
	// Users of Interpreter are expected to supply a loader for at least __main__
	// package.
	Packages map[string]Loader

	// Logger is called by Starlark's print(...) function.
	//
	// The callback takes the position in the starlark source code where
	// print(...) was called and the message it received.
	//
	// The default implementation just prints the message to stderr.
	Logger func(file string, line int, message string)

	// ThreadModifier is called whenever interpreter makes a new starlark.Thread.
	//
	// It can inject additional state into thread locals. Useful when hooking up
	// a thread to starlarktest's reporter in unit tests.
	ThreadModifier func(th *starlark.Thread)

	modules map[moduleKey]*loadedModule // cache of the loaded modules
	globals starlark.StringDict         // global symbols exposed to all modules
}

// moduleKey is a key of a module within a cache of loaded modules.
type moduleKey struct {
	pkg  string // package name, e.g. "stdlib"
	path string // path within the package, e.g. "abc/script.star"
}

// makeModuleKey takes '[@pkg]//path', parses and cleans it up a bit.
//
// Does some light validation. Module loaders are expected to validate module
// paths more rigorously, since they interpret them anyway.
func makeModuleKey(ref string) (key moduleKey, err error) {
	hasPkg := strings.HasPrefix(ref, "@")

	idx := strings.Index(ref, "//")
	if idx == -1 || (!hasPkg && idx != 0) {
		err = errors.New("a module path should be either '//<path>' or '@<package>//<path>'")
		return
	}

	if hasPkg {
		if key.pkg = ref[1:idx]; key.pkg == "" {
			err = errors.New("a package alias can't be empty")
			return
		}
	}
	key.path = path.Clean(ref[idx+2:])

	return
}

// loadedModule represents an executed starlark module.
type loadedModule struct {
	dict starlark.StringDict // global dict of the module after we executed it
	err  error               // non-nil if this module could not be loaded
}

// Init initializes the interpreter and loads '@stdlib//builtins.star'.
//
// Registers most basic built-in symbols first (like 'struct' and 'fail'), then
// whatever was passed via Predeclared. Then loads '@stdlib//builtins.star',
// which may define more symbols or override already defined ones. Whatever
// symbols not starting with '_' end up in the global dict of
// '@stdlib//builtins.star' module will become available as global symbols in
// all modules.
//
// The context ends up available to builtins through Context(...).
func (intr *Interpreter) Init(ctx context.Context) error {
	intr.modules = map[moduleKey]*loadedModule{}

	intr.globals = make(starlark.StringDict, len(intr.Predeclared))
	for k, v := range intr.Predeclared {
		intr.globals[k] = v
	}

	// Load the stdlib, if any.
	top, err := intr.LoadModule(ctx, StdlibPkg, "builtins.star")
	if err != nil && err != ErrNoModule && err != ErrNoPackage {
		return err
	}
	for k, v := range top {
		if !strings.HasPrefix(k, "_") {
			intr.globals[k] = v
		}
	}
	return nil
}

// LoadModule finds and executes a starlark module, returning its dict.
//
// 'pkg' is a package alias, it will be used to lookup the package loader in
// intr.Packages. 'path' is a module path (without leading '//') within
// the package.
//
// The context ends up available to builtins through Context(...).
//
// Caches the result of the execution. Modules are always loaded only once.
func (intr *Interpreter) LoadModule(ctx context.Context, pkg, path string) (starlark.StringDict, error) {
	key, err := makeModuleKey(fmt.Sprintf("@%s//%s", pkg, path))
	if err != nil {
		return nil, err
	}

	switch m, ok := intr.modules[key]; {
	case m != nil: // already loaded or attempted and failed
		return m.dict, m.err
	case ok:
		// This module is being loaded right now, Starlark stack trace will show
		// the sequence of load(...) calls that led to this cycle.
		return nil, errors.New("cycle in the module dependency graph")
	}

	// Add a placeholder to indicate we are loading this module to detect cycles.
	intr.modules[key] = nil

	m := &loadedModule{
		err: fmt.Errorf("panic when loading %q", key), // overwritten on non-panic
	}
	defer func() { intr.modules[key] = m }()

	m.dict, m.err = intr.execModule(ctx, pkg, path)
	return m.dict, m.err
}

// Thread creates a new Starlark thread associated with the given context.
//
// Thread() can be used, for example, to invoke callbacks registered by the
// loaded Starlark code.
//
// The context ends up available to builtins through Context(...).
//
// The returned thread has no implementation of load(...). Use LoadModule to
// load top-level Starlark code instead. Note that load(...) statements are
// forbidden inside Starlark functions anyway.
func (intr *Interpreter) Thread(ctx context.Context) *starlark.Thread {
	th := &starlark.Thread{
		Print: func(th *starlark.Thread, msg string) {
			position := th.Caller().Position()
			if intr.Logger != nil {
				intr.Logger(position.Filename(), int(position.Line), msg)
			} else {
				fmt.Fprintf(os.Stderr, "[%s:%d] %s\n", position.Filename(), position.Line, msg)
			}
		},
	}
	th.SetLocal(threadCtxKey, ctx)
	if intr.ThreadModifier != nil {
		intr.ThreadModifier(th)
	}
	return th
}

// execModule really loads and execute the module.
func (intr *Interpreter) execModule(ctx context.Context, pkg, path string) (starlark.StringDict, error) {
	loader, ok := intr.Packages[pkg]
	if !ok {
		return nil, ErrNoPackage
	}
	dict, src, err := loader(path)
	if err != nil {
		return nil, err
	}

	// This is a native module constructed in Go? No need to interpret it then.
	if dict != nil {
		return dict, nil
	}

	// Otherwise make a thread for executing the code. We do not reuse threads
	// between modules. All global state is passed through intr.globals,
	// intr.modules and ctx.
	th := intr.Thread(ctx)
	th.Load = func(th *starlark.Thread, loadPath string) (starlark.StringDict, error) {
		key, err := makeModuleKey(loadPath)
		if err != nil {
			return nil, err
		}
		// By default (when not specifying @<pkg>) modules within a package refer
		// to sibling files in the same package.
		if key.pkg == "" {
			key.pkg = pkg
		}
		return intr.LoadModule(ctx, key.pkg, key.path)
	}

	// Construct a full module name for error messages and stack traces. Omit the
	// name of the top-level package with user-supplied code ("__main__") to avoid
	// confusing users who are oblivious of packages.
	module := ""
	if pkg != MainPkg {
		module = "@" + pkg
	}
	module += "//" + path

	// Execute the module. It may load other modules inside, which will call
	// th.Load callback above.
	return starlark.ExecFile(th, module, src, intr.globals)
}
