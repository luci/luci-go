// Copyright 2021 The LUCI Authors.
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

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"

	"go.chromium.org/luci/common/errors"
)

// goList calls "go list -json <args>" and parses the result.
func goList(args []string, out any) error {
	cmd := exec.Command("go", append([]string{"list", "-json"}, args...)...)
	buf, err := cmd.Output()
	if err != nil {
		if er, ok := err.(*exec.ExitError); ok && len(er.Stderr) > 0 {
			return errors.Fmt("%s", er.Stderr)
		}
		return err
	}
	return json.Unmarshal(buf, out)
}

// isInPackage returns true if the filename is a part of the package.
func isInPackage(fileName string, pkg string) (bool, error) {
	var pkgInfo struct {
		Dir string
	}
	if err := goList([]string{"-find", pkg}, &pkgInfo); err != nil {
		return false, err
	}
	expectedDir, err := os.Stat(pkgInfo.Dir)
	if err != nil {
		return false, err
	}
	actualDir, err := os.Stat(filepath.Dir(fileName))
	if err != nil {
		return false, err
	}
	return os.SameFile(expectedDir, actualDir), nil
}
