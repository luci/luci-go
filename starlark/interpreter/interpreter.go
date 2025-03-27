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
// Packages also have names which can be used to reference files across packages
// (in both 'load' and 'exec'). In particular modules from one package may refer
// to modules from another package by using the following syntax:
//
//	load("@<package name>//<path within the package>", ...)
//
// The mapping between a package name and package's source code (a collection of
// *.star files under a single root) is static and supplied by the Go code that
// uses the interpreter via Packages map.
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
// Consequently, each Starlark thread created by the interpreter is either
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
// to be available in the global scope of all modules. They can either be
// declared directly using Predeclared dict (useful for builtins implemented
// natively by Go code) or via LoadBuiltins call (useful for builtins that are
// themselves implemented in Starlark).
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
	"go.starlark.net/syntax"
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
// It has no relation to the module that holds the latest stack frame. For
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
// It takes the Starlark thread context and a module path relative to the
// package and returns either module's dict (e.g. for go native modules) or
// module's source code, to be interpreted.
//
// Returns ErrNoModule if there's no such module in the package.
//
// The source code is returned as 'string' to guarantee immutability and
// allowing efficient use of string Go constants.
type Loader func(ctx context.Context, path string) (dict starlark.StringDict, src string, err error)

// Interpreter knows how to execute Starlark modules that can load or execute
// other Starlark modules.
//
// Interpreter's public fields should not be mutated once the Interpreter is
// in use (i.e. after the first call to any of its methods).
type Interpreter struct {
	// Predeclared is a dict with predeclared symbols that are available globally.
	//
	// They are available when loading builtins and when executing user modules.
	// Can be used to extend the interpreter with native Go functions.
	Predeclared starlark.StringDict

	// Packages is a mapping from a package name to a loader with package code.
	//
	// All Starlark code will be loaded exclusively through these loaders.
	Packages map[string]Loader

	// MainPackage optionally designates a package with main code.
	//
	// The only effect is that stack traces will reference modules from this
	// package using short "//<path>" form instead of the fully-qualified standard
	// "@<pkg>//<path>" form (to make them more readable).
	//
	// Has no other effect.
	MainPackage string

	// Logger is called by Starlark's print(...) function.
	//
	// The callback takes the position in the Starlark source code where
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
	PreExec func(th *starlark.Thread, key ModuleKey)

	// PostExec is called after finishing running code through some 'exec' or
	// ExecModule.
	//
	// It is always called, even if the 'exec' failed. May restore the state
	// modified by PreExec. Note that PreExec/PostExec calls can nest (if an
	// 'exec'-ed script calls 'exec' itself).
	//
	// 'load' calls do not trigger PreExec/PostExec hooks.
	PostExec func(th *starlark.Thread, key ModuleKey)

	// CheckVisible, if not nil, defines what modules a module can load or exec.
	//
	// It is called before every load(...) or exec(...) statement, receiving the
	// ModuleKey of the module that calls load/exec (aka "loader") and the
	// ModuleKey of the module being loaded/executed (aka "loaded").
	//
	// If it returns an error, the load/exec statement will fail instead of
	// loading/executing the module.
	//
	// Doesn't affect direct calls to LoadModule, LoadBuiltins or ExecModule
	// (but affects loads and execs they may do in Starlark code).
	CheckVisible func(th *starlark.Thread, loader, loaded ModuleKey) error

	// ForbidLoad, if set, disables load(...) builtin.
	//
	// Attempting to call load(...) will result in a failure with the message
	// given by ForbidLoad value.
	//
	// Has no effect on explicit LoadModule calls from Go code.
	ForbidLoad string

	// ForbidExec, if set, disables exec(...) builtin.
	//
	// Attempting to call exec(...) will result in a failure with the message
	// given by ForbidExec value.
	//
	// Has no effect on explicit ExecModule calls from Go code.
	ForbidExec string

	// Options passed to the Starlark interpreter (or DefaultFileOptions if nil).
	Options *syntax.FileOptions

	modules map[ModuleKey]*loadedModule // cache of the loaded modules
	execed  map[ModuleKey]struct{}      // a set of modules that were ever exec'ed
	visited []ModuleKey                 // all modules, in order of visits
	globals starlark.StringDict         // global symbols exposed to all modules
}

