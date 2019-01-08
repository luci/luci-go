# Copyright 2019 The LUCI Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

load('@stdlib//internal/graph.star', 'graph')
load('@stdlib//internal/luci/common.star', 'keys')
load('@stdlib//internal/luci/lib/validate.star', 'validate')


def recipe(
    name,
    recipe=None,
    repository=None,
    cipd_package=None,
    cipd_version=None):
  """Defines where to locate a particular recipe.

  Builders refer to recipes in their 'recipe' field. Multiple builders can
  execute same recipe (perhaps passing different properties to it).

  Recipes are located inside recipe packages. A recipe package can either be
  loaded from 'refs/heads/master' of some git repository (the deprecated legacy
  way), or from a particular version of a cipd package, called "recipe bundle".
  The latter is the preferred way.

  Typically the cipd package name with the recipe bundle will look like:

      infra/recipe_bundles/chromium.googlesource.com/chromium/tools/build

  Recipes bundled from internal repositories are typically under

      infra_internal/recipe_bundles/...

  But if you're building your own recipe bundles, they could be located
  elsewhere.

  The cipd version to fetch is usually a lower-cased git ref (like
  'refs/heads/master'), or it can be a cipd tag (like 'git_revision:abc...').

  Args:
    name: name of this recipe entity to refer to it from builders.
    recipe: name of a recipe inside the recipe package, defaults to 'name'.
    repository: git repository URL of the recipe package.
    cipd_package: a cipd package name with the recipe bundle.
    cipd_version: a version of the recipe bundle package to fetch.
  """
  name = validate.string('name', name)
  recipe = validate.string('recipe', recipe, default=name, required=False)
  repository = validate.string('repository', repository, required=False)
  cipd_package = validate.string('cipd_package', cipd_package, required=False)
  cipd_version = validate.string('cipd_version', cipd_version, required=False)

  if cipd_package and not cipd_version:
    fail('bad "cipd_version": required if "cipd_package" is set')
  if cipd_version and not cipd_package:
    fail('bad "cipd_package": required if "cipd_version" is set')

  if cipd_package and repository:
    fail('"cipd_package" and "repository" are incompatible with each other')
  if not cipd_package and not repository:
    fail('either "cipd_package" or "repository" is required')

  graph.add_node(keys.recipe(name), props = {
      'recipe': recipe,
      'repository': repository,
      'cipd_package': cipd_package,
      'cipd_version': cipd_version,
  })
