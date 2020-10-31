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

package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// RepoDir returns an absolute path to the repo that fileName belongs to.
func RepoDir(fileName string) (string, error) {
	return Run(fileName, "rev-parse", "--show-toplevel")
}

// AbsoluteGitDir returns an absolute path to the git dir of the repo that
// fileName belongs to.
func AbsoluteGitDir(fileName string) (string, error) {
	return Run(fileName, "rev-parse", "--absolute-git-dir")
}

// Run executes a git command in the git repo that fileName belongs to,
// and returns standard output.
func Run(fileName string, args ...string) (out string, err error) {
	dir, err := dirFromPath(fileName)
	if err != nil {
		return "", err
	}

	args = append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", args...)
	outBytes, err := cmd.Output()
	out = strings.TrimSuffix(string(outBytes), "\n")
	return out, errors.Annotate(err, "git %q failed", args).Err()
}

// dirFromPath returns fileName as is if it points to a dir, otherwise returns
// fileName's dir.
func dirFromPath(fileName string) (dir string, err error) {
	switch stat, err := os.Stat(fileName); {
	case err != nil:
		return "", err
	case stat.IsDir():
		return fileName, nil
	default:
		return filepath.Dir(fileName), nil
	}
}
