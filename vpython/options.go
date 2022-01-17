// Copyright 2017 The LUCI Authors.
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

package vpython

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"go.chromium.org/luci/vpython/api/vpython"
	"go.chromium.org/luci/vpython/python"
	"go.chromium.org/luci/vpython/spec"
	"go.chromium.org/luci/vpython/venv"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/filesystem"
)

// IsUserError is tagged into errors caused by bad user inputs (e.g. modules or
// scripts which don't exist).
var IsUserError = errors.BoolTag{
	Key: errors.NewTagKey("this error occurred due to a user input."),
}

// Options is the set of options to use to construct and execute a VirtualEnv
// Python application.
type Options struct {
	// The Python command-line to execute. Must not be nil.
	CommandLine *python.CommandLine

	// EnvConfig is the VirtualEnv configuration to run from.
	EnvConfig venv.Config

	// DefaultSpec is the default specification to use, if no specification was
	// supplied or probed.
	DefaultSpec vpython.Spec

	// BaseWheels is the set of wheels to include in the spec. These will always
	// be merged into the runtime spec and normalized, such that any duplicate
	// wheels will be deduplicated.
	BaseWheels []*vpython.Spec_Package

	// SpecLoader is the spec.Loader to use to load a specification file for a
	// given script.
	//
	// The empty value is a valid default spec.Loader.
	SpecLoader spec.Loader

	// WaitForEnv, if true, means that if another agent holds a lock on the target
	// environment, we will wait until it is available. If false, we will
	// immediately exit Setup with an error.
	WaitForEnv bool

	// WorkDir is the Python working directory. If empty, the current working
	// directory will be used.
	//
	// If EnvRoot is empty, WorkDir will be used as the base environment root.
	WorkDir string

	// Environ is environment to pass to subprocesses.
	Environ environ.Env

	// ClearPythonPath, if true, instructs vpython to clear the PYTHONPATH
	// environment variable prior to launch.
	//
	// TODO(iannucci): Delete this once we're satisfied that PYTHONPATH exports
	// are under control.
	ClearPythonPath bool

	// VpythonOptIn, if true, means that users must explicitly chose to enter/stay
	// in the vpython environment when invoking subprocesses. For example, they
	// would need to use sys.executable or 'vpython' for the subprocess.
	//
	// Practically, when this is true, the virtualenv's bin directory will NOT be
	// added to $PATH for the subprocess.
	VpythonOptIn bool
}

func (o *Options) resolve(c context.Context) error {
	// Resolve our working directory to an absolute path.
	if o.WorkDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return errors.Annotate(err, "failed to get working directory").Err()
		}
		o.WorkDir = wd
	}
	if err := filesystem.AbsPath(&o.WorkDir); err != nil {
		return errors.Annotate(err, "failed to resolve absolute path of WorkDir").Err()
	}

	// Resolve our target python script.
	if err := o.ResolveSpec(c); err != nil {
		return errors.Annotate(err, "failed to resolve Python script").Err()
	}
	if len(o.BaseWheels) > 0 {
		o.EnvConfig.Spec = o.EnvConfig.Spec.Clone()
		o.EnvConfig.Spec.Wheel = append(o.EnvConfig.Spec.Wheel, o.BaseWheels...)
	}

	return nil
}

// ResolveSpec resolves the configured environment specification. The resulting
// spec is installed into o's EnvConfig.Spec field.
func (o *Options) ResolveSpec(c context.Context) (err error) {
	if o.CommandLine == nil {
		panic("a CommandLine must be specified")
	}

	// If a spec is explicitly provided, we're done.
	if o.EnvConfig.Spec != nil {
		return nil
	}

	o.EnvConfig.Spec = &o.DefaultSpec

	target := o.CommandLine.Target

	// If there's no target, then we're dropping to an interactive shell
	_, interactive := target.(python.NoTarget)

	// Reading script from stdin is the same as no script in that we don't
	// have a source file to key off of to find the spec, so resolve from CWD
	script, isScriptTarget := target.(python.ScriptTarget)
	loadFromStdin := isScriptTarget && (script.Path == "-")

	// If we're loading a module, then we could attempt to find the module and
	// start the search there. But resolving the module path in full generality
	// would be slow and/or complicated. Perhaps we'll revisit in the future,
	// but for now let's just start the search in the CWD, as this is at least
	// a subset of the paths we should search.
	_, isModuleTarget := target.(python.ModuleTarget)

	// We're either dropping to interactive mode, reading a script from stdin,
	// or loading a module. Regardless, try to resolve the spec from the CWD.
	if interactive || loadFromStdin || isModuleTarget {
		spec, path, err := o.SpecLoader.LoadForScript(c, o.WorkDir, false)
		if err != nil {
			return errors.Annotate(err, "failed to load spec for script: %s", target).Err()
		}
		if spec != nil {
			relpath, err := filepath.Rel(o.WorkDir, path)
			if err != nil {
				return errors.Annotate(err, "failed to get relative path for %s", path).Err()
			}

			if interactive {
				fmt.Fprintf(os.Stderr, "Starting interactive mode, loading vpython spec from %s\n", relpath)
			}

			if loadFromStdin {
				fmt.Fprintf(os.Stderr, "Reading from stdin, loading vpython spec from %s\n", relpath)
			}

			o.EnvConfig.Spec = spec
			return nil
		}
	}

	// If we're running a Python script, assert that the target script exists.
	// Additionally, track whether it's a file or a module (directory).
	isModule := false
	if isScriptTarget {
		logging.Debugf(c, "Resolved Python target script: %s", target)

		// Resolve to absolute script path.
		if err := filesystem.AbsPath(&script.Path); err != nil {
			return errors.Annotate(err, "failed to get absolute path of: %s", target).Err()
		}

		// Confirm that the script path actually exists.
		st, err := os.Stat(script.Path)
		if err != nil {
			return IsUserError.Apply(err)
		}

		// If the script is a directory, then we assume that we're doing a module
		// invocation (__main__.py).
		isModule = st.IsDir()
	}

	// If it's a script, try resolving from filesystem first.
	if isScriptTarget {
		// Use evaluatedScriptPath only for searching the spec. We should use the
		// spec located near the real path of the script instead of the one near
		// the symbol link.
		// Skip EvalSymlinks for windows because it is broken:
		// https://github.com/golang/go/issues/40180
		evaluatedScriptPath := script.Path
		if runtime.GOOS != "windows" {
			if evaluatedScriptPath, err = filepath.EvalSymlinks(script.Path); err != nil {
				return errors.Annotate(err, "failed to get real path for script: %s", script.Path).Err()
			}
		}
		spec, _, err := o.SpecLoader.LoadForScript(c, evaluatedScriptPath, isModule)
		if err != nil {
			return errors.Annotate(err, "failed to load spec for script: %s", target).
				InternalReason("isModule(%v)", isModule).Err()
		}
		if spec != nil {
			o.EnvConfig.Spec = spec
			return nil
		}
	}

	// If standard resolution doesn't yield a spec, fall back on our default spec.
	logging.Infof(c, "Unable to resolve specification path. Using default specification.")
	return nil
}
