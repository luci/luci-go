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

// Package interpreter contains basic starlark interpreter with some features
// that make it useful for non-trivial scripts:
//
//  * Scripts can load other script from the file system.
//  * There's one designated "interpreter root" directory that can be used to
//    load scripts given their path relative to this root. For example,
//    load("//some/script.star", ...).
//  * Scripts can load protobuf message descriptors (built into the interpreter
//    binary) and instantiate protobuf messages defined there.
//  * Scripts have access to some built-in starlark modules (aka 'stdlib'),
//    supplied by whoever instantiates Interpreter.
//  * All symbols from "init.star" stdlib module are available globally to all
//    non-stdlib scripts.
//
// Additionally, all script have access to some predefined symbols:
//
//  `proto`, a library with protobuf helpers, see starlarkproto.ProtoLib.
//
//  def struct(**kwargs):
//    """Returns an object resembling namedtuple from Python.
//
//    kwargs will become attributes of the returned object.
//    See also starlarkstruct package.
//
//
//  def fail(msg):
//    """Aborts the script execution with an error message."""
//
//
//  def mutable(value=None):
//    """Returns an object with get and set methods.
//
//    Allows modules to explicitly declare that they wish to keep some
//    mutable (non-frozen) state, which can be modified after the module has
//    loaded (e.g. from an exported function).
//    """
//
//
//  def to_json(value):
//    """Serializes a value to compact JSON.
//
//    Args:
//      value: a starlark value: scalars, lists, tuples, dicts containing only
//        starlark values.
//
//    Returns:
//      A string with its compact JSON serialization.
//    """
package interpreter

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/starlark/starlarkproto"
)

// ErrNoStdlibModule should be returned by the stdlib loader function if the
// requested module doesn't exist.
var ErrNoStdlibModule = errors.New("no such stdlib module")

// Interpreter knows how to load starlark scripts that can load other starlark
// scripts, instantiate protobuf messages (with descriptors compiled into the
// interpreter binary) and have access to some built-in standard library.
type Interpreter struct {
	// Root is a path to a root directory with scripts.
	//
	// Scripts will be able to load other scripts from this directory using
	// "//path/rel/to/root" syntax in load(...).
	//
	// Default is ".".
	Root string

	// Predeclared is a dict with predeclared symbols that are available globally.
	//
	// They are available when loading stdlib and when executing user scripts.
	Predeclared starlark.StringDict

	// Usercode knows how to load source code files from the file system.
	//
	// It takes a path relative to Root and returns its content (or an error).
	// Used by ExecFile and by load("<path>") statements inside starlark scripts.
	//
	// The default implementation justs uses io.ReadFile.
	Usercode func(path string) (string, error)

	// Stdlib is a set of scripts that constitute a builtin standard library.
	//
	// It is defined as function that takes a script path and returns its body
	// or ErrNoStdlibModule error if there's no such stdlib module.
	//
	// The stdlib is preloaded before execution of other scripts. The stdlib
	// loading starts with init.star script that can load other stdlib scripts
	// using load("builtin:<path>", ...).
	//
	// All globals of init.star script that do not start with '_' will become
	// available as globals in all user scripts.
	Stdlib func(path string) (string, error)

	// Logger is called by starlark's print(...) function.
	//
	// The callback takes the position in the starlark source code where
	// print(...) was called and the message it received.
	//
	// The default implementation just prints the message to stderr.
	Logger func(file string, line int, message string)

	modules map[string]*loadedModule // dicts of loaded modules, keyed by path
	globals starlark.StringDict      // predeclared + symbols from init.star
}

// loadedModule represents an executed starlark module.
//
// We do not reload modules all the time but rather cache their dicts, just like
// Python.
type loadedModule struct {
	globals starlark.StringDict
	err     error // non-nil if this module could not be loaded
}

