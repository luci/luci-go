// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package python

import (
	"os/exec"

	"github.com/luci/luci-go/common/errors"

	"golang.org/x/net/context"
)

// Find attempts to find a Python interpreter matching the supplied version
// using PATH.
//
// In order to accommodate multiple configurations on operating systems, Find
// will attempt to identify versions that appear on the path
func Find(c context.Context, vers Version) (*Interpreter, error) {
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

	for _, s := range searches {
		p, err := exec.LookPath(s)
		if err != nil {
			if e, ok := err.(*exec.Error); ok && e.Err == exec.ErrNotFound {
				// Not found is okay.
				continue
			}
			return nil, errors.Annotate(err).Reason("failed to search PATH for: %(interp)q").
				D("interp", s).
				Err()
		}

		i := Interpreter{Python: p}
		if err := i.Normalize(); err != nil {
			return nil, err
		}

		iv, err := i.GetVersion(c)
		if err != nil {
			return nil, errors.Annotate(err).Reason("failed to get version for: %(interp)q").
				D("interp", i.Python).
				Err()
		}
		if vers.IsSatisfiedBy(iv) {
			return &i, nil
		}
	}

	return nil, errors.New("no Python found")
}
