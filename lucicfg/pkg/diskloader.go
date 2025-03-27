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
	"context"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/starlark/interpreter"

	"go.chromium.org/luci/lucicfg/fileset"
)

// diskPackageLoader returns a loader that can load files belonging to the
// package at the given root path (which contains PACKAGE.star) on the local
// disk.
//
// It is aware of possible nested packages (i.e. subdirectories of the root that
// have their own PACKAGE.star). Files inside such nested packages are
// invisible.
func diskPackageLoader(root, pkgName string, resources *fileset.Set, cache *statCache) (interpreter.Loader, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	return GenericLoader(GenericLoaderParams{
		Package:   pkgName,
		Resources: resources,

		IsVisible: func(ctx context.Context, dir string) (bool, error) {
			abs := filepath.Join(root, filepath.FromSlash(dir))
			switch pkgRoot, found, err := findRoot(abs, PackageScript, root, cache); {
			case err != nil:
				return false, err
			case !found:
				// This should not normally be happening, since we know root is a
				// package root. It can theoretically happen if it was deleted on disk
				// after we started running lucicfg.
				return false, errors.Reason("path %q is not inside of any package", dir).Err()
			case pkgRoot == root:
				return true, nil
			default:
				return false, nil
			}
		},

		Fetch: func(ctx context.Context, path string) ([]byte, error) {
			body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
			if os.IsNotExist(err) {
				return nil, interpreter.ErrNoModule
			}
			return body, err
		},
	}), nil
}

// diskLoaderValidator implements LoaderValidator using files on disk.
type diskLoaderValidator struct {
	pkgRoot   string     // absolute path to the package root
	repoRoot  string     // absolute path to the package repository root
	statCache *statCache // to dedup os.Stat calls
}

func (d *diskLoaderValidator) ValidateEntrypoint(ctx context.Context, entrypoint string) error {
	switch info, err := os.Stat(filepath.Join(d.pkgRoot, filepath.FromSlash(entrypoint))); {
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

func (d *diskLoaderValidator) ValidateDepDecl(ctx context.Context, dep *DepDecl) error {
	if dep.LocalPath == "" {
		return nil // can only verify local dependencies here
	}

	depAbs := filepath.Join(d.pkgRoot, filepath.FromSlash(dep.LocalPath))

	// Verify it is not outside of the d.repoRoot.
	switch depRepoRel, err := filepath.Rel(d.repoRoot, depAbs); {
	case err != nil:
		return errors.Annotate(err, "unexpected error checking the local dependency").Err()
	case depRepoRel == ".." || strings.HasPrefix(depRepoRel, ".."+string(filepath.Separator)):
		return errors.Reason("a local dependency must not point outside of the repository it is declared in").Err()
	}

	// Verify it is not in a submodule.
	switch depRepoRoot, _, err := findRoot(depAbs, "", d.repoRoot, d.statCache); {
	case err != nil:
		return errors.Annotate(err, "unexpected error checking the local dependency").Err()
	case depRepoRoot != d.repoRoot:
		return errors.Reason("a local dependency should not reside in a git submodule").Err()
	}
	return nil
}
