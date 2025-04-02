// Copyright 2025 The LUCI Authors.
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

package oscompat

import (
	"os"
	"syscall"
)

// OpenSharedDelete opens a file with shared delete.
//
// This should be used to open files which may be deleted while it's
// being read, like for files that get replaced by a different copy to
// emulate atomic writes.
//
// See also https://groups.google.com/d/topic/golang-dev/dS2GnwizSkk
func OpenSharedDelete(name string) (*os.File, error) {
	lpFileName, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	handle, err := syscall.CreateFile(
		lpFileName,
		uint32(syscall.GENERIC_READ),
		uint32(syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE),
		nil,
		uint32(syscall.OPEN_EXISTING),
		syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), name), nil
}
