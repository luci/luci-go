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
	"strings"

	"go.chromium.org/luci/vpython/python"
	"go.chromium.org/luci/vpython/venv"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"

	"golang.org/x/net/context"
)

const (
	// EnvironmentStampPathENV is the exported environment variable for the
	// environment stamp path.
	//
	// This is added to the bootstrap environment used by Run to allow subprocess
	// "vpython" invocations to automatically inherit the same environment.
	EnvironmentStampPathENV = "VPYTHON_VENV_ENV_STAMP_PATH"
)

type runCommand struct {
	args    []string
	env     environ.Env
	workDir string
}

// Run sets up a Python VirtualEnv and executes the supplied Options.
//
// Run returns nil if if the Python environment was successfully set-up and the
// Python interpreter was successfully run with a zero return code. If the
// Python interpreter returns a non-zero return code, a PythonError (potentially
// wrapped) will be returned.
//
// A generalized return code to return for an error value can be obtained via
// ReturnCode.
//
// Run consists of:
//
//	- Identify the target Python script to run (if there is one).
//	- Identifying the Python interpreter to use.
//	- Composing the environment specification.
//	- Constructing the virtual environment (download, install).
//	- Execute the Python process with the supplied arguments.
//
// The Python subprocess is bound to the lifetime of ctx, and will be terminated
// if ctx is cancelled.
func Run(c context.Context, opts Options) error {
	// Resolve our Options.
	if err := opts.resolve(c); err != nil {
		return errors.Annotate(err, "could not resolve options").Err()
	}

	// Create our virtual environment root directory.
	opts.EnvConfig.FailIfLocked = !opts.WaitForEnv
	err := venv.With(c, opts.EnvConfig, func(c context.Context, ve *venv.Env) error {
		e := opts.Environ.Clone()
		python.IsolateEnvironment(&e)

		e.Set("VIRTUAL_ENV", ve.Root) // Set by VirtualEnv script.
		if ve.EnvironmentStampPath != "" {
			e.Set(EnvironmentStampPathENV, ve.EnvironmentStampPath)
		}

		// Prefix PATH with the VirtualEnv "bin" directory.
		prefixPATH(e, ve.BinDir)

		// Run our bootstrapped Python command.
		logging.Debugf(c, "Python environment:\nWorkDir: %s\nEnv: %s", opts.WorkDir, e)
		if err := systemSpecificLaunch(c, ve, opts.Args, e, opts.WorkDir); err != nil {
			return errors.Annotate(err, "failed to execute bootstrapped Python").Err()
		}
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "").Err()
	}
	return nil
}

// Exec runs the specified Python command.
//
// Once the process launches, Context cancellation will not have an impact.
//
// interp is the Python interperer to run.
//
// args is the set of arguments to pass to the interpreter.
//
// env is the environment to install.
//
// dir, if not empty, is the working directory of the command.
//
// setupFn, if not nil, is a function that will be run immediately before
// execution, after all operations that are permitted to fail have completed.
// Any error returned here will result in a panic.
//
// If an error occurs during execution, it will be returned here. Otherwise,
// Exec will not return, and this process will exit with the return code of the
// executed process.
//
// The implementation of Exec is platform-specific.
func Exec(c context.Context, interp *python.Interpreter, args []string, env environ.Env, dir string, setupFn func() error) error {
	argv := interp.IsolatedCommandParams(args...)
	logging.Debugf(c, "Exec Python command: %v", argv)
	return execImpl(c, argv, env, dir, nil)
}

func prefixPATH(env environ.Env, components ...string) {
	if len(components) == 0 {
		return
	}

	// Clone "components" so we don't mutate our caller's array.
	components = append([]string(nil), components...)

	// If there is a current PATH (likely), add that to the end.
	cur, _ := env.Get("PATH")
	if len(cur) > 0 {
		components = append(components, cur)
	}

	env.Set("PATH", strings.Join(components, string(os.PathListSeparator)))
}
