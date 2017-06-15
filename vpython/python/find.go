// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package python

import (
	"os/exec"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"

	"golang.org/x/net/context"
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
			return interp, nil
		}

		logging.WithError(err).Debugf(c, "Could not find Python for: %q.", s)
		lookErrs.Assign(i, err)
	}

	// No Python interpreter could be identified.
	return nil, errors.Annotate(lookErrs.Get()).Reason("no Python found").Err()
}

func findInterpreter(c context.Context, name string, vers Version, lookPath LookPathFunc) (*Interpreter, error) {
	if lookPath == nil {
		lookPath = osExecLookPath
	}
	lpr, err := lookPath(c, name)
	if err != nil {
		return nil, errors.Annotate(err).Reason("could not find executable for: %(name)q").
			D("name", name).
			Err()
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
		return nil, errors.Annotate(err).Reason("failed to get version for: %(interp)q").
			D("interp", i.Python).
			Err()
	}
	if !vers.IsSatisfiedBy(iv) {
		return nil, errors.Reason("interpreter %(interp)q version %(interpVersion)q does not satisfy %(version)q").
			D("interp", i.Python).
			D("interpVersion", iv).
			D("version", vers).
			Err()
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
