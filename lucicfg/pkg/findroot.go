// Copyright 2025 The LUCI Authors.
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

package pkg

import (
	"errors"
	"os"
	"path/filepath"
)

// Used to discover boundaries of repositories.
var repoSentinel = []string{".git", ".citc"}

// findRoot, given a directory path on disk, finds the closest repository or
// volume root directory and returns it as an absolute path.
//
// If given a markerFile, will stop searching if finds a directory that contains
// this file, returning (dir path, true, nil) in that case.
func findRoot(dir, markerFile string) (string, bool, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", false, err
	}

	var probes []string
	if markerFile == "" {
		probes = repoSentinel
	} else {
		// Note the order is important: need to probe for the marker file before
		// probing .git in case the marker file is at the repo root.
		probes = append(make([]string, 0, 3), markerFile)
		probes = append(probes, repoSentinel...)
	}

	for {
		for _, probe := range probes {
			switch _, err := os.Stat(filepath.Join(dir, probe)); {
			case err == nil:
				return dir, probe == markerFile, nil // found the repository root or the marker file
			case errors.Is(err, os.ErrNotExist):
				// Carry on searching
			default:
				// Some file system error (likely no access).
				return "", false, err
			}
		}

		up := filepath.Dir(dir)
		if up == dir {
			return dir, false, nil // hit the volume root
		}

		dir = up
	}
}
