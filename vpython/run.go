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

type runCommand struct {
	args    []string
	env     environ.Env
	workDir string
}

// Run sets up a Python VirtualEnv and executes the supplied Options.
//
// If the Python interpreter was successfully launched, Run will never return,
// and the process will exit with the return code of the Python interpreter.
//
// If the Python environment could not be set-up, or if the interpreter could
// not be invoked, Run will return an non-nil error.
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
		// Prepend BinDir to $PATH
		e.Set("PATH", strings.Join(
			[]string{ve.BinDir, e.GetEmpty("PATH")}, string(os.PathListSeparator)))

		// Run our bootstrapped Python command.
		logging.Debugf(c, "Python environment:\nWorkDir: %s\nEnv: %s", opts.WorkDir, e)
		if err := systemSpecificLaunch(c, ve, opts.CommandLine, e, opts.WorkDir); err != nil {
			return errors.Annotate(err, "failed to execute bootstrapped Python").Err()
		}
		return nil
	})
	if err == nil {
		panic("must not return nil error")
	}
	return err
}

// Exec runs the specified Python command.
//
// Once the process launches, Context cancellation will not have an impact.
//
// interp is the Python interperer to run.
//
// cl is the populated CommandLine to run.
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
func Exec(c context.Context, interp *python.Interpreter, cl *python.CommandLine, env environ.Env, dir string, setupFn func() error) error {
	cl = cl.Clone()
	cl.SetIsolatedFlags()

	argv := append([]string{interp.Python}, cl.BuildArgs()...)
	logging.Debugf(c, "Exec Python command: %#v", argv)
	return execImpl(c, argv, env, dir, nil)
}
