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

package python

import (
	"context"
	"os/exec"
	"sync"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/filesystem"
)

// Interpreter represents a system Python interpreter. It exposes the ability
// to use common functionality of that interpreter.
type Interpreter struct {
	// Python is the path to the system Python interpreter.
	Python string

	// cachedVersion is the cached Version for this interpreter. It is populated
	// on the first GetVersion call.
	cachedVersion   *Version
	cachedVersionMu sync.Mutex

	// testCommandHook, if not nil, is called on generated Command results prior
	// to returning them.
	testCommandHook func(*exec.Cmd)
}

// Normalize normalizes the Interpreter configuration by resolving relative
// paths into absolute paths.
func (i *Interpreter) Normalize() error {
	return filesystem.AbsPath(&i.Python)
}

// IsolatedCommand returns a configurable exec.Cmd structure bound to this
// Interpreter.
//
// The supplied arguments have several Python isolation flags prepended to them
// to remove environmental factors such as:
//	- The user's "site.py".
//	- The current PYTHONPATH environment variable.
//	- Compiled ".pyc/.pyo" files.
func (i *Interpreter) IsolatedCommand(c context.Context, target Target, args ...string) *exec.Cmd {
	// Isolate the supplied arguments.
	cl := CommandLine{
		Target: target,
		Args:   args,
	}
	cl.SetIsolatedFlags()
	cmd := exec.CommandContext(c, i.Python, cl.BuildArgs()...)
	if i.testCommandHook != nil {
		i.testCommandHook(cmd)
	}
	return cmd
}

// GetVersion runs the specified Python interpreter to extract its version
// from `platform.python_version` and maps it to a known specification verison.
func (i *Interpreter) GetVersion(c context.Context) (v Version, err error) {
	i.cachedVersionMu.Lock()
	defer i.cachedVersionMu.Unlock()

	// Check again, under write-lock.
	if i.cachedVersion != nil {
		v = *i.cachedVersion
		return
	}

	cmd := i.IsolatedCommand(c, CommandTarget{
		"import platform, sys; sys.stdout.write(platform.python_version())",
	})

	out, err := cmd.Output()
	if err != nil {
		err = errors.Annotate(err, "").Err()
		return
	}

	if v, err = ParseVersion(string(out)); err != nil {
		return
	}
	if v.IsZero() {
		err = errors.Reason("unknown version output").Err()
		return
	}

	i.cachedVersion = &v
	return
}

// IsolateEnvironment mutates e to remove any environmental influence over
// the Python interpreter.
//
// If keepPythonPath is true, PYTHONPATH will not be cleared. This is used
// by the actual VirtualEnv Python invocation to preserve PYTHONPATH since it is
// a form of user input.
//
// If e is nil, no operation will be performed.
func IsolateEnvironment(e *environ.Env, keepPythonPath bool) {
	if e == nil {
		return
	}

	// Remove PYTHONPATH if instructed.
	if !keepPythonPath {
		e.Remove("PYTHONPATH")
	}

	// Remove PYTHONHOME from the environment. PYTHONHOME is used to set the
	// location of standard Python libraries, which we make a point of overriding.
	//
	// https://docs.python.org/2/using/cmdline.html#envvar-PYTHONHOME
	e.Remove("PYTHONHOME")

	// set PYTHONNOUSERSITE, which prevents a user's "site" configuration
	// from influencing Python startup. The system "site" should already be
	// ignored b/c we're using the VirtualEnv Python interpreter.
	e.Set("PYTHONNOUSERSITE", "1")
}
