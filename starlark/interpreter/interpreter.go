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

// Package interpreter contains customized Starlark interpreter.
//
// It is an opinionated wrapper around the basic single-file interpreter
// provided by go.starlark.net library. It implements 'load' and a new built-in
// 'exec' in a specific way to support running Starlark programs that consist of
// many files that reference each other, thus allowing decomposing code into
// small logical chunks and enabling code reuse.
//
// # Modules and packages
//
// Two main new concepts are modules and packages. A package is a collection
// of Starlark files living under the same root. A module is just one such
// Starlark file. Furthermore, modules can either be "library-like" (executed
// via 'load' statement) or "script-like" (executed via 'exec' function).
//
// Library-like modules can load other library-like modules via 'load', but may
// not call 'exec'. Script-like modules may use both 'load' and 'exec'.
//
// Modules within a single package can refer to each other (in 'load' and
// 'exec') using their relative or absolute (if start with "//") paths.
//
// Packages also have identifiers, though they are local to the interpreter
// (e.g. not defined anywhere in the package itself). For that reason they are
// called "package aliases".
//
// Modules from one package may refer to modules from another package by using
// the following syntax:
//
//	load("@<package alias>//<path within the package>", ...)
//
// Presently the mapping between a package alias (e.g. 'stdlib') and package's
// source code (a collection of *.star files under a single root) is static and
// supplied by Go code that uses the interpreter. This may change in the future
// to allow loading packages dynamically, e.g. over the network.
//
// # Special packages
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
// Exec'ing modules
//
// The built-in function 'exec' can be used to execute a module for its side
// effects and get its global dict as a return value. It works similarly to
// 'load', except it doesn't import any symbols into the caller's namespace, and
// it is not idempotent: calling 'exec' on already executed module is an error.
//
// Each module is executed with its own instance of starlark.Thread. Modules
// are either loaded as libraries (via 'load' call or LoadModule) or executed
// as scripts (via 'exec' or ExecModule), but not both. Attempting to load
// a module that was previous exec'ed (and vice versa) is an error.
//
// Consequently, each Starlark thread created by the nterpreter is either
// executing some 'load' or some 'exec'. This distinction is available to
// builtins through GetThreadKind function. Some builtins may check it. For
// example, a builtin that mutates a global state may return an error when it is
// called from a 'load' thread.
//
// Dicts of modules loaded via 'load' are reused, e.g. if two different scripts
// load the exact same module, they'll get the exact same symbols as a result.
// The loaded code always executes only once. The interpreter MAY load modules
// in parallel in the future, libraries must not rely on their loading order and
// must not have side effects.
//
// On the other hand, modules executed via 'exec' are guaranteed to be executed
// sequentially, and only once. Thus 'exec'-ed scripts essentially form a tree,
// traversed exactly once in the depth first order.
//
// # Built-in symbols
//
// In addition to 'exec' builtin implemented by the interpreter itself, users
// of the interpreter can also supply arbitrary "predeclared" symbols they want
// to be available in the global scope of all modules. Predeclared symbols and
// '@stdlib//builtins.star' module explained above, are the primary mechanisms
// of making the interpreter do something useful.
package interpreter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

var (
	// ErrNoModule is returned by loaders when they can't find a source code of
	// the requested module. It is also returned by LoadModule when it can't find
	// a module within an existing package.
	ErrNoModule = errors.New("no such module")

	// ErrNoPackage is returned by LoadModule when it can't find a package.
	ErrNoPackage = errors.New("no such package")
)

// ThreadKind is enumeration describing possible uses of a thread.
type ThreadKind int

const (
	// ThreadUnknown indicates a thread used in some custom way, not via
	// LoadModule or ExecModule.
	ThreadUnknown ThreadKind = iota

	// ThreadLoading indicates a thread that is evaluating a module referred to by
	// some load(...) statement.
	ThreadLoading

	// ThreadExecing indicates a thread that is evaluating a module referred to by
	// some exec(...) statement.
	ThreadExecing
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
	// Key with *Interpreter that created the thread.
	threadIntrKey = "interpreter.Interpreter"
	// Key with TheadKind of the thread.
	threadKindKey = "interpreter.ThreadKind"
	// Key with the ModuleKey of the currently executing module.
	threadModKey = "interpreter.ModuleKey"
)