// Init initializes the interpreter and loads the stdlib.
//
// Registers most basic built in symbols first (like 'struct' and 'fail'). Then
// executes 'init.star' from stdlib, which may define more symbols or override
// already defined ones.
// Initializes intr.Usercode and intr.Logger if they are not set.
//
// Whatever symbols end up in the global dict of init.star stdlib script will
// become available as global symbols in all scripts executed via ExecFile (or
// transitively loaded by them).
func (intr *Interpreter) Init() error {
	if intr.Root == "" {
		intr.Root = "."
	}
	var err error
	if intr.Root, err = filepath.Abs(intr.Root); err != nil {
		return errors.Annotate(err, "failed to resolve %q to an absolute path", intr.Root).Err()
	}

	if intr.Usercode == nil {
		intr.Usercode = func(path string) (string, error) {
			blob, err := ioutil.ReadFile(filepath.Join(intr.Root, path))
			return string(blob), err
		}
	}

	if intr.Logger == nil {
		intr.Logger = func(file string, line int, message string) {
			fmt.Fprintf(os.Stderr, "[%s:%d] %s\n", file, line, message)
		}
	}

	// Register most basic builtin symbols. They'll be available from all scripts
	// (stdlib and user scripts).
	intr.globals = starlark.StringDict{
		"fail":    starlark.NewBuiltin("fail", failImpl),
		"mutable": starlark.NewBuiltin("mutable", mutableImpl),
		"struct":  starlark.NewBuiltin("struct", starlarkstruct.Make),
		"to_json": starlark.NewBuiltin("to_json", toJSONImpl),
	}
	for k, v := range starlarkproto.ProtoLib() {
		intr.globals[k] = v
	}

	// Add all predeclared symbols to make them available when loading stdlib.
	for k, v := range intr.Predeclared {
		intr.globals[k] = v
	}

	// Load the stdlib, if any.
	intr.modules = map[string]*loadedModule{}
	top, err := intr.loadStdlibInit()
	if err != nil && err != ErrNoStdlibModule {
		return err
	}
	for k, v := range top {
		if !strings.HasPrefix(k, "_") {
			intr.globals[k] = v
		}
	}

	return nil
}

// ExecFile executes a starlark script file in an environment that has all
// builtin symbols and all stdlib symbols.
//
// Returns the global dict of the executed script.
func (intr *Interpreter) ExecFile(path string) (starlark.StringDict, error) {
	abs, err := intr.normalizePath(path, "")
	if err != nil {
		return nil, err
	}
	return intr.loadFileModule(abs)
}

// normalizePath converts the path of the module (as it appears in the "load"
// statement) to an absolute file system path or cleaned up "builtin:..." path.
//
// The module path can be specified in two ways:
//   1) As relative to the interpreter root: //path/here/script.star
//   2) As relative to the current executing script: ../script.star.
//
// To resolve (2) this function also takes an absolute path to the currently
// executing script. If there's no executing script (e.g. normalizePath is
// called by top-evel ExecFile call), uses current working directory to convert
// 'mod' to an absolute path.
func (intr *Interpreter) normalizePath(mod, cur string) (string, error) {
	// Builtin paths are always relative, so just clean them up.
	if strings.HasPrefix(mod, "builtin:") {
		return "builtin:" + path.Clean(strings.TrimPrefix(mod, "builtin:")), nil
	}

	switch {
	case strings.HasPrefix(mod, "//"):
		// A path relative to the scripts root directory.
		mod = filepath.Join(intr.Root, filepath.FromSlash(strings.TrimLeft(mod, "/")))
	case cur != "":
		// A path relative to the currently executing script.
		mod = filepath.Join(filepath.Dir(cur), filepath.FromSlash(mod))
	default:
		// A path relative to cwd.
		mod = filepath.FromSlash(mod)
	}

	// Make sure we get a nice looking clean path.
	abs, err := filepath.Abs(mod)
	if err != nil {
		return "", errors.Annotate(err, "failed to resolve %q to an absolute path", mod).Err()
	}
	return abs, nil
}

// rootRel converts a path to be relative to the interpreter root.
//
// We give relative paths to starlark.ExecFile so that stack traces look nicer.
func (intr *Interpreter) rootRel(path string) string {
	rel, err := filepath.Rel(intr.Root, path)
	if err != nil {
		panic(fmt.Errorf("failed to resolve %q as relative path to %q", path, intr.Root))
	}
	return rel
}

// thread returns a new starlark.Thread to use for executing a single file.
//
// We do not reuse threads between files. All global state is passed through
// intr.globals and intr.modules.
func (intr *Interpreter) thread() *starlark.Thread {
	return &starlark.Thread{
		Load: intr.loadImpl,
		Print: func(th *starlark.Thread, msg string) {
			position := th.Caller().Position()
			intr.Logger(position.Filename(), int(position.Line), msg)
		},
	}
}

