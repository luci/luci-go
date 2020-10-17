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

// AbsoluteGitPath returns absolute path to the git dir.
func AbsoluteGitPath(path string) (string, error) {
	return Run(path, "rev-parse", "--absolute-git-dir")
}

// DirFromPath returns path as is if it is a dir, otherwise the dir containing
// the file at path.
func DirFromPath(path string) (dir string, err error) {
	switch stat, err := os.Stat(path); {
	case err != nil:
		return "", err
	case stat.IsDir():
		return path, nil
	default:
		return filepath.Dir(path), nil
	}
}

// RepoRoot returns path to the repo root.
func RepoRoot(path string) (string, error) {
	return Run(path, "rev-parse", "--show-toplevel")
}

// Run executes a git command.
func Run(path string, args ...string) (output string, err error) {
	dir, err := DirFromPath(path)
	if err != nil {
		return "", err
	}

	args = append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", args...)
	stdoutBytes, err := cmd.Output()
	output = strings.TrimSuffix(string(stdoutBytes), "\n")
	return output, errors.Annotate(err, "git %q failed", args).Err()
}
