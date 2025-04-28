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

package builder

import (
	"io"
	"path/filepath"
	"regexp"
	"sort"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
	"go.chromium.org/luci/common/errors"
	"github.com/goccy/go-yaml"
)

// PackageDef defines how exactly to build a package.
//
// It specified  what files to put into it, how to name them, how to name
// the package itself, etc. It is loaded from *.yaml file.
type PackageDef struct {
	// Package defines a name of the package.
	Package string

	// Root defines where to search for files. It may either be an absolute path,
	// or it may be a path relative to the package file itself. If omitted, it
	// defaults to "." (i.e., the same directory as the package file)
	Root string

	// InstallMode defines how to deploy the package file: "copy" or "symlink".
	InstallMode pkg.InstallMode `yaml:"install_mode"`

	// PreserveModTime instructs CIPD to preserve the mtime of the files.
	PreserveModTime bool `yaml:"preserve_mtime"`

	// PreserveWritable instructs CIPD to preserve the user-writable permission
	// mode on the files.
	PreserveWritable bool `yaml:"preserve_writable"`

	// Data describes what is deployed with the package.
	Data []PackageChunkDef
}

// PackageChunkDef represents one entry in 'data' section of package definition.
//
// It is either a single file, or a recursively scanned directory (with optional
// list of regexps for files to skip).
type PackageChunkDef struct {
	// Dir is a directory to add to the package (recursively).
	Dir string

	// File is a single file to add to the package.
	File string

	// VersionFile defines where to drop JSON file with package version.
	VersionFile string `yaml:"version_file"`

	// Exclude is a list of regexp patterns to exclude when scanning a directory.
	Exclude []string
}

// LoadPackageDef loads package definition from a YAML source code.
//
// It substitutes %{...} strings in the definition with corresponding values
// from 'vars' map.
func LoadPackageDef(r io.Reader, vars map[string]string) (PackageDef, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return PackageDef{}, errors.Annotate(err, "reading package definition file").Tag(cipderr.IO).Err()
	}

	out := PackageDef{}
	if err = yaml.Unmarshal(data, &out); err != nil {
		return PackageDef{}, errors.Annotate(err, "bad package definition file").Tag(cipderr.BadArgument).Err()
	}

	// Substitute variables in all strings.
	for _, str := range out.strings() {
		*str, err = subVars(*str, vars)
		if err != nil {
			return PackageDef{}, err
		}
	}

	// Validate global package properties.
	if err = common.ValidatePackageName(out.Package); err != nil {
		return PackageDef{}, err
	}
	if err = pkg.ValidateInstallMode(out.InstallMode); err != nil {
		return PackageDef{}, err
	}

	versionFile := ""
	for i, chunk := range out.Data {
		// Make sure 'dir' and 'file' etc. aren't used together.
		has := make([]string, 0, 3)
		if chunk.File != "" {
			has = append(has, "file")
		}
		if chunk.VersionFile != "" {
			has = append(has, "version_file")
		}
		if chunk.Dir != "" {
			has = append(has, "dir")
		}
		if len(has) == 0 {
			return out, errors.Reason("files entry #%d needs 'file', 'dir' or 'version_file' key", i).Tag(cipderr.BadArgument).Err()
		}
		if len(has) != 1 {
			return out, errors.Reason("files entry #%d should have only one key, got %q", i, has).Tag(cipderr.BadArgument).Err()
		}
		//'version_file' can appear only once, it must be a clean relative path.
		if chunk.VersionFile != "" {
			if versionFile != "" {
				return out, errors.Reason("'version_file' entry can be used only once").Tag(cipderr.BadArgument).Err()
			}
			versionFile = chunk.VersionFile
			if !fs.IsCleanSlashPath(versionFile) {
				return out, errors.Reason("'version_file' must be a path relative to the package root: %s", versionFile).Tag(cipderr.BadArgument).Err()
			}
		}
	}

	// Default 'root' to a directory with the package def file.
	if out.Root == "" {
		out.Root = "."
	}
	return out, nil
}

