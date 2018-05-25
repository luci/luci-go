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

// +build !windows

package filesystem

import (
	"os"

	"go.chromium.org/luci/common/errors"
)

// removeOne removes one file or directory on non-Windows filesystems.
//
// This code works with filesystems which allow removal of file so long as its
// parent directory is writable.
func removeOne(path string, _ os.FileInfo) error {
	if err := MakePathUserWritable(path, nil); err != nil {
		return errors.Annotate(err, "failed to make writable path").InternalReason("path(%q)", path).Err()
	}
	if err := os.Remove(path); err != nil {
		return errors.Annotate(err, "failed to remove path").InternalReason("path(%q)", path).Err()
	}
	return nil
}