// Context returns a context of the thread created through Interpreter.
//
// Panics if the Starlark thread wasn't created through Interpreter.Thread().
func Context(th *starlark.Thread) context.Context {
	if ctx := th.Local(threadCtxKey); ctx != nil {
		return ctx.(context.Context)
	}
	panic("not an Interpreter thread, no context in its locals")
}

// GetThreadInterpreter returns Interpreter that created the Starlark thread.
//
// Panics if the Starlark thread wasn't created through Interpreter.Thread().
func GetThreadInterpreter(th *starlark.Thread) *Interpreter {
	if intr := th.Local(threadIntrKey); intr != nil {
		return intr.(*Interpreter)
	}
	panic("not an Interpreter thread, no Interpreter in its locals")
}

// GetThreadKind tells what sort of thread 'th' is: it is either inside some
// load(...), some exec(...) or some custom call (probably a callback called
// from native code).
//
// Panics if the Starlark thread wasn't created through Interpreter.Thread().
func GetThreadKind(th *starlark.Thread) ThreadKind {
	if k := th.Local(threadKindKey); k != nil {
		return k.(ThreadKind)
	}
	panic("not an Interpreter thread, no ThreadKind in its locals")
}

// GetThreadModuleKey returns a ModuleKey with the location of the module being
// processed by a current load(...) or exec(...) statement.
//
// It has no relation to the module that holds the top-level stack frame. For
// example, if a currently loading module 'A' calls a function in module 'B' and
// this function calls GetThreadModuleKey, it will see module 'A' as the result,
// even though the call goes through code in module 'B'.
//
// Returns nil if the current thread is not executing any load(...) or
// exec(...), i.e. it has ThreadUnknown kind.
func GetThreadModuleKey(th *starlark.Thread) *ModuleKey {
	if modKey, ok := th.Local(threadModKey).(ModuleKey); ok {
		return &modKey
	}
	return nil
}

// Loader knows how to load modules of some concrete package.
//
// It takes the starlark thread context and a module path relative to the
// package and returns either module's dict (e.g. for go native modules) or
// module's source code, to be interpreted.
//
// Returns ErrNoModule if there's no such module in the package.
//
// The source code is returned as 'string' to guarantee immutability and
// allowing efficient use of string Go constants.
type Loader func(ctx context.Context, path string) (dict starlark.StringDict, src string, err error)

// Interpreter knows how to execute starlark modules that can load or execute
// other starlark modules.
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

	// PreExec is called before launching code through some 'exec' or ExecModule.
	//
	// It may modify the thread or some other global state in preparation for
	// executing a script.
	//
	// 'load' calls do not trigger PreExec/PostExec hooks.
	PreExec func(th *starlark.Thread, module ModuleKey)

	// PostExec is called after finishing running code through some 'exec' or
	// ExecModule.
	//
	// It is always called, even if the 'exec' failed. May restore the state
	// modified by PreExec. Note that PreExec/PostExec calls can nest (if an
	// 'exec'-ed script calls 'exec' itself).
	//
	// 'load' calls do not trigger PreExec/PostExec hooks.
	PostExec func(th *starlark.Thread, module ModuleKey)

	modules map[ModuleKey]*loadedModule // cache of the loaded modules
	execed  map[ModuleKey]struct{}      // a set of modules that were ever exec'ed
	visited []ModuleKey                 // all modules, in order of visits
	globals starlark.StringDict         // global symbols exposed to all modules
}

// ModuleKey is a key of a module within a cache of loaded modules.
//
// It identifies a package and a file within the package.
type ModuleKey struct {
	Package string // a package name, e.g. "stdlib"
	Path    string // path within the package, e.g. "abc/script.star"
}

// String returns a fully-qualified module name to use in error messages.
//
// If is either "@pkg//path" or just "//path" if pkg is "__main__". We omit the
// name of the top-level package with user-supplied code ("__main__") to avoid
// confusing users who are oblivious of packages.
func (key ModuleKey) String() string {
	pkg := ""
	if key.Package != MainPkg {
		pkg = "@" + key.Package
	}
	return pkg + "//" + key.Path
}

