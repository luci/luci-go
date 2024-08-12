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
	"fmt"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/encoding/protojson"

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
