// Copyright 2015 The LUCI Authors.
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

package isolate

import (
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/stringmapflag"
)

// IsolatedGenJSONVersion is used in the batcharchive json format.
//
// TODO(tandrii): Migrate to batch_archive.go.
const IsolatedGenJSONVersion = 1

// ValidVariable is the regexp of valid isolate variable name.
const ValidVariable = "[A-Za-z_][A-Za-z_0-9]*"

var validVariableMatcher = regexp.MustCompile(ValidVariable)
var variableSubstitutionMatcher = regexp.MustCompile("<\\(" + ValidVariable + "\\)")

// IsValidVariable returns true if the variable is a valid symbol name.
func IsValidVariable(variable string) bool {
	return validVariableMatcher.MatchString(variable)
}

// ArchiveOptions for archiving trees.
type ArchiveOptions struct {
	Isolate             string              `json:"isolate"`
	IgnoredPathFilterRe string              `json:"ignored_path_filter_re"`
	PathVariables       stringmapflag.Value `json:"path_variables"`
	ConfigVariables     stringmapflag.Value `json:"config_variables"`
	AllowMissingFileDir bool                `json:"allow_missing_file_dir"`
}

// Init initializes with non-nil values.
func (a *ArchiveOptions) Init() {
	a.PathVariables = map[string]string{}
	if runtime.GOOS == "windows" {
		a.PathVariables["EXECUTABLE_SUFFIX"] = ".exe"
	} else {
		a.PathVariables["EXECUTABLE_SUFFIX"] = ""
	}
	a.ConfigVariables = map[string]string{}
}

func genExtensionsRegex(exts ...string) string {
	if len(exts) == 0 {
		return ""
	}
	res := make([]string, len(exts))
	for i, e := range exts {
		res[i] = `(\.` + e + `)`
	}
	return "((" + strings.Join(res, "|") + ")$)"
}

func genDirectoriesRegex(dirs ...string) string {
	if len(dirs) == 0 {
		return ""
	}
	res := make([]string, len(dirs))
	for i, d := range dirs {
		res[i] = "(" + d + ")"
	}
	// #Backslashes: https://stackoverflow.com/a/4025505/12003165
	return `((^|[\\/])(` + strings.Join(res, "|") + `)([\\/]|$))`
}

// PostProcess post-processes the flags to fix any compatibility issue.
func (a *ArchiveOptions) PostProcess(cwd string) {
	if a.IgnoredPathFilterRe == "" {
		// Set default ignored paths regexp
		// .swp are vim files
		a.IgnoredPathFilterRe = genExtensionsRegex("swp") + "|" + genDirectoriesRegex(`\.git`, `\.hg`, `\.svn`, "__pycache__", `\.wptcache`)
	}
	if !filepath.IsAbs(a.Isolate) {
		a.Isolate = filepath.Join(cwd, a.Isolate)
	}
	a.Isolate = filepath.Clean(a.Isolate)

	for k, v := range a.PathVariables {
		// This is due to a Windows + GYP specific issue, where double-quoted paths
		// would get mangled in a way that cannot be resolved unless a space is
		// injected.
		a.PathVariables[k] = strings.TrimSpace(v)
	}
}

// ReplaceVariables replaces any occurrences of '<(FOO)' in 'str' with the
// corresponding variable from 'opts'.
//
// If any substitution refers to a variable that is missing, the returned error will
// refer to the first such variable. In the case of errors, the returned string will
// still contain a valid result for any non-missing substitutions.
func ReplaceVariables(str string, opts *ArchiveOptions) (string, error) {
	var err error
	subst := variableSubstitutionMatcher.ReplaceAllStringFunc(str,
		func(match string) string {
			varName := match[2 : len(match)-1]
			if v, ok := opts.PathVariables[varName]; ok {
				return v
			}
			if v, ok := opts.ConfigVariables[varName]; ok {
				return v
			}
			if err == nil {
				err = errors.New("no value for variable '" + varName + "'")
			}
			return match
		})
	return subst, err
}

func processDependencies(deps []string, isolateDir string, opts *ArchiveOptions) ([]string, string, error) {
	// Expand variables in the deps, and convert each path to an absolute form.
	for i := range deps {
		dep, err := ReplaceVariables(deps[i], opts)
		if err != nil {
			return nil, "", err
		}
		deps[i] = filepath.Join(isolateDir, dep)
	}

	// Find the root directory of all the files (the root might be above isolateDir).
	rootDir := isolateDir
	resultDeps := make([]string, 0, len(deps))
	for _, dep := range deps {
		// Check if the dep is outside isolateDir.
		info, err := os.Stat(dep)
		if err != nil {
			if !opts.AllowMissingFileDir {
				return nil, "", errors.Fmt("failed to call Stat for %s: %w", dep, err)
			}
			log.Printf("Ignore missing dep: %s, err: %v", dep, err)
			continue
		}
		base := filepath.Dir(dep)
		if info.IsDir() {
			base = dep
			// Downstream expects the dependency of a directory to always end
			// with '/', but filepath.Join() removes that, so we add it back.
			dep += osPathSeparator
		}
		resultDeps = append(resultDeps, dep)
		for {
			rel, err := filepath.Rel(rootDir, base)
			if err != nil {
				return nil, "", errors.Fmt("failed to call filepath.Rel(%s, %s): %w", rootDir, base, err)
			}
			if !strings.HasPrefix(rel, "..") {
				break
			}
			newRootDir := filepath.Dir(rootDir)
			if newRootDir == rootDir {
				return nil, "", errors.New("failed to find root dir")
			}
			rootDir = newRootDir
		}
	}
	if rootDir != isolateDir {
		log.Printf("Root: %s", rootDir)
	}
	return resultDeps, rootDir, nil
}

// ProcessIsolate parses an isolate file, returning the list of dependencies
func ProcessIsolate(opts *ArchiveOptions) ([]string, string, error) {
	content, err := os.ReadFile(opts.Isolate)
	if err != nil {
		return nil, "", errors.Fmt("failed to read file: %s: %w", opts.Isolate, err)
	}
	deps, isolateDir, err := LoadIsolateForConfig(filepath.Dir(opts.Isolate), content, opts.ConfigVariables)
	if err != nil {
		return nil, "", err
	}

	deps, rootDir, err := processDependencies(deps, isolateDir, opts)
	if err != nil {
		return nil, "", err
	}
	return deps, rootDir, nil
}
