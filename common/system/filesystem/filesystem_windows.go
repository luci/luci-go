// Copyright 2019 The LUCI Authors.
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
	"syscall"

	"golang.org/x/sys/windows"
)

func umask(mask int) int {
	return 0
}

func addReadMode(mode os.FileMode) os.FileMode {
	return mode | syscall.S_IRUSR
}

func getFreeSpace(path string) (uint64, error) {
	wpath, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var freeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(wpath, nil, nil, &freeBytes); err != nil {
		return 0, err
	}
	return freeBytes, nil
}
