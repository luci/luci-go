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
	"os"
	"os/exec"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/filesystem"
)

// LookPathResult is the result of LookPathFunc.
type LookPathResult struct {
	// Path, if not zero, is the absolute path to the identified target
	// executable.
	Path string

	// Version, if not zero, is the Python version string. The most common (only?)
	// LookPathFunc implementation (in go.chromium.org/luci/vpython/application/probe.go)
	// calls the candidate Python to determine its version, and then stashes the
	// result here to void redundant lookups if this LookPathResult ends up being
	// selected.
	Version Version
}

// LookPathFunc attempts to find a file identified by "target", similar to
// exec.LookPath.
//
// "target" will be the name of the program being looked up. It will NOT be a
// path, relative or absolute. It also should not include an extension where
// otherwise applicable (e.g., "python" instead of "python.exe"); the lookup
// function is responsible for trying known extensions as part of the lookup.
//
// A nil LookPathFunc will use exec.LookPath.
type LookPathFunc func(c context.Context, target string, pathDir []string) (*LookPathResult, error)

// Find attempts to find a Python interpreter matching the supplied version
// using PATH.
//
// In order to accommodate multiple configurations on operating systems, Find
// will attempt to identify versions that appear on the path
func Find(c context.Context, vers Version, lookPath LookPathFunc, env environ.Env) (*Interpreter, error) {
	// pythonM.m, pythonM, python
	searches := make([]string, 0, 3)
	pv := vers
	pv.Patch = 0
	if pv.Minor > 0 {
		searches = append(searches, pv.PythonBase())
		pv.Minor = 0
	}
	if pv.Major > 0 {
		searches = append(searches, pv.PythonBase())
		pv.Major = 0
	}
	searches = append(searches, pv.PythonBase())

	lookErrs := errors.NewLazyMultiError(len(searches))
	for i, s := range searches {
		interp, err := findInterpreter(c, s, vers, lookPath, env)
		if err == nil {
			// Resolve to absolute path.
			if err := filesystem.AbsPath(&interp.Python); err != nil {
				return nil, errors.Annotate(err, "could not get absolute path for: %q", interp.Python).Err()
			}

			return interp, nil
		}

		logging.WithError(err).Debugf(c, "Could not find Python for: %q.", s)
		lookErrs.Assign(i, err)
	}

	// No Python interpreter could be identified.
	return nil, errors.Annotate(lookErrs.Get(), "no Python found").Err()
}

func findInterpreter(c context.Context, name string, vers Version, lookPath LookPathFunc, env environ.Env) (*Interpreter, error) {
	pathDir := make([]string, 1, 1)
	var pathEntries []string

	if lookPath == nil {
		lookPath = osExecLookPath
		pathEntries = strings.Split("ignored", string(os.PathListSeparator))
	} else {
		pathEntries = strings.Split(env.GetEmpty("PATH"), string(os.PathListSeparator))
	}

	for idx := 0; idx < len(pathEntries); idx++ {
		pathDir[0] = pathEntries[idx]
		lpr, err := lookPath(c, name, pathDir)
		if err != nil {
			//logging.WithError(err).Debugf(c, "No Python found in %q.", pathDir[0])
			continue
		}

		i := Interpreter{
			Python: lpr.Path,
		}

		// If our LookPathResult included a target version, install that into the
		// Interpreter, allowing it to use this cached value when GetVersion is
		// called instead of needing to perform an additional lookup.
		//
		// Note that our LookPathResult may not populate Version, in which case we
		// will not pre-cache it.
		if !lpr.Version.IsZero() {
			i.cachedVersion = &lpr.Version
		}
		if err := i.Normalize(); err != nil {
			logging.WithError(err).Debugf(c, "Could not normalize Python for %q.", i.Python)
			continue
		}

		iv, err := i.GetVersion(c)
		if err != nil {
			logging.WithError(err).Debugf(c, "failed to get version for: %q", i.Python)
			continue
		}
		if vers.IsSatisfiedBy(iv) {
			return &i, nil
		} else {
			logging.Debugf(c, "interpreter %q version %q does not satisfy %q", i.Python, iv, vers)
		}
	}

	return nil, errors.Reason("No interpreter found satisfying %q", vers).Err()
}

func osExecLookPath(c context.Context, target string, pathDir []string) (*LookPathResult, error) {
	v, err := exec.LookPath(target)
	if err != nil {
		return nil, err
	}
	return &LookPathResult{Path: v}, nil
}
