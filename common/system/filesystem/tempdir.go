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

import "os"

// TempDir configures a temporary directory.
type TempDir struct {
	// Dir is the base diectory. If empty, the default will be used (see
	// os.MkdirTemp)
	Dir string

	// Prefix is the prefix to apply to the temporary directory. If empty, a
	// default will be used (see os.MkdirTemp).
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
	tdir, err := os.MkdirTemp(td.Dir, td.Prefix)
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
