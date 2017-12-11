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
	"os"

	"golang.org/x/net/context"

	"go.chromium.org/luci/vpython/api/vpython"
	"go.chromium.org/luci/vpython/python"
	"go.chromium.org/luci/vpython/spec"
	"go.chromium.org/luci/vpython/venv"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/filesystem"
)

// Options is the set of options to use to construct and execute a VirtualEnv
// Python application.
type Options struct {
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

	// Args are the arguments to forward to the Python process.
	Args []string

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
func (o *Options) ResolveSpec(c context.Context) error {
	// If a spec is explicitly provided, we're done.
	if o.EnvConfig.Spec != nil {
		return nil
	}

	// Determine our specification from the Python command-line that we are being
	// invoked with.
	cmd, err := python.ParseCommandLine(o.Args)
	if err != nil {
		return errors.Annotate(err, "failed to parse Python command-line").Err()
	}

	// If we're running a Python script, assert that the target script exists.
	// Additionally, track whether it's a file or a module (directory).
	script, isScriptTarget := cmd.Target.(python.ScriptTarget)
	isModule := false
	if isScriptTarget {
		logging.Debugf(c, "Resolved Python target script: %s", cmd.Target)

		// Resolve to absolute script path.
		if err := filesystem.AbsPath(&script.Path); err != nil {
			return errors.Annotate(err, "failed to get absolute path of: %s", cmd.Target).Err()
		}

		// Confirm that the script path actually exists.
		st, err := os.Stat(script.Path)
		if err != nil {
			return errors.Annotate(err, "failed to stat Python script: %s", cmd.Target).Err()
		}

		// If the script is a directory, then we assume that we're doing a module
		// invocation (__main__.py).
		isModule = st.IsDir()
	}

	// If it's a script, try resolving from filesystem first.
	if isScriptTarget {
		spec, err := o.SpecLoader.LoadForScript(c, script.Path, isModule)
		if err != nil {
			return errors.Annotate(err, "failed to load spec for script: %s", cmd.Target).
				InternalReason("isModule(%v)", isModule).Err()
		}
		if spec != nil {
			o.EnvConfig.Spec = spec
			return nil
		}
	}

	// If standard resolution doesn't yield a spec, fall back on our default spec.
	logging.Infof(c, "Unable to resolve specification path. Using default specification.")
	o.EnvConfig.Spec = &o.DefaultSpec
	return nil
}
