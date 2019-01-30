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
// The interpreter itself adds 'exec' symbol.
//
// Exec'ing modules
//
// The built-in 'exec(...)' can be used to execute a module for its side
// effects and get its global dict as a return value. It works similarly to
// 'load', except it doesn't import any symbols into the caller's namespace, and
// it is not idempotent: calling 'exec' on already executed module is an error.
//
// Modules can either be loaded or exec'ed, but not both. Attempting to load
// a module that was previous exec'ed (and vice versa) is an error.
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

const (
	// Key of context.Context inside starlark.Thread's local store.
	threadCtxKey = "interpreter.Context"
	// Key with the current package name inside starlark.Thread's local store.
	threadPkgKey = "interpreter.Package"
)

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
	execed  map[moduleKey]struct{}      // a set of modules that were ever exec'ed
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
//
// If 'th' is not nil, it is used to get the name of the currently executing
// package if '@pkg' part in 'ref' is omitted.
func makeModuleKey(ref string, th *starlark.Thread) (key moduleKey, err error) {
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

	// Grab the package name from thread locals, if given.
	if !hasPkg && th != nil {
		if key.pkg, _ = th.Local(threadPkgKey).(string); key.pkg == "" {
			err = errors.New("no current package name in thread locals")
			return
		}
	}

	return
}

// loadedModule represents an executed starlark module.
type loadedModule struct {
	dict starlark.StringDict // global dict of the module after we executed it
	err  error               // non-nil if this module could not be loaded
}

// Init initializes the interpreter and loads '@stdlib//builtins.star'.
//
// Registers whatever was passed via Predeclared plus 'exec'. Then loads
// '@stdlib//builtins.star', which may define more symbols or override already
// defined ones. Whatever symbols not starting with '_' end up in the global
// dict of '@stdlib//builtins.star' module will become available as global
// symbols in all modules.
//
// The context ends up available to builtins through Context(...).
func (intr *Interpreter) Init(ctx context.Context) error {
	intr.modules = map[moduleKey]*loadedModule{}
	intr.execed = map[moduleKey]struct{}{}

	intr.globals = make(starlark.StringDict, len(intr.Predeclared)+1)
	for k, v := range intr.Predeclared {
		intr.globals[k] = v
	}
	intr.globals["exec"] = intr.execBuiltin()

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

// LoadModule finds and loads a starlark module, returning its dict.
//
// This is similar to load(...) statement: caches the result of the execution.
// Modules are always loaded only once.
//
// 'pkg' is a package alias, it will be used to lookup the package loader in
// intr.Packages. 'path' is a module path (without leading '//') within
// the package.
//
// The context ends up available to builtins through Context(...).
func (intr *Interpreter) LoadModule(ctx context.Context, pkg, path string) (starlark.StringDict, error) {
	key, err := makeModuleKey(fmt.Sprintf("@%s//%s", pkg, path), nil)
	if err != nil {
		return nil, err
	}

	// If the module has been 'exec'-ed previously it is not allowed to be loaded.
	// Modules are either 'library-like' or 'script-like', not both.
	if _, yes := intr.execed[key]; yes {
		return nil, errors.New("the module has been exec'ed before and therefore is not loadable")
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

	m.dict, m.err = intr.runModule(ctx, pkg, path)
	return m.dict, m.err
}

// ExecModule finds and executes a starlark module, returning its dict.
//
// This is similar to exec(...) statement: always executes the module.
//
// 'pkg' is a package alias, it will be used to lookup the package loader in
// intr.Packages. 'path' is a module path (without leading '//') within
// the package.
//
// The context ends up available to builtins through Context(...).
func (intr *Interpreter) ExecModule(ctx context.Context, pkg, path string) (starlark.StringDict, error) {
	key, err := makeModuleKey(fmt.Sprintf("@%s//%s", pkg, path), nil)
	if err != nil {
		return nil, err
	}

	// If the module has been loaded previously it is not allowed to be 'exec'-ed.
	// Modules are either 'library-like' or 'script-like', not both.
	if _, yes := intr.modules[key]; yes {
		return nil, errors.New("the module has been loaded before and therefore is not executable")
	}

	// Reexecing a module is forbidden.
	if _, yes := intr.execed[key]; yes {
		return nil, errors.New("the module has already been executed, 'exec'-ing same code twice is forbidden")
	}
	intr.execed[key] = struct{}{}

	// Actually execute the code.
	return intr.runModule(ctx, pkg, path)
}

// Thread creates a new Starlark thread associated with the given context.
//
// Thread() can be used, for example, to invoke callbacks registered by the
// loaded Starlark code.
//
// The context ends up available to builtins through Context(...).
//
// The returned thread has no implementation of load(...) or exec(...). Use
// LoadModule or ExecModule to load top-level Starlark code instead. Note that
// load(...) statements are forbidden inside Starlark functions anyway.
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

// execBuiltin returns exec(...) builtin symbol.
func (intr *Interpreter) execBuiltin() *starlark.Builtin {
	return starlark.NewBuiltin("exec", func(th *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var module starlark.String
		if err := starlark.UnpackArgs("exec", args, kwargs, "module", &module); err != nil {
			return nil, err
		}

		// Check the thread was produced by LoadModule or ExecModule. Custom threads
		// (e.g. as used for running Starlark callbacks from native code) do not
		// have enough context to perform 'exec' safely.
		if _, yes := th.Local(threadPkgKey).(string); !yes {
			return nil, fmt.Errorf("exec: forbidden in this thread")
		}

		// See also th.Load in runModule.
		key, err := makeModuleKey(module.GoString(), th)
		if err != nil {
			return nil, err
		}
		dict, err := intr.ExecModule(Context(th), key.pkg, key.path)
		if err != nil {
			// Starlark stringifies errors using Error(). For EvalError, Error()
			// string is just a root cause, it does not include the backtrace where
			// the execution failed. Preserve it explicitly by sticking the backtrace
			// into the error message.
			//
			// Note that returning 'err' as is will preserve the backtrace inside
			// the execed module, but we'll lose the backtrace of the 'exec' call
			// itself, since EvalError object will just bubble to the top of the Go
			// call stack unmodified.
			if evalErr, ok := err.(*starlark.EvalError); ok {
				return nil, fmt.Errorf("exec %s failed: %s", module.GoString(), evalErr.Backtrace())
			}
			return nil, fmt.Errorf("cannot exec %s: %s", module.GoString(), err)
		}

		// StringDict -> regular Dict.
		val := starlark.Dict{}
		for k, v := range dict {
			val.SetKey(starlark.String(k), v)
		}
		return &val, nil
	})
}

// runModule really loads and executes the module, used by both LoadModule and
// ExecModule.
func (intr *Interpreter) runModule(ctx context.Context, pkg, path string) (starlark.StringDict, error) {
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
	th.Load = func(th *starlark.Thread, module string) (starlark.StringDict, error) {
		key, err := makeModuleKey(module, th)
		if err != nil {
			return nil, err
		}
		return intr.LoadModule(ctx, key.pkg, key.path)
	}

	// Let builtins (and in particular makeModuleKey) know the package the module
	// belongs too. This is also a marker for exec(...) that it is allowed.
	th.SetLocal(threadPkgKey, pkg)

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
