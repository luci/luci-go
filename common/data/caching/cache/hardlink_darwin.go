// Copyright 2022 The LUCI Authors.
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

package cache

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

var cachedMajorVersion int
var cachedMajorVersionOnce sync.Once

func mustGetMajorVersion() int {
	cachedMajorVersionOnce.Do(func() {
		var utsname unix.Utsname
		if err := unix.Uname(&utsname); err != nil {
			panic(fmt.Sprintf("failed to call uname(): %v", err))
		}

		release := string(utsname.Release[:])
		vers := strings.Split(release, ".")
		if len(vers) < 2 {
			panic(fmt.Sprintf("unexpected release from uname: %s", string(release)))
		}

		var err error
		cachedMajorVersion, err = strconv.Atoi(vers[0])
		if err != nil {
			panic(fmt.Sprintf("failed to parse version %s: %v", vers[0], err))
		}
	})
	return cachedMajorVersion
}

func makeHardLinkOrClone(src, dst string) error {
	// Hardlinked executables don't work well with dyld.
	// Use clonefile instead on macOS 12 or newer to workaround that.
	// ref: https://crbug.com/1296318#c54
	if mustGetMajorVersion() >= 12 {
		return unix.Clonefile(src, dst, unix.CLONE_NOFOLLOW|unix.CLONE_NOOWNERCOPY)
	}
	return os.Link(src, dst)
}
