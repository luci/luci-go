// Copyright 2018 The LUCI Authors.
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

// +build windows

package filesystem

import (
	"os"

	"go.chromium.org/luci/common/errors"
)

// removeOne removes one file or directory on Windows filesystem.
func removeOne(path string, fi os.FileInfo) error {
	// If we aren't handed a FileInfo, use Lstat to get one.
	if fi == nil {
		var err error
		if fi, err = os.Lstat(path); err != nil {
			return errors.Annotate(err, "could not Lstat path").InternalReason("path(%q)", path).Err()
		}
	}
	//
	// So, follow optimistic removal strategy: try to remove first, on failure
	// try changing permissions and retrying removal.
	errRm := os.Remove(path)
	if errRm == nil {
		return nil
	}
	if errChmod := MakePathUserWritable(path, fi); errChmod != nil {
		return errors.Annotate(errors.NewMultiError(errRm, errChmod),
			"failed to remove and also to make writable path").InternalReason("path(%q)", path).Err()
	}
	if err := os.Remove(path); err != nil {
		return errors.Annotate(err, "failed to remove path").InternalReason("path(%q)", path).Err()
	}
	return nil
}
