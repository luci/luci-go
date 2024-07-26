// Copyright 2024 The LUCI Authors.
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

package host

import (
	"os"
	"path/filepath"
	"runtime"

	"go.chromium.org/luci/common/errors"
)

func resolveExe(path string) (string, error) {
	if filepath.Ext(path) != "" {
		return path, nil
	}

	lme := errors.NewLazyMultiError(2)
	for i, ext := range []string{".exe", ".bat"} {
		candidate := path + ext
		if _, err := os.Stat(candidate); !lme.Assign(i, err) {
			return candidate, nil
		}
	}

	me := lme.Get().(errors.MultiError)
	return path, errors.Reason("cannot find .exe (%q) or .bat (%q)", me[0], me[1]).Err()
}

// processCmd resolves the cmd by constructing the absolute path and resolving
// the exe suffix.
func processCmd(path, cmd string) (string, error) {
	relPath := filepath.Join(path, cmd)
	absPath, err := filepath.Abs(relPath)
	if err != nil {
		return "", errors.Annotate(err, "absoluting %q", relPath).Err()
	}
	if runtime.GOOS == "windows" {
		absPath, err = resolveExe(absPath)
		if err != nil {
			return "", errors.Annotate(err, "resolving %q", absPath).Err()
		}
	}
	return absPath, nil
}
