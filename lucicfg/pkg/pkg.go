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

// Package pkg implements lucicfg packages functionality.
package pkg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/starlark/interpreter"

	"go.chromium.org/luci/lucicfg/buildifier"
	"go.chromium.org/luci/lucicfg/internal"
)

// PackageScript is a name of the script with the package definition.
const PackageScript = "PACKAGE.star"

// LegacyPackageNamePlaceholder is used as package name of legacy packages.
const LegacyPackageNamePlaceholder = "@legacy-unknown"

// Entry is a main package plus an entry point executable file in it.
//
// This package and all its dependencies are fully resolved and prefetched and
// ready for execution.
//
// It can be loaded either from files on disk or from some abstracted storage.
// For that reason it doesn't include absolute paths.
type Entry struct {
	// Main contains the code of the main package itself.
	Main interpreter.Loader
	// Deps contains the code of all dependencies at resolved versions.
	Deps map[string]interpreter.Loader
	// Package is the name of the package being executed from its PACKAGE.star.
	Package string
	// Path is a slash-separate path from the repo root to the package root.
	Path string
	// Script is a slash-separated path to the script within the package to run.
	Script string
	// LucicfgVersionConstraints is the requirements for the lucicfg version.
	LucicfgVersionConstraints []LucicfgVersionConstraint
	// Local is set only if this entry was loaded from the local disk.
	Local *Local
}

// LucicfgVersionConstraint puts a constraint on the current lucicfg version.
type LucicfgVersionConstraint struct {
	Min     LucicfgVersion // minimal required version
	Package string         // name of the package that required it
	Main    bool           // true if this is the main package (i.e. contains the entry point)
}

// EntryOnDisk loads the entry point based on a Starlark file on disk.
//
// It searches for the closest (up the tree) PACKAGE.star file to find the
// package the entry point the script belongs to. It then loads this package and
// all its dependencies (based on PACKAGE.lock file).
//
// If there's no PACKAGE.star, synthesizes a minimal package using the given
// file's directory as the package root. This is useful during the migration to
// packages.
//
// The given RepoManager will be used to load all remote (non-local) transitive
// dependencies. It is used only if this package has a remote (perhaps
// transitive) dependency. If nil, remote dependencies won't be supported.
//
// Local dependencies are always supported.
//
// Returns the loaded entry point with Local populated.
//
// The returned error may be backtracable.
func EntryOnDisk(ctx context.Context, path string, remotes RepoManager) (*Entry, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, errors.Annotate(err, "taking absolute path of %q", path).Err()
	}

	var def *Definition

	// Find PACKAGE.star indicating the root of the package.
	var found bool
	scriptDir, main := filepath.Split(abs)
	root, found, err := findRoot(scriptDir, PackageScript, nil)
	if err != nil {
		return nil, errors.Annotate(err, "searching for %s", PackageScript).Err()
	}
	if found {
		main, err = filepath.Rel(root, abs)
		if err != nil {
			return nil, errors.Annotate(err, "getting relative path from %s to %s", root, abs).Err()
		}
		pkgScript := filepath.Join(root, PackageScript)
		pkgBody, err := os.ReadFile(pkgScript)
		if err != nil {
			return nil, errors.Annotate(err, "reading %s", PackageScript).Err()
		}
		def, err = LoadDefinition(ctx, pkgBody, &diskLoaderValidator{root: root})
		if err != nil {
			return nil, errors.Annotate(err, "loading %s", cwdRel(pkgScript)).Err()
		}
	} else {
		// Fallback to the pre-PACKAGE.star behavior where entry point scripts were
		// assumed to be at the root of the main package.
		root = scriptDir
	}

	// Drop trailing "/" from the root path.
	root = filepath.Clean(root)

	// Calculate path from the repo root to the package root. This is needed by
	// some lucicfg introspection functionality (these paths end up in
	// project.cfg).
	repoRoot, _, err := findRoot(root, "", nil)
	if err != nil {
		return nil, errors.Annotate(err, "could not determine the repository or volume root of %q", abs).Err()
	}
	rel, err := filepath.Rel(repoRoot, root)
	if err != nil {
		return nil, errors.Annotate(err, "calculating path of %q relative to %q", root, repoRoot).Err()
	}
	script := filepath.ToSlash(main)

	// If no PACKAGE.star, return minimal package for compatibility with old code.
	if def == nil {
		code := interpreter.FileSystemLoader(root)
		return &Entry{
			Main:    code,
			Package: LegacyPackageNamePlaceholder,
			Path:    filepath.ToSlash(rel),
			Script:  script,
			Local: &Local{
				Code:       code,
				DiskPath:   root,
				Definition: legacyDefinition(),
				Formatter:  legacyFormatter(root),
			},
		}, nil
	}

	// Verify the entry point is known.
	if !internal.GetTestingTweaks(ctx).SkipEntrypointCheck {
		if !slices.Contains(def.Entrypoints, script) {
			return nil, errors.Reason(
				"%s is not declared as a pkg.entrypoint(...) in %s and "+
					"thus cannot be executed. Available entrypoints: %v",
				script, PackageScript, def.Entrypoints).Err()
		}
	}

	if remotes == nil {
		remotes = &ErroringRepoManager{
			Error: errors.Reason("remote dependencies are not supported in this context").Err(),
		}
	}

	// Construct a trivial Repo implementation that just fetches files directly
	// from disk. Since we don't really know what this local repo represents (it
	// may not even exists as a remote repo, e.g. for new packages being
	// developed), use a special magical RepoKey for it. See also a comment for
	// PinnedVersion for more details on special status of root-local packages.
	localRepo := &LocalDiskRepo{
		Root: repoRoot,
		Key:  RepoKey{Root: true},
	}

	// Load transitive closure of all dependencies.
	repoPath := filepath.ToSlash(rel)
	deps, err := discoverDeps(ctx, &DepContext{
		Package: def.Name,
		Version: PinnedVersion,
		Repo:    localRepo,
		Path:    repoPath,
		RepoManager: &PreconfiguredRepoManager{
			Repos: []Repo{localRepo},
			Other: remotes,
		},
		Known: def,
	})
	if err != nil {
		return nil, err
	}

	// Convert them to a form suitable for Entry.
	depsLoaders := make(map[string]interpreter.Loader, len(deps))
	constraints := []LucicfgVersionConstraint{
		{
			Min:     def.MinLucicfgVersion,
			Package: def.Name,
			Main:    true,
		},
	}
	for _, dep := range deps {
		name, ok := strings.CutPrefix(dep.Package, "@")
		if !ok {
			panic(fmt.Sprintf("unexpected package name %q", dep.Package))
		}
		depsLoaders[name] = dep.Code
		constraints = append(constraints, LucicfgVersionConstraint{
			Min:     dep.Min,
			Package: dep.Package,
		})
	}

	code, err := diskPackageLoader(root, def.Resources)
	if err != nil {
		return nil, err
	}
	return &Entry{
		Main:                      code,
		Deps:                      depsLoaders,
		Package:                   def.Name,
		Path:                      repoPath,
		Script:                    script,
		LucicfgVersionConstraints: constraints,
		Local: &Local{
			Code:       code,
			DiskPath:   root,
			Definition: def,
			Formatter:  legacyCompatibleFormatter(root, def.FmtRules),
		},
	}, nil
}

