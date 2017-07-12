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

// +build darwin dragonfly freebsd linux nacl netbsd openbsd solaris

package prober

import (
	"os"
	"path/filepath"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/system/environ"
)

func findExecutable(file string) error {
	d, err := os.Stat(file)
	if err != nil {
		return err
	}
	if m := d.Mode(); !m.IsDir() && m&0111 != 0 {
		return nil
	}
	return os.ErrPermission
}

// findInDir is a paraphrased and trimmed version of "exec.LookPath"
// (for Windows),
//
// Copied from:
// https://github.com/golang/go/blob/d234f9a75413fdae7643e4be9471b4aeccf02478/src/os/exec/lp_unix.go
//
// Modified to:
//	- Use a supplied "dir" instead of scanning through PATH.
//	- Not consider cases where "file" is an absolute path
//	- Ignore the possibility that "file" may be in the CWD; only look in "dir".
func findInDir(file, dir string, env environ.Env) (string, error) {
	// NOTE(rsc): I wish we could use the Plan 9 behavior here
	// (only bypass the path if file begins with / or ./ or ../)
	// but that would not match all the Unix shells.

	if dir == "" {
		// Unix shell semantics: path element "" means "."
		dir = "."
	}
	path := filepath.Join(dir, file)
	if err := findExecutable(path); err == nil {
		return path, nil
	}
	return "", errors.New("not found")
}