// FindFiles scans files system and returns files to be added to the package.
//
// It uses a path to package definition file directory ('cwd' argument) to find
// a root of the package.
func (def *PackageDef) FindFiles(cwd string) ([]fs.File, error) {
	// Root of the package is defined relative to package def YAML file.
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return nil, errors.Annotate(err, "bad input directory").Tag(cipderr.BadArgument).Err()
	}
	root := filepath.Clean(def.Root)
	if !filepath.IsAbs(root) {
		root = filepath.Join(absCwd, root)
	}

	// Helper to get absolute path to a file given path relative to root.
	makeAbs := func(p string) string {
		return filepath.Join(root, filepath.FromSlash(p))
	}

	// Used to skip duplicates.
	seen := map[string]fs.File{}
	add := func(f fs.File) {
		if seen[f.Name()] == nil {
			seen[f.Name()] = f
		}
	}

	scanOpts := fs.ScanOptions{
		PreserveModTime:  def.PreserveModTime,
		PreserveWritable: def.PreserveWritable,
	}

	for _, chunk := range def.Data {
		// Handled elsewhere.
		if chunk.VersionFile != "" {
			continue
		}

		// Individual file.
		if chunk.File != "" {
			file, err := fs.WrapFile(makeAbs(chunk.File), root, nil, scanOpts)
			if err != nil {
				return nil, err
			}
			add(file)
			continue
		}

		// A subdirectory to scan (with filtering).
		if chunk.Dir != "" {
			// Absolute path to directory to scan.
			startDir := makeAbs(chunk.Dir)
			// Exclude files as specified in 'exclude' section.
			exclude, err := makeExclusionFilter(chunk.Exclude)
			if err != nil {
				return nil, errors.Annotate(err, "dir %q", chunk.Dir).Err()
			}
			// Run the scan.
			files, err := fs.ScanFileSystem(startDir, root, exclude, scanOpts)
			if err != nil {
				return nil, errors.Annotate(err, "dir %q", chunk.Dir).Err()
			}
			for _, f := range files {
				add(f)
			}
			continue
		}

		// LoadPackageDef does validation, so this should not happen.
		return nil, errors.Reason("unexpected definition: %v", chunk).Tag(cipderr.BadArgument).Err()
	}

	// Sort by Name().
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)

	// Final sorted array of fs.File.
	out := make([]fs.File, 0, len(names))
	for _, n := range names {
		out = append(out, seen[n])
	}
	return out, nil
}

// VersionFile defines where to drop JSON file with package version.
func (def *PackageDef) VersionFile() string {
	// It is already validated by LoadPackageDef, so just return it.
	for _, chunk := range def.Data {
		if chunk.VersionFile != "" {
			return chunk.VersionFile
		}
	}
	return ""
}

// makeExclusionFilter produces a predicate that checks a relative file path
// against a list of regexps and returns true to exclude it.
//
// Note that regexps are defined against slash-separated relative paths (to make
// the package definition YAML platform-agnostic).
func makeExclusionFilter(patterns []string) (fs.ScanFilter, error) {
	if len(patterns) == 0 {
		return nil, nil
	}

	// Compile regular expressions. Note that we want to verify that each
	// individual pattern is a valid regexp. For that reason we don't just
	// concatenate them in a single uber-regexp and compile it afterwards.
	exps := []*regexp.Regexp{}
	for _, expr := range patterns {
		if expr == "" {
			continue
		}
		if expr[0] != '^' {
			expr = "^" + expr
		}
		if expr[len(expr)-1] != '$' {
			expr = expr + "$"
		}
		re, err := regexp.Compile(expr)
		if err != nil {
			return nil, errors.Annotate(err, "bad exclusion pattern").Tag(cipderr.BadArgument).Err()
		}
		exps = append(exps, re)
	}

	return func(rel string) bool {
		rel = filepath.ToSlash(rel)
		for _, exp := range exps {
			if exp.MatchString(rel) {
				return true
			}
		}
		return false
	}, nil
}

////////////////////////////////////////////////////////////////////////////////
// Variable substitution.

var subVarsRe = regexp.MustCompile(`\$\{[^\}]+\}`)

// strings return array of pointers to all strings in PackageDef that can
// contain ${var} variables.
func (def *PackageDef) strings() []*string {
	out := []*string{
		&def.Package,
		&def.Root,
	}
	// Important to use index here, to get a point to a real object, not its copy.
	for i := range def.Data {
		out = append(out, def.Data[i].strings()...)
	}
	return out
}

// strings return array of pointers to all strings in PackageChunkDef that can
// contain ${var} variables.
func (def *PackageChunkDef) strings() []*string {
	out := []*string{
		&def.Dir,
		&def.File,
		&def.VersionFile,
	}
	for i := range def.Exclude {
		out = append(out, &def.Exclude[i])
	}
	return out
}

// subVars replaces "${key}" in strings with values from 'vars' map. Returns
// error if some keys weren't found in 'vars' map.
func subVars(s string, vars map[string]string) (string, error) {
	var badKeys []string
	res := subVarsRe.ReplaceAllStringFunc(s, func(match string) string {
		// Strip '${' and '}'.
		key := match[2 : len(match)-1]
		val, ok := vars[key]
		if !ok {
			badKeys = append(badKeys, key)
			return match
		}
		return val
	})
	if len(badKeys) != 0 {
		return res, errors.Reason("values for some variables are not provided: %v", badKeys).Tag(cipderr.BadArgument).Err()
	}
	return res, nil
}
