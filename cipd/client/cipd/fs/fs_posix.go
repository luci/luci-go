// Copyright 2015 The LUCI Authors.
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

package fs

import (
	"os"
	"syscall"
)

func openFile(path string) (*os.File, error) {
	return os.Open(path)
}

func atomicRename(source, target string) error {
	return os.Rename(source, target)
}

func errnoNotEmpty(err error) bool {
	return err == syscall.ENOTEMPTY || err == syscall.EEXIST || err == syscall.EISDIR
}

func errnoNotDir(err error) bool {
	return err == syscall.ENOTDIR
}

func errnoAccessDenied(err error) bool {
	return false
}
