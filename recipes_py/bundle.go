// Copyright 2024 The LUCI Authors.
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

package recipespy

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/filesystem"
)

// Bundle creates a hermetically runnable recipe bundle.
//
// This is done by packaging the repo and all dependency repos into a folder
// and then generating an entrypoint script with `-O` override flags to this
// folder.
// Optionally provides dep repos with local override paths to pick up the repo
// from local override instead of repos checked out to .recipe_deps directory
// during the recipe bootstrap process.
//
// The general principle is that the input to bundle is:
//   - a fully bootstrapped recipe repo. All dependency repos specified in
//     recipes.cfg are checked out to .recipe_deps directory under the recipe
//     root dir.
//   - files tagged with the `recipes` gitattribute value (see
//     `git help gitattributes`).
//
// and the output is a runnable folder at `dest` for the named repo.
//
// # Included files
//
// By default, bundle will include all recipes/ and recipe_modules/ files in
// your repo, plus the `recipes.cfg` file, and excluding all json expectation
// files. Recipe bundle also uses the standard `gitattributes` mechanism for
// tagging files within the repo, and will also include these files when
// generating the bundle. In particular, it looks for files tagged with the
// string `recipes`. As an example, you could put this in a `.gitattributes`
// file in your repo:
//
//	*.py       recipes
//	*_test.py -recipes
//
// That would include all .py files, but exclude all _test.py files. See the
// page `git help gitattributes` for more information on how gitattributes work.
//
// The recipe repo to bundle may or may not be a git repo. There is a slight
// difference when bundling a recipe repo that is a git repo that the bundling
// process leverages the git index, so any untracked file will NOT be in the
// final bundle.
func Bundle(ctx context.Context, repoPath string, dest string, overrides map[string]string) error {
	switch {
	case repoPath == "":
		return errors.New("recipes_py.Bundle: repoPath is required")
	case dest == "":
		return errors.New("recipes_py.Bundle: destination is required")
	}

	mainRepo, err := RepoFromPath(repoPath)
	if err != nil {
		return err
	}
	if err := mainRepo.ensureDeps(ctx); err != nil {
		return err
	}
	depRepos, err := calculateDepRepos(mainRepo, overrides)
	if err != nil {
		return err
	}

	if dest, err = prepareDestDir(ctx, dest); err != nil {
		return err
	}
	for _, repo := range append([]*Repo{mainRepo}, depRepos...) {
		if err := exportRepo(ctx, repo, dest); err != nil {
			return err
		}
	}
	if err := exportProtos(mainRepo, dest); err != nil {
		return nil
	}
	if err := prepareScripts(mainRepo, depRepos, dest); err != nil {
		return err
	}
	return nil
}

// calculateDepRepos creates the dep Repos from .recipe_deps directory or
// the overrides.
//
// Returns are sorted by the repo name.
// Returns error if the provided overridden dep is not the dep of the main
// repo.
func calculateDepRepos(main *Repo, overrides map[string]string) ([]*Repo, error) {
	unusedOverrides := stringset.New(len(overrides))
	for dep := range overrides {
		unusedOverrides.Add(dep)
	}
	depRepos := make([]*Repo, 0, len(main.Spec.GetDeps()))
	for dep := range main.Spec.GetDeps() {
		depRepoPath, overridden := overrides[dep]
		if !overridden {
			depRepoPath = filepath.Join(main.RecipeDepsPath(), dep)
		} else {
			unusedOverrides.Del(dep)
		}
		repo, err := RepoFromPath(depRepoPath)
		if err != nil {
			return nil, err
		}
		depRepos = append(depRepos, repo)
	}
	if unusedOverrides.Len() > 0 {
		return nil, fmt.Errorf("overrides %s provided but not used", strings.Join(unusedOverrides.ToSortedSlice(), ", "))
	}
	slices.SortFunc(depRepos, func(left, right *Repo) int {
		return strings.Compare(left.Name(), right.Name())
	})
	return depRepos, nil
}

// prepareDestDir creates the destination dir if it doesn't exists.
//
// It will error if
//   - dest exists but it is not a dir
//   - dest exists and it is a dir but it is not empty
//
// Returns the absolute path to the destination dir.
func prepareDestDir(ctx context.Context, dest string) (string, error) {
	logging.Infof(ctx, "preparing destination directory %q", dest)
	ret, err := filepath.Abs(dest)
	if err != nil {
		return "", fmt.Errorf("failed to convert dest path %q to absolute path: %w", dest, err)
	}

	switch empty, err := filesystem.IsEmptyDir(ret); {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.MkdirAll(ret, fs.ModePerm); err != nil {
			return "", fmt.Errorf("failed to create dir %q: %w", ret, err)
		}
	case err != nil:
		return "", err
	case !empty:
		return "", fmt.Errorf("dest path %q exists but the directory is not empty", dest)
	}
	return ret, nil
}

// exportRepo packages the repo to the provided dest.
func exportRepo(ctx context.Context, repo *Repo, dest string) error {
	return errors.New("implement")
}

// exportProtos copies the proto files of the given repo to the provided dest.
func exportProtos(repo *Repo, dest string) error {
	return errors.New("implement")
}

// prepareScripts creates entrypoint scripts for various platforms.
func prepareScripts(main *Repo, deps []*Repo, dest string) error {
	return errors.New("implement")
}
