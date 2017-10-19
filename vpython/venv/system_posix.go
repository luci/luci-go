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

// +build darwin linux freebsd netbsd openbsd android

package venv

import (
	"os"
	"path/filepath"
	"syscall"

	"go.chromium.org/luci/common/errors"
)

// venvBinDir resolves the path where VirtualEnv binaries are installed.
func venvBinDir(root string) string {
	return filepath.Join(root, "bin")
}

func checkProcessRunning(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return errors.Annotate(err, "failed to find process").Err()
	}

	if err := proc.Signal(os.Signal(syscall.Signal(0))); err != nil {
		return errors.Annotate(err, "failed to signal process").Err()
	}
	return nil
}
