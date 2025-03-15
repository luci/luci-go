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

// Package fileset contains glob-like pattern matching functionality.
//
// Used to describe a set of files in some root directory.
package fileset

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// Set represents a set of files under some root directory.
//
// It can answer a question if some concrete slash-separate path belong to the
// set or not.
type Set struct {
	positive []string // globs included in the set
	negative []string // globs excluded from the set
}

// Contains returns true if the given slash-separated clear relative path belong
// to the set.
//
// Returns an error if the set or the path are malformed.
func (s *Set) Contains(path string) (bool, error) {
	if isPos, err := matchesAny(path, s.positive); !isPos || err != nil {
		return false, err
	}
	if isNeg, err := matchesAny(path, s.negative); isNeg || err != nil {
		return false, err
	}
	return true, nil
}

// New constructs a file set.
//
// Each entry in `patterns` is either `<glob pattern>` (a "positive" glob) or
// `!<glob pattern>` (a "negative" glob). A path is considered to be in the set
// if its matches any of the positive globs and none of the negative globs.
//
// If `patterns` is empty, the set is considered empty. If all patterns
// are negative, single `**/*` positive pattern is implied as well.
//
// If a glob pattern starts with "**/", its remaining portion indicate a pattern
// for the file name portion of the path (e.g. "**/*.star" means any Starlark
// file regardless of its directory). Otherwise the glob is applied to the
// full slash-separated path of the file (e.g. "dir/*.star" means Starlark
// files directly under "dir/").
func New(patterns []string) (*Set, error) {
	var pos, neg []string
	for _, pat := range patterns {
		if strings.HasPrefix(pat, "!") {
			neg = append(neg, pat[1:])
		} else {
			pos = append(pos, pat)
		}
	}
	if len(pos) == 0 && len(neg) != 0 {
		pos = []string{"**/*"}
	}
	// Unfortunately, there's no simple way to check the pattern syntax is valid
	// without actually matching against some input that fully matches the
	// pattern. So we always return nil error here for now. Bad patterns will
	// result in Contains returning an error.
	return &Set{
		positive: pos,
		negative: neg,
	}, nil
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

// ScanDirectory recursively discovers all regular files in the given
// directory `dir` whose paths relative to `dir` belong to the given set.
//
// See NewSet for the format of `patterns`. If the directory doesn't exist or
// the set is empty, returns empty slice.
//
// Returned paths are sorted, slash-separated and relative to `dir`.
func ScanDirectory(dir string, set *Set) ([]string, error) {
	// Avoid scanning the directory if the tracked set is known to be empty.
	if len(set.positive) == 0 {
		return nil, nil
	}

	// Missing directory is considered empty.
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}

	var found []string
	err := filepath.WalkDir(dir, func(p string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.Type().IsRegular() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		yes, err := set.Contains(rel)
		if yes {
			found = append(found, rel)
		}
		return err
	})
	if err != nil {
		if errors.Is(err, path.ErrBadPattern) {
			return nil, err
		}
		return nil, errors.Annotate(err, "failed to scan the directory").Err()
	}

	slices.Sort(found)
	return found, nil
}
