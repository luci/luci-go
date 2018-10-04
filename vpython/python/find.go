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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/filesystem"
)

// LookPathResult is the result of LookPathFunc.
type LookPathResult struct {
	// Path, if not zero, is the absolute path to the identified target
	// executable.
	Path string

	// Version, if not zero, is the Python version string. Standard "vpython"
	// wrapper identification may choose to call Python with "--version" to
	// identify it. If so, it may choose to populate this field to avoid redundant
	// calls to "--version".
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
type LookPathFunc func(c context.Context, target string) (*LookPathResult, error)

// Find attempts to find a Python interpreter matching the supplied version
// using PATH.
//
// In order to accommodate multiple configurations on operating systems, Find
// will attempt to identify versions that appear on the path
func Find(c context.Context, vers Version, lookPath LookPathFunc) (*Interpreter, error) {
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
		interp, err := findInterpreter(c, s, vers, lookPath)
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

func findInterpreter(c context.Context, name string, vers Version, lookPath LookPathFunc) (*Interpreter, error) {
	if lookPath == nil {
		lookPath = osExecLookPath
	}
	lpr, err := lookPath(c, name)
	if err != nil {
		return nil, errors.Annotate(err, "could not find executable for: %q", name).Err()
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
		return nil, err
	}

	iv, err := i.GetVersion(c)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get version for: %q", i.Python).Err()
	}
	if !vers.IsSatisfiedBy(iv) {
		return nil, errors.Reason("interpreter %q version %q does not satisfy %q", i.Python, iv, vers).Err()
	}

	return &i, nil
}

func osExecLookPath(c context.Context, target string) (*LookPathResult, error) {
	v, err := exec.LookPath(target)
	if err != nil {
		return nil, err
	}
	return &LookPathResult{Path: v}, nil
}
