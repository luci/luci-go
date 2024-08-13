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

	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/common/exec"
	recipepb "go.chromium.org/luci/recipes_py/proto"
)

var cfgPathSegments = []string{"infra", "config", "recipes.cfg"}

// Repo represents a recipe repo.
//
// It is a folder on disk which contains all of the requirements of a recipe
// repo:
//   - an infra/config/recipes.cfg file
//   - a `recipes` and/or `recipe_modules` folder
//   - a recipes.py script
type Repo struct {
	// Path to the repo root on disk. Absolute Path.
	Path string
	// Spec is the config in infra/config/recipes.cfg file
	Spec *recipepb.RepoSpec
}

// Name is a shorthand to get the recipe repo name.
func (r *Repo) Name() string {
	return r.Spec.GetRepoName()
}

// CfgPathRel returns the relative path from the repo root to recipes.cfg.
func (r *Repo) CfgPathRel() string {
	return filepath.Join(cfgPathSegments...)
}

// CfgPath returns the absolute path to recipes.cfg
func (r *Repo) CfgPath() string {
	return filepath.Join(r.Path, r.CfgPathRel())
}

// RecipesRootPath returns the path to the base where all recipes are found in
// the repo.
//
// This is where the "recipes" and "recipe_modules" directories live.
func (r *Repo) RecipesRootPath() string {
	if r.Spec.GetRecipesPath() == "" {
		return r.Path
	}
	return filepath.Join(r.Path, r.RecipesRootPathRel())
}

// RecipesRootPathRel is like RecipesRootPath but returns relative path to the
// root. Returns empty path if root path is the repo root.
func (r *Repo) RecipesRootPathRel() string {
	if r.Spec.GetRecipesPath() == "" {
		return ""
	}
	return filepath.FromSlash(r.Spec.GetRecipesPath())
}

// RecipeDepsPath returns the absolute path to the `.recipe_deps` directory.
func (r *Repo) RecipeDepsPath() string {
	return filepath.Join(r.RecipesRootPath(), ".recipe_deps")
}

// ensureDeps ensures the dependencies of the repo is populated.
//
// This is done by checking the existence of `RecipeDepsPath`.
// Invoke `recipes.py fetch` if `RecipeDepsPath` doesn't exist.
func (r *Repo) ensureDeps(ctx context.Context) error {
	switch _, err := os.Stat(r.RecipeDepsPath()); {
	case errors.Is(err, fs.ErrNotExist):
		cmd := exec.CommandContext(ctx, filepath.Join(r.RecipesRootPath(), "recipes.py"), "fetch")
		if err := cmd.Run(); err != nil {
			return errors.New("failed to run `recipes.py fetch`")
		}
		if _, err := os.Stat(r.RecipeDepsPath()); err != nil {
			// recipe_deps should have been populated by now.
			return err
		}
	case err != nil:
		return err
	}
	return nil

}

// RepoFromPath creates a recipe Repo instance from a local path.
func RepoFromPath(path string) (*Repo, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to convert repo path %q to absolute path: %w", path, err)
	}
	cfgPath := path
	for _, seg := range cfgPathSegments {
		cfgPath = filepath.Join(cfgPath, seg)
	}
	cfgContent, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read recipes.cfg at %q: %w", cfgPath, err)
	}
	repo := &Repo{
		Path: absPath,
		Spec: &recipepb.RepoSpec{},
	}
	if err := protojson.Unmarshal(cfgContent, repo.Spec); err != nil {
		return nil, fmt.Errorf("failed to parse recipes.cfg: %w", err)
	}
	return repo, nil
}