// DefaultFileOptions are options used by default.
func DefaultFileOptions() *syntax.FileOptions {
	return &syntax.FileOptions{Set: true}
}

// ModuleKey is a key of a module within a cache of loaded modules.
//
// It identifies a package and a file within the package. Expected to be used
// as a map key (i.e. will never have hidden internal structure).
type ModuleKey struct {
	Package string // a package name, e.g. "pkg"
	Path    string // path within the package, e.g. "abc/script.star"
}

// String returns the fully-qualified module name as "@pkg//path".
func (key ModuleKey) String() string {
	return fmt.Sprintf("@%s//%s", key.Package, key.Path)
}

// Friendly returns a user-friendly module name.
//
// It is like String, but skips the "@pkg" part if this is the main package
// associated with the interpreter that launched the given thread. If 'th' is
// nil or it is not associated with an interpreter, returns the fully-qualified
// module name as "@pkg//path".
func (key ModuleKey) Friendly(th *starlark.Thread) string {
	if th != nil {
		if intr := th.Local(threadIntrKey); intr != nil {
			return intr.(*Interpreter).moduleKeyToString(key)
		}
	}
	return key.String()
}

// MakeModuleKey takes '[@pkg]//<path>' or '<path>', parses and normalizes it.
//
// Converts the path to be relative to the package root. Does some light
// validation, in particular checking the resulting path doesn't start with
// '../'. Module loaders are expected to validate module paths more rigorously
// (since they interpret them anyway).
//
// 'th' is used to get the name of the currently executing package and a path to
// the currently executing module within it. It is the module being load-ed or
// exec-ed by this thread.
//
// The thread is required if 'ref' is not given as a fully-qualified module name
// (i.e. does NOT look like '@pkg//path'). Using relative module paths without
// passing 'th' will result in an error.
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
			err = errors.New("a package name can't be empty")
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

// loadedModule represents an executed Starlark module.
type loadedModule struct {
	dict starlark.StringDict // global dict of the module after we executed it
	err  error               // non-nil if this module could not be loaded
}

// ensureInitialized initializes the interpreter if it wasn't initialized yet.
//
// Registers whatever was passed via Predeclared plus 'exec' as global symbols
// available to all modules loaded through the interpreter.
func (intr *Interpreter) ensureInitialized() {
	if intr.modules != nil {
		return // already initialized
	}
	intr.modules = map[ModuleKey]*loadedModule{}
	intr.execed = map[ModuleKey]struct{}{}
	intr.globals = make(starlark.StringDict, len(intr.Predeclared)+1)
	for k, v := range intr.Predeclared {
		intr.globals[k] = v
	}
	intr.globals["exec"] = intr.execBuiltin()
}

// LoadBuiltins loads the given module and appends all its exported symbols
// (symbols that do not start with "_") to the list of global symbols available
// in the global scope to all subsequently loaded modules.
//
// Overriding existing global symbols is not allowed and will result in
// an error.
//
// The context ends up available to builtins executed as part of this load
// operation through Context(...).
func (intr *Interpreter) LoadBuiltins(ctx context.Context, pkg, path string) error {
	intr.ensureInitialized()
	top, err := intr.LoadModule(ctx, pkg, path)
	if err != nil {
		return err
	}
	for k, v := range top {
		if !strings.HasPrefix(k, "_") {
			if _, exist := intr.globals[k]; exist {
				return fmt.Errorf("builtin %q from %s is overriding an existing builtin",
					k, intr.moduleKeyToString(ModuleKey{Package: pkg, Path: path}))
			}
			intr.globals[k] = v
		}
	}
	return nil
}

