// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package isolate

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/luci/luci-go/client/archiver"
	"github.com/luci/luci-go/common/flag/stringlistflag"
	"github.com/luci/luci-go/common/flag/stringmapflag"
	"github.com/luci/luci-go/common/isolated"
	"github.com/luci/luci-go/common/isolatedclient"
	"github.com/luci/luci-go/common/runtime/tracer"
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

// Tree to be isolated.
type Tree struct {
	Cwd  string
	Opts ArchiveOptions
}

// ArchiveOptions for achiving trees.
type ArchiveOptions struct {
	Isolate         string              `json:"isolate"`
	Isolated        string              `json:"isolated"`
	Blacklist       stringlistflag.Flag `json:"blacklist"`
	PathVariables   stringmapflag.Value `json:"path_variables"`
	ExtraVariables  stringmapflag.Value `json:"extra_variables"`
	ConfigVariables stringmapflag.Value `json:"config_variables"`
}

// Init initializes with non-nil values.
func (a *ArchiveOptions) Init() {
	a.Blacklist = stringlistflag.Flag{}
	a.PathVariables = map[string]string{}
	if IsWindows() {
		a.PathVariables["EXECUTABLE_SUFFIX"] = ".exe"
	} else {
		a.PathVariables["EXECUTABLE_SUFFIX"] = ""
	}
	a.ExtraVariables = map[string]string{}
	a.ConfigVariables = map[string]string{}
}