// MakeModuleKey takes '[@pkg]//<path>' or '<path>', parses and normalizes it.
//
// Converts the path to be relative to the package root. Does some light
// validation, in particular checking the resulting path doesn't start with
// '../'. Module loaders are expected to validate module paths more rigorously
// (since they interpret them anyway).
//
// 'th' is used to get the name of the currently executing package and a path to
// the currently executing module within it. It is required if 'ref' is not
// given as an absolute path (i.e. does NOT look like '@pkg//path').
func MakeModuleKey(th *starlark.Thread, ref string) (key ModuleKey, err error) {
	defer func() {
		if err == nil && (strings.HasPrefix(key.Path, "../") || key.Path == "..") {
			err = errors.New("outside the package root")
		}
	}()

	// 'th' can be nil here if MakeModuleKey is called by LoadModule or
	// ExecModule: they are entry points into Starlark code, there's no thread
	// yet when they start.
	var current *ModuleKey
	if th != nil {
		current = GetThreadModuleKey(th)
	}

	// Absolute paths start with '//' or '@'. Everything else is a relative path.
	hasPkg := strings.HasPrefix(ref, "@")
	isAbs := hasPkg || strings.HasPrefix(ref, "//")

	if !isAbs {
		if current == nil {
			err = errors.New("can't resolve relative module path: no current module information in thread locals")
		} else {
			key = ModuleKey{
				Package: current.Package,
				Path:    path.Join(path.Dir(current.Path), ref),
			}
		}
		return
	}

	idx := strings.Index(ref, "//")
	if idx == -1 || (!hasPkg && idx != 0) {
		err = errors.New("a module path should be either '//<path>', '<path>' or '@<package>//<path>'")
		return
	}

	if hasPkg {
		if key.Package = ref[1:idx]; key.Package == "" {
			err = errors.New("a package alias can't be empty")
			return
		}
	}
	key.Path = path.Clean(ref[idx+2:])

	// Grab the package name from thread locals, if given.
	if !hasPkg {
		if current == nil {
			err = errors.New("no current package name in thread locals")
		} else {
			key.Package = current.Package
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
	intr.modules = map[ModuleKey]*loadedModule{}
	intr.execed = map[ModuleKey]struct{}{}

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
	key, err := MakeModuleKey(nil, fmt.Sprintf("@%s//%s", pkg, path))
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

	m.dict, m.err = intr.runModule(ctx, key, ThreadLoading)
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
	key, err := MakeModuleKey(nil, fmt.Sprintf("@%s//%s", pkg, path))
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
	return intr.runModule(ctx, key, ThreadExecing)
}

// Visited returns a list of modules visited by the interpreter.
//
// Includes both loaded and execed modules, successfully or not.
func (intr *Interpreter) Visited() []ModuleKey {
	return intr.visited
}

// LoadSource returns a body of a file inside a package.
//
// It doesn't have to be a Starlark file, can be any text file as long as
// the corresponding package Loader can find it.
//
// 'ref' is either an absolute reference to the file, in the same format as
// accepted by 'load' and 'exec' (i.e. "[@pkg]//path"), or a path relative to
// the currently executing module (i.e. just "path").
//
// Only Starlark threads started via LoadModule or ExecModule can be used with
// this function. Other threads don't have enough context to resolve paths
// correctly.
func (intr *Interpreter) LoadSource(th *starlark.Thread, ref string) (string, error) {
	if kind := GetThreadKind(th); kind != ThreadLoading && kind != ThreadExecing {
		return "", errors.New("wrong kind of thread (not enough information to " +
			"resolve the file reference), only threads that do 'load' and 'exec' can call this function")
	}

	target, err := MakeModuleKey(th, ref)
	if err != nil {
		return "", err
	}

	dict, src, err := intr.invokeLoader(Context(th), target)
	if err == nil && dict != nil {
		err = fmt.Errorf("it is a native Go module")
	}
	if err != nil {
		// Avoid term "module" since LoadSource is often used to load non-star
		// files.
		if err == ErrNoModule {
			err = fmt.Errorf("no such file")
		}
		return "", fmt.Errorf("cannot load %s: %s", target, err)
	}
	return src, nil
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
			position := th.CallFrame(1).Pos
			if intr.Logger != nil {
				intr.Logger(position.Filename(), int(position.Line), msg)
			} else {
				fmt.Fprintf(os.Stderr, "[%s:%d] %s\n", position.Filename(), position.Line, msg)
			}
		},
	}
	th.SetLocal(threadCtxKey, ctx)
	th.SetLocal(threadIntrKey, intr)
	th.SetLocal(threadKindKey, ThreadUnknown)
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
		mod := module.GoString()

		// Only threads started via 'exec' (or equivalently ExecModule) can exec
		// other scripts. Modules that are loaded via load(...), or custom callbacks
		// from native code aren't allowed to call exec, since exec's impurity may
		// lead to unexpected results.
		if GetThreadKind(th) != ThreadExecing {
			return nil, fmt.Errorf("exec %s: forbidden in this context, only exec'ed scripts can exec other scripts", mod)
		}

		// See also th.Load in runModule.
		key, err := MakeModuleKey(th, mod)
		if err != nil {
			return nil, err
		}
		dict, err := intr.ExecModule(Context(th), key.Package, key.Path)
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
				return nil, fmt.Errorf("exec %s failed: %s", mod, evalErr.Backtrace())
			}
			return nil, fmt.Errorf("cannot exec %s: %s", mod, err)
		}

		return starlarkstruct.FromStringDict(starlark.String("execed"), dict), nil
	})
}

