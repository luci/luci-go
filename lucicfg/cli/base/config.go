// Copyright 2022 The LUCI Authors.
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

// Package base contains code shared by other CLI subpackages.
package base

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/bazelbuild/buildtools/build"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/filesystem"
	"go.chromium.org/luci/lucicfg/buildifier"
	"google.golang.org/protobuf/encoding/protojson"
)

// ConfigName is the file name we will be used for lucicfg formatting
const ConfigName = ".lucicfgfmtrc"

// GuessRewriterConfig walks up the filesystem from the common ancestor of `paths` to find a
// .lucicfgfmtrc file, parses it and returns a Rewriter from it.
//
// If no config file exists, this returns a default Rewriter.
//
// This function also calls CheckBogusConfig and will return an error if one is found.
func GuessRewriterConfig(paths []string) (*build.Rewriter, error) {
	// Find the common ancestor
	commonAncestorPath, err := filesystem.GetCommonAncestor(paths, []string{".git"})
	if err != nil {
		return nil, err
	}

	if err := CheckForBogusConfig(commonAncestorPath); err != nil {
		return nil, err
	}

	if path, err := findConfigPathUpwards(commonAncestorPath); err != nil {
		return nil, err
	} else {
		// Notify users that a config file was found
		if path != "" {
			fmt.Printf("\nConfig file found at %s\n", path)
		}
		return LoadRewriterFromConfig(path)
	}
}

// CheckForBogusConfig will look for any config files contained in a subdirectory of entryPath
// (recursively).
//
// Because we intend for there to be at most one config file per workspace, and for that config
// file to be located at the root of the workspace, any such extra config files would be errors.
// Due to the 'stateless' nature of fmt and lint, we search down the directory hierarchy here to
// try to detect such misconfiguration, but in the future when these subcommands become
// stateful (like validate currently is), we may remove this check.
func CheckForBogusConfig(entryPath string) error {
	// Traverse downwards
	if err := filepath.WalkDir(entryPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip checking of entry path, downwards is exclusive
		if d.IsDir() && path != entryPath {
			if _, err := os.Stat(filepath.Join(path, ConfigName)); err == nil {
				return errors.Reason(
					"\nFound a config in a subdirectory<%s> of a star file."+
						"Please move to the highest common ancestor directory - %s\n",
					path,
					entryPath).Err()
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	} else {
		return nil
	}
}

func findConfigPathUpwards(path string) (string, error) {
	var currentDir = path
	for {
		if _, err := os.Stat(filepath.Join(currentDir, ConfigName)); err == nil {
			return filepath.Join(currentDir, ConfigName), nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		} else {
			var parent = filepath.Dir(currentDir)

			if _, err := os.Stat(filepath.Join(path, ".git")); err == nil || parent == currentDir {
				return "", nil
			}

			currentDir = parent
		}
	}
}

func convertOrderingToTable(nameOrdering []string) map[string]int {
	count := len(nameOrdering)
	table := make(map[string]int, count)
	// This sequentially gives the names a priority value in the range
	// [-count, 0). This ensures that all names have distinct priority
	// values that sort them in the specified order. Since all priority
	// values are less than the default 0, all names present in the
	// ordering will sort before names that don't appear in the ordering.
	for i, n := range nameOrdering {
		table[n] = i - count
	}
	return table
}

func rewriterFromConfig(nameOrdering map[string]int) *build.Rewriter {
	var rewriter = &build.Rewriter{
		RewriteSet: []string{
			"listsort",
			"loadsort",
			"formatdocstrings",
			"reorderarguments",
			"editoctal",
		},
	}
	if nameOrdering != nil {
		rewriter.NamePriority = nameOrdering
		rewriter.RewriteSet = append(rewriter.RewriteSet, "callsort")
	}
	return rewriter
}

// LoadRewriterFromConfig will return a rewriter given the path of the
// config file.
//
// Note - if path is an empty string, we will return a default rewriter object
func LoadRewriterFromConfig(path string) (*build.Rewriter, error) {
	var rewriter = rewriterFromConfig(nil)
	if path != "" {
		contents, err := os.ReadFile(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				fmt.Printf("Failed on reading file - %s", path)
				return nil, err
				// Return empty rewriter if file does not exist
			} else {
				return rewriter, nil
			}
		}

		luci := &buildifier.LucicfgFmtConfig{}
		if err := protojson.Unmarshal(contents, luci); err != nil {
			return nil, err
		} else {
			rewriter = rewriterFromConfig(convertOrderingToTable(
				luci.ArgumentNameOrdering),
			)
		}
		fmt.Printf("\nFound config at %s\n", path)
	}
	return rewriter, nil
}
