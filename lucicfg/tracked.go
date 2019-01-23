// Copyright 2019 The LUCI Authors.
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

package lucicfg

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// FindTrackedFiles recursively discovers all regular files in the given
// directory that match given patterns.
//
// Each entry in `patterns` is either `<glob pattern>` (a "positive" glob) or
// `!<glob pattern>` (a "negative" glob). A file under `dir` is considered
// tracked if it matches any of the positive globs and none of the negative
// globs. If `patterns` is empty, no files are considered tracked.
//
// Returned file names are sorted, slash-separated and relative to `dir`.
func FindTrackedFiles(dir string, patterns []string) ([]string, error) {
	if len(patterns) == 0 {
		return nil, nil
	}

	// Categorize patterns into "positive" and "negative"
	var pos, neg []string
	for _, pat := range patterns {
		if strings.HasPrefix(pat, "!") {
			neg = append(neg, pat[1:])
		} else {
			pos = append(pos, pat)
		}
	}

	var tracked []string
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || !info.Mode().IsRegular() {
			return err
		}

		isPos, err := matchesAny(info.Name(), pos)
		if err != nil {
			return err
		}
		isNeg, err := matchesAny(info.Name(), neg)
		if err != nil {
			return err
		}
		if !isPos || isNeg {
			return nil // not tracked
		}

		rel, err := filepath.Rel(dir, p)
		if err == nil {
			tracked = append(tracked, filepath.ToSlash(rel))
		}
		return err
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to scan the directory for tracked files").Err()
	}

	sort.Strings(tracked)
	return tracked, nil
}

func matchesAny(name string, pats []string) (yes bool, err error) {
	for _, pat := range pats {
		switch match, err := filepath.Match(pat, name); {
		case err != nil:
			return false, errors.Annotate(err, "bad pattern %q", pat).Err()
		case match:
			return true, nil
		}
	}
	return false, nil
}
