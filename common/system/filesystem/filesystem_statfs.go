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

//go:build unix && !(netbsd || openbsd || solaris || illumos)
// +build unix,!netbsd,!openbsd,!solaris,!illumos

package filesystem

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func umask(mask int) int {
	return syscall.Umask(mask)
}

func addReadMode(mode os.FileMode) os.FileMode {
	return mode | 0444
}

func getFreeSpace(path string) (uint64, error) {
	var statfs unix.Statfs_t
	if err := unix.Statfs(path, &statfs); err != nil {
		return 0, err
	}
	return uint64(statfs.Bavail) * uint64(statfs.Bsize), nil
}
