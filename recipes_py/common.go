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
	"errors"

	recipepb "go.chromium.org/luci/recipes_py/proto"
)

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

// RepoFromPath creates a recipe Repo instance from a local path.
func RepoFromPath(path string) (*Repo, error) {
	return nil, errors.New("implement")
}
