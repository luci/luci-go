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

package filesystem

import (
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/luci/common/errors"
)

// IsNotExist calls os.IsNotExist on the unwrapped err.
func IsNotExist(err error) bool { return os.IsNotExist(errors.Unwrap(err)) }

// MakeDirs is a convenience wrapper around os.MkdirAll that applies a 0755
// mask to all created directories.
func MakeDirs(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return errors.Annotate(err, "").Err()
	}
	return nil
}

// AbsPath is a convenience wrapper around filepath.Abs that accepts a string
// pointer, base, and updates it on successful resolution.
func AbsPath(base *string) error {
	v, err := filepath.Abs(*base)
	if err != nil {
		return errors.Annotate(err, "unable to resolve absolute path").
			InternalReason("base(%q)", *base).Err()
	}
	*base = v
	return nil
}

// Touch creates a new, empty file at the specified path.
//
// If when is zero-value, time.Now will be used.
func Touch(path string, when time.Time, mode os.FileMode) error {
	// Try and create a file at the target path.
	fd, err := os.OpenFile(path, (os.O_CREATE | os.O_RDWR), mode)
	if err == nil {
		if err := fd.Close(); err != nil {
			return errors.Annotate(err, "failed to close new file").Err()
		}
		if when.IsZero() {
			// If "now" was specified, and we created a new file, then its times will
			// be now by default.
			return nil
		}
	}

	// Couldn't create a new file. Either it exists already, it is a directory,
	// or there was an OS-level failure. Since we can't really distinguish
	// between these cases, try opening for write (update timestamp) and error
	// if this fails.
	if when.IsZero() {
		when = time.Now()
	}
	if err := os.Chtimes(path, when, when); err != nil {
		return errors.Annotate(err, "failed to Chtimes").InternalReason("path(%q)", path).Err()
	}

	return nil
}

// RemoveAll is a wrapper around os.RemoveAll which makes sure all files are
// writeable (recursively) prior to removing them.
//
// If the specified path does not exist, RemoveAll will return nil.
func RemoveAll(path string) error {
	err := removeAllImpl(path, func(path string, fi os.FileInfo) error {
		// If we aren't handed a FileInfo, use Lstat to get one.
		if fi == nil {
			var err error
			if fi, err = os.Lstat(path); err != nil {
				return errors.Annotate(err, "could not Lstat path").InternalReason("path(%q)", path).Err()
			}
		}

		// Make user-writable, if it's not already.
		if err := MakePathUserWritable(path, fi); err != nil {
			return err
		}

		if err := os.Remove(path); err != nil {
			return errors.Annotate(err, "failed to remove path").InternalReason("path(%q)", path).Err()
		}
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "failed to recurisvely remove path").InternalReason("path(%q)", path).Err()
	}
	return nil
}

// MakeReadOnly recursively iterates through all of the files and directories
// starting at path and marks them read-only.
func MakeReadOnly(path string, filter func(string) bool) error {
	return recursiveChmod(path, filter, func(mode os.FileMode) os.FileMode {
		return mode & (^os.FileMode(0222))
	})
}

// MakePathUserWritable updates the filesystem metadata on a single file or
// directory to make it user-writable.
//
// fi is optional. If nil, os.Stat will be called on path. Otherwise, fi will
// be regarded as the results of calling os.Stat on path. This is provided as
// an optimization, since some filesystem operations automatically yield a
// FileInfo.
func MakePathUserWritable(path string, fi os.FileInfo) error {
	if fi == nil {
		var err error
		if fi, err = os.Stat(path); err != nil {
			return errors.Annotate(err, "failed to Stat path").InternalReason("path(%q)", path).Err()
		}
	}

	// Make user-writable, if it's not already.
	mode := fi.Mode()
	if (mode & 0200) == 0 {
		mode |= 0200
		if err := os.Chmod(path, mode); err != nil {
			return errors.Annotate(err, "could not Chmod path").InternalReason("mode(%#o)/path(%q)", mode, path).Err()
		}
	}
	return nil
}

func recursiveChmod(path string, filter func(string) bool, chmod func(mode os.FileMode) os.FileMode) error {
	if filter == nil {
		filter = func(string) bool { return true }
	}

	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.Annotate(err, "").Err()
		}

		mode := info.Mode()
		if (mode.IsRegular() || mode.IsDir()) && filter(path) {
			if newMode := chmod(mode); newMode != mode {
				if err := os.Chmod(path, newMode); err != nil {
					return errors.Annotate(err, "failed to Chmod").InternalReason("path(%q)", path).Err()
				}
			}
		}
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "").Err()
	}
	return nil
}
