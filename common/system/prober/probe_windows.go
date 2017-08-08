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

package prober

import (
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"
)

func chkStat(file string) error {
	d, err := os.Stat(file)
	if err != nil {
		return err
	}
	if d.IsDir() {
		return os.ErrPermission
	}
	return nil
}

func hasExt(file string) bool {
	i := strings.LastIndex(file, ".")
	if i < 0 {
		return false
	}
	return strings.LastIndexAny(file, `:\/`) < i
}

func findExecutable(file string, exts []string) (string, error) {
	if len(exts) == 0 {
		return file, chkStat(file)
	}
	if hasExt(file) {
		if chkStat(file) == nil {
			return file, nil
		}
	}
	for _, e := range exts {
		if f := file + e; chkStat(f) == nil {
			return f, nil
		}
	}
	return "", os.ErrNotExist
}

// findInDir is a paraphrased and trimmed version of "exec.LookPath"
// (for Windows),
//
// Copied from:
// https://github.com/golang/go/blob/d234f9a75413fdae7643e4be9471b4aeccf02478/src/os/exec/lp_windows.go
//
// Modified to:
//	- Use a supplied "dir" instead of scanning through PATH.
//	- Not consider cases where "file" is an absolute path
//	- Ignore the possibility that "file" may be in the CWD; only look in "dir".
func findInDir(file, dir string, env environ.Env) (string, error) {
	var exts []string
	if x, ok := env.Get(`PATHEXT`); ok {
		for _, e := range strings.Split(strings.ToLower(x), `;`) {
			if e == "" {
				continue
			}
			if e[0] != '.' {
				e = "." + e
			}
			exts = append(exts, e)
		}
	} else {
		exts = []string{".com", ".exe", ".bat", ".cmd"}
	}

	if f, err := findExecutable(filepath.Join(dir, file), exts); err == nil {
		return f, nil
	}
	return "", errors.New("not found")
}
