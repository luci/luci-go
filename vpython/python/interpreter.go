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
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/filesystem"
)

// Interpreter represents a system Python interpreter. It exposes the ability
// to use common functionality of that interpreter.
type Interpreter struct {
	// Python is the path to the system Python interpreter.
	Python string
}

// Normalize normalizes the Interpreter configuration by resolving relative
// paths into absolute paths.
func (i *Interpreter) Normalize() error {
	return filesystem.AbsPath(&i.Python)
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