// Local represents some local package without any of its dependencies.
//
// Information here is used for purely local operations (like formatting and
// linting) that don't need remote dependencies.
type Local struct {
	// Code contains the code of the package.
	Code interpreter.Loader
	// DiskPath is an absolute path to the package directory on disk.
	DiskPath string
	// Definition is the full package definition.
	Definition *Definition
	// Formatter defines how to format files in the package.
	Formatter buildifier.FormatterPolicy
}

// PackageOnDisk loads a local package from a directory on disk.
//
// This is similar to EntryOnDisk, except it doesn't fetch (or validate) any
// dependencies and doesn't require an entry point.
//
// This is used for local "fmt" and "lint" checks that don't care about external
// dependencies and do not actually run any Starlark code (but they still need
// options from PACKAGE.star and an interpreter.Loader to read file bodies).
//
// The given directory should contain PACKAGE.star. If it doesn't, this
// directory will be treated as a legacy package directory for compatibility
// with pre-PACKAGE.star code (some minimal package definition will be
// synthesized for it).
//
// The returned error may be backtracable.
func PackageOnDisk(ctx context.Context, dir string) (*Local, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, errors.Annotate(err, "taking absolute path of %q", dir).Err()
	}

	pkgScript := filepath.Join(abs, PackageScript)
	switch body, err := os.ReadFile(pkgScript); {
	case err == nil:
		def, err := LoadDefinition(ctx, body, &diskLoaderValidator{root: abs})
		if err != nil {
			return nil, errors.Annotate(err, "loading %s", cwdRel(pkgScript)).Err()
		}
		code, err := diskPackageLoader(abs, def.Resources)
		if err != nil {
			return nil, err
		}
		return &Local{
			Code:       code,
			DiskPath:   abs,
			Definition: def,
			Formatter:  legacyCompatibleFormatter(abs, def.FmtRules),
		}, nil

	case errors.Is(err, os.ErrNotExist):
		// Legacy mode.
		return &Local{
			Code:       interpreter.FileSystemLoader(abs),
			DiskPath:   abs,
			Definition: legacyDefinition(),
			Formatter:  legacyFormatter(abs),
		}, nil

	default:
		return nil, errors.Annotate(err, "reading %s", PackageScript).Err()
	}
}

// cwdRel converts the given path to be relative to the current working
// directory, if possible.
//
// Exclusively for error messages. Not very rigorous and gives up on errors.
func cwdRel(abs string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return abs
	}
	rel, err := filepath.Rel(cwd, abs)
	if err != nil {
		return abs
	}
	return rel
}

func legacyDefinition() *Definition {
	return &Definition{
		Name:      LegacyPackageNamePlaceholder,
		Resources: []string{"**/*"},
	}
}

type diskLoaderValidator struct {
	root string
}

func (d *diskLoaderValidator) ValidateEntrypoint(ctx context.Context, entrypoint string) error {
	switch info, err := os.Stat(filepath.Join(d.root, filepath.FromSlash(entrypoint))); {
	case err == nil:
		if !info.Mode().IsRegular() {
			return errors.Reason("not a regular file").Err()
		}
		return nil
	case errors.Is(err, os.ErrNotExist):
		return errors.Reason("no such file in the package").Err()
	default:
		return err
	}
}