// invokeLoader loads the module via the loader associated with the package.
func (intr *Interpreter) invokeLoader(ctx context.Context, key ModuleKey) (dict starlark.StringDict, src string, err error) {
	loader, ok := intr.Packages[key.Package]
	if !ok {
		return nil, "", ErrNoPackage
	}
	return loader(ctx, key.Path)
}

// runModule really loads and executes the module, used by both LoadModule and
// ExecModule.
func (intr *Interpreter) runModule(ctx context.Context, key ModuleKey, kind ThreadKind) (starlark.StringDict, error) {
	// Grab the source code.
	dict, src, err := intr.invokeLoader(ctx, key)
	switch {
	case err != nil:
		return nil, err
	case dict != nil:
		// This is a native module constructed in Go, no need to interpret it.
		return dict, nil
	}

	// Otherwise make a thread for executing the code. We do not reuse threads
	// between modules. All global state is passed through intr.globals,
	// intr.modules and ctx.
	th := intr.Thread(ctx)
	th.Load = func(th *starlark.Thread, module string) (starlark.StringDict, error) {
		key, err := MakeModuleKey(th, module)
		if err != nil {
			return nil, err
		}
		dict, err := intr.LoadModule(ctx, key.Package, key.Path)
		// See comment in execBuiltin about why we extract EvalError backtrace into
		// new error.
		if evalErr, ok := err.(*starlark.EvalError); ok {
			err = fmt.Errorf("%s", evalErr.Backtrace())
		}
		return dict, err
	}

	// Let builtins know what this thread is doing. Some calls (most notably Exec
	// itself) are allowed only from exec'ing threads, not from load'ing ones.
	th.SetLocal(threadKindKey, kind)
	// Let builtins (and in particular MakeModuleKey and LoadSource) know the
	// package and the module that the thread executes.
	th.SetLocal(threadModKey, key)

	if kind == ThreadExecing {
		if intr.PreExec != nil {
			intr.PreExec(th, key)
		}
		if intr.PostExec != nil {
			defer intr.PostExec(th, key)
		}
	}

	// Record we've been here.
	intr.visited = append(intr.visited, key)

	// Execute the module. It may 'load' or 'exec' other modules inside, which
	// will either call LoadModule or ExecModule.
	//
	// Use user-friendly module name (with omitted "@__main__") for error messages
	// and stack traces to avoid confusing the user.
	return starlark.ExecFile(th, key.String(), src, intr.globals)
}
