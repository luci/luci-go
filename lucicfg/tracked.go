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
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// TrackedSet returns a predicate that classifies whether a slash-separated path
// belongs to a tracked set or not.
//
// Each entry in `patterns` is either `<glob pattern>` (a "positive" glob) or
// `!<glob pattern>` (a "negative" glob). A path is considered tracked if its
// base name matches any of the positive globs and none of the negative globs.
// If `patterns` is empty, no paths are considered tracked. If all patterns
// are negative, single `**/*` positive pattern is implied as well.
//
// The predicate returns an error if some pattern is malformed.
func TrackedSet(patterns []string) func(string) (bool, error) {
	if len(patterns) == 0 {
		return func(string) (bool, error) { return false, nil }
	}

	var pos, neg []string
	for _, pat := range patterns {
		if strings.HasPrefix(pat, "!") {
			neg = append(neg, pat[1:])
		} else {
			pos = append(pos, pat)
		}
	}

	if len(pos) == 0 {
		pos = []string{"**/*"}
	}

	return func(p string) (bool, error) {
		if isPos, err := matchesAny(p, pos); !isPos || err != nil {
			return false, err
		}
		if isNeg, err := matchesAny(p, neg); isNeg || err != nil {
			return false, err
		}
		return true, nil
	}
}

// FindTrackedFiles recursively discovers all regular files in the given
// directory whose names match given patterns.
//
// See TrackedSet for the format of `patterns`. If the directory doesn't exist,
// returns empty slice.
//
// Returned file names are sorted, slash-separated and relative to `dir`.
func FindTrackedFiles(dir string, patterns []string) ([]string, error) {
	// Avoid scanning the directory if the tracked set is known to be empty.
	if len(patterns) == 0 {
		return nil, nil
	}

	// Missing directory is considered empty.
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}

	isTracked := TrackedSet(patterns)

	var tracked []string
	err := filepath.WalkDir(dir, func(p string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.Type().IsRegular() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		yes, err := isTracked(rel)
		if yes {
			tracked = append(tracked, rel)
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
		subject := name
		if strings.HasPrefix(pat, "**/") {
			pat = pat[3:]
			subject = path.Base(name)
		}
		switch match, err := path.Match(pat, subject); {
		case err != nil:
			return false, errors.Annotate(err, "bad pattern %q", pat).Err()
		case match:
			return true, nil
		}
	}
	return false, nil
}