// loadIfMissing loads the module by calling the given callback if the module
// hasn't been loaded before or just returns the existing module dict otherwise.
func (intr *Interpreter) loadIfMissing(key string, loader func() (starlark.StringDict, error)) (starlark.StringDict, error) {
	switch m, ok := intr.modules[key]; {
	case m != nil: // already loaded or attempted to be loaded
		return m.globals, m.err
	case ok: // this module is being loaded right now
		return nil, errors.New("cycle in load graph")
	}

	// Add a placeholder to indicate we are loading this module to detect cycles.
	intr.modules[key] = nil

	m := &loadedModule{
		err: fmt.Errorf("panic when loading %q", key), // overwritten on non-panic
	}
	defer func() { intr.modules[key] = m }()

	m.globals, m.err = loader()
	return m.globals, m.err
}

// loadImpl implements starlark's load(...) builtin.
//
// It understands 3 kinds of modules:
//  1) A module from the file system referenced either via a path relative to
//     the interpreter root ("//a/b/c.star") or relative to the currently
//     executing script ("../a/b/c.star").
//  2) A protobuf file (compiled into the interpreter binary), referenced by
//     the location of the proto file in the protobuf lib registry:
//        "builtin:go.chromium.org/luci/.../file.proto"
//  3) An stdlib module, as supplied by Stdlib callback:
//        "builtin:some/path/to/be/passed/to/the/callback.star"
func (intr *Interpreter) loadImpl(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	// Grab a name of a module that is calling load(...). This would be either a
	// "builtin:..." path or a path relative to the root, since that's what we
	// pass to starlark.ExecFile.
	cur := thread.TopFrame().Position().Filename()

	// Convert it back to an absolute path, as required by normalizePath.
	curIsBuiltin := strings.HasPrefix(cur, "builtin:")
	if !curIsBuiltin {
		cur = filepath.Join(intr.Root, cur)
	}

	// Cleanup and normalize the path to the module being loaded. 'module' here
	// will be either a "builtin:..." path or an absolute file system path.
	module, err := intr.normalizePath(module, cur)
	if err != nil {
		return nil, err
	}

	// Builtin scripts can load only other builtin scripts, not something from the
	// filesystem.
	loadingBuiltin := strings.HasPrefix(module, "builtin:")
	if curIsBuiltin && !loadingBuiltin {
		return nil, errors.Reason(
			"builtin module %q is attempting to load non-builtin %q, this is forbidden", cur, module).Err()
	}

	// Actually load the module if it hasn't been loaded before.
	return intr.loadIfMissing(module, func() (starlark.StringDict, error) {
		if loadingBuiltin {
			return intr.loadBuiltinModule(module)
		}
		return intr.loadFileModule(module)
	})
}

// loadBuiltinModule loads a builtin module, given as "builtin:<path>".
//
// It can be either a proto descriptor (compiled into the binary) or a stdlib
// module (as supplied by Stdlib callback).
//
// Returns ErrNoStdlibModule if there's no such stdlib module.
func (intr *Interpreter) loadBuiltinModule(module string) (starlark.StringDict, error) {
	path := strings.TrimPrefix(module, "builtin:")
	if strings.HasSuffix(path, ".proto") {
		return starlarkproto.LoadProtoModule(path)
	}
	if intr.Stdlib == nil {
		return nil, ErrNoStdlibModule
	}
	src, err := intr.Stdlib(path)
	if err != nil {
		return nil, err
	}
	return starlark.ExecFile(intr.thread(), module, src, intr.globals)
}

// loadStdlibInit loads init.star from the stdlib and returns its global dict.
//
// Returns ErrNoStdlibModule if there's no init.star in the stdlib.
func (intr *Interpreter) loadStdlibInit() (starlark.StringDict, error) {
	const initStar = "builtin:init.star"
	return intr.loadIfMissing(initStar, func() (starlark.StringDict, error) {
		return intr.loadBuiltinModule(initStar)
	})
}

// loadFileModule loads a starlark module from the file system.
//
// 'path' must always be absolute here, per normalizePath output.
func (intr *Interpreter) loadFileModule(path string) (starlark.StringDict, error) {
	path = intr.rootRel(path)
	src, err := intr.Usercode(path)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read %q", path).Err()
	}
	return starlark.ExecFile(intr.thread(), path, src, intr.globals)
}