// PostProcess post-processes the flags to fix any compatibility issue.
func (a *ArchiveOptions) PostProcess(cwd string) {
	// Set default blacklist only if none is set.
	if len(a.Blacklist) == 0 {
		// This cannot be generalized as ".*" as there is known use that require
		// a ".pki" directory to be mapped.
		a.Blacklist = stringlistflag.Flag{
			// Temporary python files.
			"*.pyc",
			// Temporary vim files.
			"*.swp",
			".git",
			".hg",
			".svn",
		}
	}
	if !filepath.IsAbs(a.Isolate) {
		a.Isolate = filepath.Join(cwd, a.Isolate)
	}
	a.Isolate = filepath.Clean(a.Isolate)

	if !filepath.IsAbs(a.Isolated) {
		a.Isolated = filepath.Join(cwd, a.Isolated)
	}
	a.Isolated = filepath.Clean(a.Isolated)

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
			if v, ok := opts.ExtraVariables[varName]; ok {
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

// Archive processes a .isolate, generates a .isolated and archive it.
// Returns a *Item to the .isolated.
func Archive(arch *archiver.Archiver, opts *ArchiveOptions) *archiver.Item {
	displayName := filepath.Base(opts.Isolated)
	defer tracer.Span(arch, strings.SplitN(displayName, ".", 2)[0]+":archive", nil)(nil)
	f, err := archive(arch, opts, displayName)
	if err != nil {
		arch.Cancel(err)
		i := &archiver.Item{DisplayName: displayName}
		i.SetErr(err)
		return i
	}
	return f
}

// ProcessIsolate parses an isolate file, returning the list of dependencies
// (both files and directories), the root directory and the initial Isolated struct.
func ProcessIsolate(opts *ArchiveOptions) ([]string, string, *isolated.Isolated, error) {
	content, err := ioutil.ReadFile(opts.Isolate)
	if err != nil {
		return nil, "", nil, err
	}
	cmd, deps, readOnly, isolateDir, err := LoadIsolateForConfig(filepath.Dir(opts.Isolate), content, opts.ConfigVariables)
	if err != nil {
		return nil, "", nil, err
	}

	// Expand variables in the commands.
	for i := range cmd {
		if cmd[i], err = ReplaceVariables(cmd[i], opts); err != nil {
			return nil, "", nil, err
		}
	}

	// Expand variables in the deps, and convert each path to a cleaned absolute form.
	for i := range deps {
		dep, err := ReplaceVariables(deps[i], opts)
		if err != nil {
			return nil, "", nil, err
		}
		deps[i] = join(isolateDir, dep)
	}

	// Find the root directory of all the files (the root might be above isolateDir).
	rootDir := isolateDir
	for _, dep := range deps {
		// Check if the dep is outside isolateDir.
		base := filepath.Dir(dep)
		for {
			rel, err := filepath.Rel(rootDir, base)
			if err != nil {
				return nil, "", nil, err
			}
			if !strings.HasPrefix(rel, "..") {
				break
			}
			newRootDir := filepath.Dir(rootDir)
			if newRootDir == rootDir {
				return nil, "", nil, errors.New("failed to find root dir")
			}
			rootDir = newRootDir
		}
	}
	if rootDir != isolateDir {
		log.Printf("Root: %s", rootDir)
	}

	// Prepare the .isolated struct.
	isol := &isolated.Isolated{
		Algo:     "sha-1",
		Files:    map[string]isolated.File{},
		ReadOnly: readOnly.ToIsolated(),
		Version:  isolated.IsolatedFormatVersion,
	}
	if len(cmd) != 0 {
		isol.Command = cmd
	}
	if rootDir != isolateDir {
		relPath, err := filepath.Rel(rootDir, isolateDir)
		if err != nil {
			return nil, "", nil, err
		}
		isol.RelativeCwd = relPath
	}
	return deps, rootDir, isol, nil
}

func processing(opts *ArchiveOptions) (filesCount, dirsCount int, deps []string, rootDir string, isol *isolated.Isolated, err error) {
	deps, rootDir, isol, err = ProcessIsolate(opts)
	if err != nil {
		return 0, 0, nil, "", nil, err
	}

	for i := range deps {
		if deps[i][len(deps[i])-1] == os.PathSeparator {
			dirsCount++
		} else {
			filesCount++
		}
	}

	// Processing of the .isolate file ended.
	return filesCount, dirsCount, deps, rootDir, isol, nil
}

// join joins the provided pair of path elements. Unlike filepath.Join, join
// will preserve the trailing slash of the second element, if any.
// TODO(djd): delete this function. Preserving the slash is fragile, and we
// can tell if a path is a dir by stat'ing it (which we need to do anyway).
func join(p1, p2 string) string {
	joined := filepath.Join(p1, p2)
	if p2[len(p2)-1] == os.PathSeparator {
		joined += osPathSeparator // Retain trailing slash.
	}
	return joined
}

func archive(arch *archiver.Archiver, opts *ArchiveOptions, displayName string) (*archiver.Item, error) {
	end := tracer.Span(arch, strings.SplitN(displayName, ".", 2)[0]+":loading", nil)
	filesCount, dirsCount, deps, rootDir, i, err := processing(opts)
	end(tracer.Args{"err": err})
	if err != nil {
		return nil, err
	}
	// Handle each dependency, either a file or a directory..
	fileItems := make([]*archiver.Item, 0, filesCount)
	dirItems := make([]*archiver.Item, 0, dirsCount)
	for _, dep := range deps {
		relPath, err := filepath.Rel(rootDir, dep)
		if err != nil {
			return nil, err
		}
		if dep[len(dep)-1] == os.PathSeparator {
			relPath, err := filepath.Rel(rootDir, dep)
			if err != nil {
				return nil, err
			}
			dirItems = append(dirItems, archiver.PushDirectory(arch, dep, relPath, opts.Blacklist))
		} else {
			// Grab the stats right away.
			info, err := os.Lstat(dep)
			if err != nil {
				return nil, err
			}
			mode := info.Mode()
			if mode&os.ModeSymlink == os.ModeSymlink {
				l, err := os.Readlink(dep)
				if err != nil {
					// Kill the process: there's no reason to continue if a file is
					// unavailable.
					log.Fatalf("Unable to stat %q: %v", dep, err)
				}
				i.Files[relPath] = isolated.SymLink(l)
			} else {
				i.Files[relPath] = isolated.BasicFile("", int(mode.Perm()), info.Size())
				fileItems = append(fileItems, arch.PushFile(relPath, dep, -info.Size()))
			}
		}
	}

	for _, item := range fileItems {
		item.WaitForHashed()
		if err = item.Error(); err != nil {
			return nil, err
		}
		f := i.Files[item.DisplayName]
		f.Digest = item.Digest()
		i.Files[item.DisplayName] = f
	}
	// Avoid duplicated entries in includes.
	// TODO(tandrii): add test to reproduce the problem.
	includesSet := map[isolated.HexDigest]bool{}
	for _, item := range dirItems {
		item.WaitForHashed()
		if err = item.Error(); err != nil {
			return nil, err
		}
		includesSet[item.Digest()] = true
	}
	for digest := range includesSet {
		i.Includes = append(i.Includes, digest)
	}
	// Make the includes list deterministic.
	sort.Sort(i.Includes)

	raw := &bytes.Buffer{}
	if err = json.NewEncoder(raw).Encode(i); err != nil {
		return nil, err
	}

	if err := ioutil.WriteFile(opts.Isolated, raw.Bytes(), 0644); err != nil {
		return nil, err
	}
	return arch.Push(displayName, isolatedclient.NewBytesSource(raw.Bytes()), 0), nil
}
