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
	"os"
	"path/filepath"
	"slices"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/starlark/interpreter"

	"go.chromium.org/luci/lucicfg/internal"
)

// PackageScript is a name of the script with the package definition.
const PackageScript = "PACKAGE.star"

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
	// Path is a slash-separate path from the repo root to the package root.
	Path string
	// Script is a slash-separated path to the script within the package to run.
	Script string
	// LucicfgVersionConstraints is the requirements for the lucicfg version.
	LucicfgVersionConstraints []LucicfgVersionConstraint
	// LintChecks are enabled lint checks.
	LintChecks []string
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
// Returns the loaded entry point and absolute path to the main package root
// directory on disk.
func EntryOnDisk(ctx context.Context, path string) (entry *Entry, root string, err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", errors.Annotate(err, "taking absolute path of %q", path).Err()
	}

	var def *Definition

	// Find PACKAGE.star indicating the root of the package.
	var found bool
	scriptDir, main := filepath.Split(abs)
	root, found, err = findRoot(scriptDir, PackageScript)
	if err != nil {
		return nil, "", errors.Annotate(err, "searching for %s", PackageScript).Err()
	}
	if found {
		main, err = filepath.Rel(root, abs)
		if err != nil {
			return nil, "", errors.Annotate(err, "getting relative path from %s to %s", root, abs).Err()
		}
		pkgBody, err := os.ReadFile(filepath.Join(root, PackageScript))
		if err != nil {
			return nil, "", errors.Annotate(err, "reading %s", PackageScript).Err()
		}
		def, err = LoadDefinition(ctx, pkgBody, &diskLoaderValidator{root: root})
		if err != nil {
			return nil, "", errors.Annotate(err, "loading package containing %s", filepath.ToSlash(main)).Err()
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
	repoRoot, _, err := findRoot(root, "")
	if err != nil {
		return nil, "", errors.Annotate(err, "could not determine the repository or volume root of %q", abs).Err()
	}
	rel, err := filepath.Rel(repoRoot, root)
	if err != nil {
		return nil, "", errors.Annotate(err, "calculating path of %q relative to %q", root, repoRoot).Err()
	}
	script := filepath.ToSlash(main)

	// If no PACKAGE.star, return minimal package for compatibility with old code.
	if def == nil {
		return &Entry{
			Main:   interpreter.FileSystemLoader(root),
			Path:   filepath.ToSlash(rel),
			Script: script,
		}, root, nil
	}

	// Verify the entry point is known.
	if !internal.GetTestingTweaks(ctx).SkipEntrypointCheck {
		if !slices.Contains(def.Entrypoints, script) {
			return nil, "", errors.Reason(
				"%s is not declared as a pkg.entrypoint(...) in %s and "+
					"thus cannot be executed. Available entrypoints: %v",
				script, PackageScript, def.Entrypoints).Err()
		}
	}

	return &Entry{
		Main:   interpreter.FileSystemLoader(root),
		Path:   filepath.ToSlash(rel),
		Script: script,
		LucicfgVersionConstraints: []LucicfgVersionConstraint{
			{
				Min:     def.MinLucicfgVersion,
				Package: def.Name,
				Main:    true,
			},
		},
		LintChecks: def.LintChecks,
	}, root, nil
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
