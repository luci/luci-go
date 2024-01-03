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

	"go.chromium.org/luci/vpython/python"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
)

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
	// Don't use cl.SetIsolatedFlags here, because they include -B and -E, which
	// both turn off commonly-used aspects of the python interpreter. We do set
	// '-s' though, because we don't want vpython to pick up the user's site
	// directory by default (to maintain some semblance of isolation).
	cl = cl.Clone()
	cl.AddSingleFlag("s")

	argv := append([]string{interp.Python}, cl.BuildArgs()...)
	logging.Debugf(c, "Exec Python command: %#v", argv)
	return execImpl(c, argv, env, dir, setupFn)
}