// LoadModule finds and loads a Starlark module, returning its dict.
//
// This is similar to load(...) statement, i.e. it caches the result of
// the execution: modules are always loaded only once.
//
// 'pkg' is a package name, it will be used to lookup the package loader in
// intr.Packages. 'path' is a module path (without leading '//') within
// the package.
//
// The context ends up available to builtins through Context(...).
func (intr *Interpreter) LoadModule(ctx context.Context, pkg, path string) (starlark.StringDict, error) {
	intr.ensureInitialized()

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

// ExecModule finds and executes a Starlark module, returning its dict.
//
// This is similar to exec(...) statement: it executes the module, but only if
// it wasn't already loaded or executed before.
//
// 'pkg' is a package name, it will be used to lookup the package loader in
// intr.Packages. 'path' is a module path (without leading '//') within
// the package.
//
// The context ends up available to builtins through Context(...).
func (intr *Interpreter) ExecModule(ctx context.Context, pkg, path string) (starlark.StringDict, error) {
	intr.ensureInitialized()

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
	intr.ensureInitialized()

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
		return "", fmt.Errorf("cannot load %s: %w", intr.moduleKeyToString(target), err)
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
	intr.ensureInitialized()
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

// moduleKeyToString converts ModuleKey to a user-friendly string.
//
// See comment for MainPackage.
func (intr *Interpreter) moduleKeyToString(key ModuleKey) string {
	if intr.MainPackage != "" && key.Package == intr.MainPackage {
		return "//" + key.Path
	}
	return key.String()
}

// execBuiltin returns exec(...) builtin symbol.
func (intr *Interpreter) execBuiltin() *starlark.Builtin {
	return starlark.NewBuiltin("exec", func(th *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var module starlark.String
		if err := starlark.UnpackArgs("exec", args, kwargs, "module", &module); err != nil {
			return nil, err
		}
		mod := module.GoString()

		if intr.ForbidExec != "" {
			return nil, fmt.Errorf("cannot exec %s: %s", mod, intr.ForbidExec)
		}

		// Only threads started via 'exec' (or equivalently ExecModule) can exec
		// other scripts. Modules that are loaded via load(...), or custom callbacks
		// from native code aren't allowed to call exec, since exec's impurity may
		// lead to unexpected results.
		if GetThreadKind(th) != ThreadExecing {
			return nil, fmt.Errorf("exec %s: forbidden in this context, only exec'ed scripts can exec other scripts", mod)
		}

		// See also th.Load in runModule.
		target, err := MakeModuleKey(th, mod)
		if err != nil {
			return nil, err
		}

		if intr.CheckVisible != nil {
			cur := GetThreadModuleKey(th)
			if cur == nil {
				panic("a ThreadExecing thread always has a current module set")
			}
			if err := intr.CheckVisible(th, *cur, target); err != nil {
				return nil, fmt.Errorf("cannot exec %s: %w", mod, err)
			}
		}

		dict, err := intr.ExecModule(Context(th), target.Package, target.Path)
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
			return nil, fmt.Errorf("cannot exec %s: %w", mod, err)
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
		target, err := MakeModuleKey(th, module)
		if err != nil {
			return nil, err
		}
		if intr.ForbidLoad != "" {
			return nil, fmt.Errorf("%s", intr.ForbidLoad)
		}
		if intr.CheckVisible != nil {
			if err := intr.CheckVisible(th, key, target); err != nil {
				return nil, err
			}
		}
		dict, err := intr.LoadModule(ctx, target.Package, target.Path)
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
	// Use user-friendly module name (with omitted main package name) for error
	// messages and stack traces to make them more readable.
	opts := intr.Options
	if opts == nil {
		opts = DefaultFileOptions()
	}
	return starlark.ExecFileOptions(opts, th, intr.moduleKeyToString(key), src, intr.globals)
}
