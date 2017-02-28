// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package filesystem

import (
	"io"
	"os"
	"syscall"

	"github.com/luci/luci-go/common/errors"
)

// removeAllImpl removes path and any children it contains.
// It removes everything it can but returns the first error
// it encounters. If the path does not exist, RemoveAll
// returns nil (no error).
//
// removeAllImpl is a clone of Go's os.RemoveAll which is a wrapper around
// os.RemoveAll which uses the supplied removeFunc for deletion. removeFunc is
// given a path and an optional os.FileMode (will be nil if absent).
//
// This was copied from:
// https://github.com/golang/go/blob/964639cc338db650ccadeafb7424bc8ebb2c0f6c/src/os/path.go
func removeAllImpl(path string, removeFunc func(string, os.FileInfo) error) error {
	// Simple case: if Remove works, we're done.
	err := removeFunc(path, nil)
	if err == nil || IsNotExist(err) {
		return nil
	}

	// Otherwise, is this a directory we need to recurse into?
	dir, serrWrapped := os.Lstat(path)
	if serrWrapped != nil {
		serr := errors.Unwrap(serrWrapped)
		if serr, ok := serr.(*os.PathError); ok && (IsNotExist(serr.Err) || serr.Err == syscall.ENOTDIR) {
			return nil
		}
		return serr
	}
	if !dir.IsDir() {
		// Not a directory; return the error from Remove.
		return err
	}

	// Directory.
	fd, err := os.Open(path)
	if err != nil {
		if IsNotExist(err) {
			// Race. It was deleted between the Lstat and Open.
			// Return nil per RemoveAll's docs.
			return nil
		}
		return err
	}

	// Remove contents & return first error.
	err = nil
	for {
		names, err1 := fd.Readdirnames(100)
		for _, name := range names {
			err1 := removeAllImpl(path+string(os.PathSeparator)+name, removeFunc)
			if err == nil {
				err = err1
			}
		}
		if err1 == io.EOF {
			break
		}
		// If Readdirnames returned an error, use it.
		if err == nil {
			err = err1
		}
		if len(names) == 0 {
			break
		}
	}

	// Close directory, because windows won't remove opened directory.
	fd.Close()

	// Remove directory.
	err1 := removeFunc(path, dir)
	if err1 == nil || IsNotExist(err1) {
		return nil
	}
	if err == nil {
		err = err1
	}
	return err
}
