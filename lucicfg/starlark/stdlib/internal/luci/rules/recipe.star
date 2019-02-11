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
load('@stdlib//internal/validate.star', 'validate')

load('@stdlib//internal/luci/common.star', 'keys')


def recipe(
      name=None,
      cipd_package=None,
      cipd_version=None,
      recipe=None,
  ):
  """Defines where to locate a particular recipe.

  Builders refer to recipes in their `recipe` field, see core.builder(...).
  Multiple builders can execute the same recipe (perhaps passing different
  properties to it).

  Recipes are located inside cipd packages called "recipe bundles". Typically
  the cipd package name with the recipe bundle will look like:

      infra/recipe_bundles/chromium.googlesource.com/chromium/tools/build

  Recipes bundled from internal repositories are typically under

      infra_internal/recipe_bundles/...

  But if you're building your own recipe bundles, they could be located
  elsewhere.

  The cipd version to fetch is usually a lower-cased git ref (like
  `refs/heads/master`), or it can be a cipd tag (like `git_revision:abc...`).

  Args:
    name: name of this recipe entity, to refer to it from builders. If `recipe`
        is None, also specifies the recipe name within the bundle. Required.
    cipd_package: a cipd package name with the recipe bundle. Required.
    cipd_version: a version of the recipe bundle package to fetch, default
        is `refs/heads/master`.
    recipe: name of a recipe inside the recipe bundle if it differs from
        `name`. Useful if recipe names clash between different recipe bundles.
        When this happens, `name` can be used as a non-ambiguous alias, and
        `recipe` can provide the actual recipe name. Defaults to `name`.
  """
  name = validate.string('name', name)
  key = keys.recipe(name)
  graph.add_node(key, props = {
      'cipd_package': validate.string('cipd_package', cipd_package),
      'cipd_version': validate.string(
          'cipd_version',
          cipd_version,
          default='refs/heads/master',
          required=False,
      ),
      'recipe': validate.string(
          'recipe',
          recipe,
          default=name,
          required=False,
      ),
  })
  return graph.keyset(key)
