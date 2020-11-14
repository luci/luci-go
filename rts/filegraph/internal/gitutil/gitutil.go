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

package gitutil

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// RefExists returns true if the git ref exists.
func RefExists(repoDir, ref string) (bool, error) {
	// Pass -- so that git knows that the argument after rev-parse is a ref
	// and not a file path.
	switch _, err := Exec(repoDir)("rev-parse", ref, "--"); {
	case err == nil:
		return true, nil
	case strings.Contains(err.Error(), "bad revision"):
		return false, nil
	default:
		return false, err
	}
}

// Exec returns a function that executes a git command and returns its
// standard output.
// The context must be a path to an existing file or directory.
//
// It is suitable only for commands that exit quickly and have small
// output, e.g. rev-parse.
func Exec(context string) func(args ...string) (out string, err error) {
	exe := "git"
	if runtime.GOOS == "windows" {
		exe = "git.exe"
	}
	return func(args ...string) (string, error) {
		dir, err := dirFromPath(context)
		if err != nil {
			return "", err
		}
		args = append([]string{"-C", dir}, args...)
		cmd := exec.Command(exe, args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		outBytes, err := cmd.Output()
		out := strings.TrimSuffix(string(outBytes), "\n")
		return out, errors.Annotate(err, "git %q failed; output: %q", args, stderr.Bytes()).Err()
	}
}

// dirFromPath returns fileName if it is a dir, otherwise returns fileName's
// dir. The file/dir must exist.
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
