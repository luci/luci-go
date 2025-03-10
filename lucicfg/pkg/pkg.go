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
	"path/filepath"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/starlark/interpreter"
)

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
}

// EntryOnDisk loads the entry point based on a Starlark file on disk.
//
// It searches for the closest (up the tree) PACKAGE.star file to find the
// package the entry point script belongs to. It then loads this package and all
// its dependencies (based on PACKAGE.lock file).
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

	// TODO(vadimsh): Find PACKAGE.star etc. For now use the previous behavior
	// (it will became a fallback behavior when PACKAGE.star is missing).

	root, main := filepath.Split(abs)

	// Calculate path from the repo root to the package root. This is needed by
	// some lucicfg introspection functionality (these paths end up in
	// project.cfg).
	repoRoot, err := findRoot(root)
	if err != nil {
		return nil, "", errors.Annotate(err, "could not determine the repository or volume root of %q", abs).Err()
	}
	rel, err := filepath.Rel(repoRoot, root)
	if err != nil {
		return nil, "", errors.Annotate(err, "calculating path of %q relative to %q", root, repoRoot).Err()
	}

	return &Entry{
		Main:   interpreter.FileSystemLoader(root),
		Path:   filepath.ToSlash(rel),
		Script: main,
	}, root, nil
}
