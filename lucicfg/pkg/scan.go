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
	"fmt"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
)

const legacyConfig = ".lucicfgfmtrc"

// ScanResult is one of the discovered root directories.
type ScanResult struct {
	// Root is the absolute path to the discovered root.
	Root string

	// IsPackage is true if this root contains PACKAGE.star and thus contains
	// a real lucicfg package.
	//
	// It is false for other roots that contain "legacy" packages. This exists for
	// compatibility with the code that isn't using PACKAGE.star yet.
	IsPackage bool

	// Files are absolute paths to *.star files under this root.
	//
	// This is a subset of files passed to ScanForRoots (i.e. NOT all files in
	// the package). Guaranteed to have at least one entry.
	Files []string
}

// RelFiles returns slash-separated paths relative to the root.
//
// This is the same set as s.Files, just in a more normalized form suitable
// for passing to a interpreter.Loader.
func (s *ScanResult) RelFiles() []string {
	out := make([]string, len(s.Files))
	for i, p := range s.Files {
		rel, err := filepath.Rel(s.Root, p)
		if err != nil {
			panic(fmt.Sprintf("filepath.Rel(%q, %q): %s", s.Root, p, err))
		}
		out[i] = filepath.ToSlash(rel)
	}
	return out
}

// ScanForRoots takes a list of paths and groups Starlark files in them by
// the package they belong to.
//
// Paths can either point to concrete Starlark files or to directories (which
// will be recursively traversed to find all Starlark files in them).
//
// If `paths` is empty, will scan the current directory.
//
// Returns two kinds of entries:
//   - Entries representing discovered packages with PACKAGE.star file.
//   - Entries representing "guessed" packages, for compatibility with code
//     without PACKAGE.star. Uses presence of ".lucicfgfmtrc" to "guess" the
//     root, falling back to a repository boundary otherwise.
//
// Does not attempt to interpret PACKAGE.star (use PackageOnDisk for that).
func ScanForRoots(paths []string) ([]*ScanResult, error) {
	return scanForRoots(paths, "")
}

// scanForRoots implements ScanForRoots.
//
// It allows to supply a "stop directory" that limits searches for package roots
// to it (instead of the repository root).
func scanForRoots(paths []string, stopDir string) ([]*ScanResult, error) {
	results := []*ScanResult{}              // all discovered roots
	perDirRoots := map[string]*ScanResult{} // abs dir path => closest root
	allRoots := map[string]*ScanResult{}    // abs root path => *ScanResult for it
	seen := stringset.New(len(paths))       // all visited leaf files

	cache := unsyncStatCache()

	findPkgRoot := func(dir string) (*ScanResult, error) {
		if found := perDirRoots[dir]; found != nil {
			return found, nil
		}

		root, marker, err := findAnyRoot(dir, stopDir, cache)
		if err != nil {
			return nil, err
		}
		if res := allRoots[root]; res != nil {
			// Already seen this root. Cache `findAnyRoot` resolution.
			perDirRoots[dir] = res
			return res, nil
		}

		// Found a new root!
		res := &ScanResult{
			Root:      root,
			IsPackage: marker == PackageScript,
		}
		results = append(results, res)
		allRoots[root] = res
		perDirRoots[dir] = res
		return res, nil
	}

	for path, err := range expandDirs(paths) {
		if err != nil {
			return nil, err
		}
		if !seen.Add(path) {
			continue
		}
		pkgRes, err := findPkgRoot(filepath.Dir(path))
		if err != nil {
			return nil, err
		}
		pkgRes.Files = append(pkgRes.Files, path)
	}

	slices.SortFunc(results, func(a, b *ScanResult) int {
		return strings.Compare(a.Root, b.Root)
	})
	for _, r := range results {
		slices.Sort(r.Files)
	}

	return results, nil
}

// findAnyRoot finds either a root with PACKAGE.star or a legacy config or
// the stopDir, or a repo root.
//
// `marker` is either "PACKAGE.star" or ".lucicfgfmtrc" or "" depending on what
// kind of root was found.
func findAnyRoot(dir, stopDir string, cache *statCache) (root, marker string, err error) {
	switch root, found, err := findRoot(dir, PackageScript, stopDir, cache); {
	case err != nil:
		return "", "", err
	case found:
		return root, PackageScript, nil
	}
	switch root, found, err := findRoot(dir, legacyConfig, stopDir, cache); {
	case err != nil:
		return "", "", err
	case found:
		return root, legacyConfig, nil
	default:
		return root, "", nil // stopDir or repo root
	}
}

// expandDirs iterates over recursive traversal of given paths.
//
// Paths can either be files (will be yielded as is) or directories (will be
// recursively traversed in search for *.star files).
func expandDirs(paths []string) iter.Seq2[string, error] {
	if len(paths) == 0 {
		paths = []string{"."}
	}

	return func(yield func(string, error) bool) {
		for _, p := range paths {
			p, err := filepath.Abs(p)
			if err != nil {
				yield(p, errors.Annotate(err, "could not absolutize %q", p).Err())
				return
			}

			switch info, err := os.Stat(p); {
			case err != nil:
				yield(p, err)
				return

			case !info.IsDir():
				if !yield(p, nil) {
					return
				}

			default:
				err := filepath.WalkDir(p, func(path string, entry fs.DirEntry, err error) error {
					if err == nil && !entry.IsDir() && strings.HasSuffix(path, ".star") {
						if !yield(path, nil) {
							return fs.SkipAll
						}
					}
					return err
				})
				if err != nil {
					yield(p, err)
					return
				}
			}
		}
	}
}
