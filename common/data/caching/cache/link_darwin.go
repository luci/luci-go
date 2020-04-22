// Copyright 2020 The LUCI Authors.
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

import "golang.org/x/sys/unix"

func link(src, dst string) error {
	// man page of linkat for OS X says that
	// `not assigning AT_SYMLINK_FOLLOW to flag may result in some filesystems return-
	// ing an error.` in
	// https://opensource.apple.com/source/xnu/xnu-3789.21.4/bsd/man/man2/link.2.auto.html
	// So this is skeptical linkat usage for crbug.com/1073368
	return unix.Linkat(unix.AT_FDCWD, src, unix.AT_FDCWD, dst, unix.AT_SYMLINK_FOLLOW)
}
