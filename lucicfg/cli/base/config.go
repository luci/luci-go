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
	"strings"

	"github.com/bazelbuild/buildtools/build"
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/filesystem"

	"go.chromium.org/luci/lucicfg/buildifier"
	"go.chromium.org/luci/lucicfg/vars"
)

// ConfigName is the file name we will be used for lucicfg formatting
const ConfigName = ".lucicfgfmtrc"

// sentinel is used to prevent the walking functions in this package from walking
// across a source control boundary. As of 2024 Q2 we are only worried about Git
// repos and cog clients, but should we ever support more VCS's and this walking
// code is still required (i.e. this hasn't been replaced with a WORKSPACE style
// config file), this should be extended.
var sentinel = []string{".git", ".citc"}

// RewriterFactory is used to map from 'file to be formatted' to a Rewriter object,
// via its GetRewriter method.
//
// This struct is obtained via the GetRewriterFactory function.
type RewriterFactory struct {
	rules          []pathRules
	configFilePath string
}

type pathRules struct {
	path  string // absolute path to the folder where this rules applies.
	rules *buildifier.LucicfgFmtConfig_Rules
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
	var rewriter = vars.GetDefaultRewriter()
	if nameOrdering != nil {
		rewriter.NamePriority = nameOrdering
		rewriter.RewriteSet = append(rewriter.RewriteSet, "callsort")
	}
	return rewriter
}

// GetRewriterFactory will attempt to create a RewriterFactory object
//
// If configPath is empty, or points to a file which doesn't exist, the returned
// factory will just produce GetDefaultRewriter() when asked about any path.
// We will return an error if the config file is invalid.
func GetRewriterFactory(configPath string) (rewriterFactory *RewriterFactory, err error) {
	rewriterFactory = &RewriterFactory{
		rules:          []pathRules{},
		configFilePath: "",
	}
	if configPath == "" {
		return
	}
	contents, err := os.ReadFile(configPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Printf("Failed on reading file - %s", configPath)
			return nil, err
		} else {
			return rewriterFactory, nil
		}
	}
	luci := &buildifier.LucicfgFmtConfig{}

	if err := prototext.Unmarshal(contents, luci); err != nil {
		return nil, err
	}
	return getPostProcessedRewriterFactory(configPath, luci)
}

// getPostProcessedRewriterFactory will contain all logic used to make sure
// RewriterFactory is normalized the way we want.
//
// Currently, we will fix paths so that they are absolute.
// We will also perform a check so that there are no duplicate paths and
// all paths are delimited with "/"
func getPostProcessedRewriterFactory(configPath string, cfg *buildifier.LucicfgFmtConfig) (*RewriterFactory, error) {
	pathSet := stringset.New(0)
	rules := cfg.Rules
	rulesSlice := make([]pathRules, 0)
	for ruleIndex, rule := range rules {
		// If a rule doesn't have any paths, err out and notify users
		if len(rule.Path) == 0 {
			return nil, errors.Reason(
				"rule[%d]: Does not contain any paths",
				ruleIndex).Err()
		}
		for rulePathIndex, pathInDir := range rule.Path {
			// Fix paths. Update to use absolute path.
			fixedPathInDir := filepath.Clean(
				filepath.Join(filepath.Dir(configPath), pathInDir),
			)
			// Check for duplicate paths. If there is, return error
			if pathSet.Contains(stringset.NewFromSlice(fixedPathInDir)) {
				return nil, errors.Reason(
					"rule[%d].path[%d]: Found duplicate path '%s'",
					ruleIndex, rulePathIndex, pathInDir).Err()
			}
			// Check for backslash in path, if there is, return error
			if strings.Contains(pathInDir, "\\") {
				return nil, errors.Reason(
					"rule[%d].path[%d]: Path should not contain backslash '%s'",
					ruleIndex, rulePathIndex, pathInDir).Err()
			}
			// Add into set to check later if duplicate
			pathSet.Add(fixedPathInDir)
			if fixedPathInDirAbs, err := filepath.Abs(fixedPathInDir); err != nil {
				return nil, errors.Annotate(err, "rule[%d].path[%d]: filepath.Abs error %s",
					ruleIndex, rulePathIndex, pathInDir).Err()
			} else {
				fixedPathInDir = fixedPathInDirAbs
			}

			rulesSlice = append(rulesSlice, pathRules{
				fixedPathInDir,
				rule,
			})
		}
	}

	return &RewriterFactory{
		rulesSlice,
		filepath.Dir(configPath),
	}, nil
}

// GetRewriter will return the Rewriter which is appropriate for formatting
// the file at `path`, using the previously loaded formatting configuration.
//
// Note the method signature will pass in values that we need to evaluate
// the correct rewriter.
//
// We will accept both relative and absolute paths.
func (f *RewriterFactory) GetRewriter(path string) (*build.Rewriter, error) {
	rules := f.rules
	// Check if path is abs, if not, fix it
	if !filepath.IsAbs(path) {
		return nil, errors.Reason("GetRewriter got non-absolute path: %q", path).Err()
	}
	longestPathMatch := ""
	var matchingRule *buildifier.LucicfgFmtConfig_Rules

	// Find the path that best matches the one we are processing.
	for _, rule := range rules {
		commonAncestor, err := filesystem.GetCommonAncestor(
			[]string{rule.path, path},
			sentinel,
		)

		if err != nil {
			return nil, err
		}

		commonAncestor = filepath.Clean(commonAncestor)
		if commonAncestor == rule.path && len(commonAncestor) > len(longestPathMatch) {
			longestPathMatch = commonAncestor
			matchingRule = rule.rules
		}
	}
	if matchingRule != nil && matchingRule.FunctionArgsSort != nil {
		return rewriterFromConfig(
			convertOrderingToTable(matchingRule.FunctionArgsSort.Arg),
		), nil
	}

	return vars.GetDefaultRewriter(), nil
}

// GuessRewriterFactoryFunc will find the common ancestor dir from all given paths
// and return a func that returns the rewriter factory.
//
// Will look for a config file upwards(inclusive). If found, it will be used to determine
// rewriter properties. It will also look downwards(exclusive) to expose any misplaced
// config files.
func GuessRewriterFactoryFunc(paths []string) (*RewriterFactory, error) {
	// Find the common ancestor
	commonAncestorPath, err := filesystem.GetCommonAncestor(paths, sentinel)

	if errors.Is(err, filesystem.ErrRootSentinel) {
		// we hit the repo root, just return function that returns default rewriter
		rewriterFactory, err := GetRewriterFactory("")
		if err != nil {
			return nil, err
		}
		return rewriterFactory, nil
	}
	if err != nil {
		// other errors are fatal
		return nil, err
	}
	if err := CheckForBogusConfig(commonAncestorPath); err != nil {
		return nil, err
	}

	luciConfigPath, err := findConfigPathUpwards(commonAncestorPath)
	if err != nil {
		return nil, err
	}
	rewriterFactory, err := GetRewriterFactory(luciConfigPath)
	if err != nil {
		return nil, err
	}

	return rewriterFactory, nil
}
