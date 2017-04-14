// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package filesystem

import (
	"io/ioutil"
)

// TempDir configures a temporary directory.
type TempDir struct {
	// Dir is the base diectory. If empty, the default will be used (see
	// ioutil.TempDir)
	Dir string

	// Prefix is the prefix to apply to the temporary directory. If empty, a
	// default will be used (see ioutil.TempDir).
	Prefix string

	// OnCleanupErr, if not nil, will be called if TempDir cleanup fails.
	//
	// If nil, cleanup errors will be silently discarded.
	CleanupErrFunc func(tdir string, err error)
}

// With creates a temporary directory and passes it to fn. After fn  exits, the
// directory and all of its contents is deleted.
//
// Any error that happens during setup or execution of the callback is returned.
// If an error occurs during cleanup, the optional CleanupErrFunc will be
// called.
func (td *TempDir) With(fn func(string) error) error {
	tdir, err := ioutil.TempDir(td.Dir, td.Prefix)
	if err != nil {
		return err
	}
	defer func() {
		if rmErr := RemoveAll(tdir); rmErr != nil {
			if cef := td.CleanupErrFunc; cef != nil {
				cef(tdir, rmErr)
			}
		}
	}()
	return fn(tdir)
}
