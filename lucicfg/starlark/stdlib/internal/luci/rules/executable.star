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

"""Defines luci.executable(...) and luci.recipe(...) rules."""

load("@stdlib//internal/graph.star", "graph")
load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/validate.star", "validate")
load("@stdlib//internal/luci/common.star", "keys")

def _executable_props(
        ctx,
        cipd_package = None,
        cipd_version = None,
        recipe = None,
        recipes_py3 = None,
        cmd = None,
        wrapper = None):
    """Defines an executable's properties. See luci.executable(...).

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      cipd_package: a cipd package name with the executable. Supports the
        module-scoped default.
      cipd_version: a version of the executable package to fetch, default
        is `refs/heads/main`. Supports the module-scoped default.
      recipe: name of a recipe. Required if the executable is a recipe bundle.
      recipes_py3: True iff this is a recipe and should run under python3.
      cmd: a list of strings to use as the executable command. If None
        (or empty), Buildbucket will fill this in on the server side to either
        `['luciexe']` or `['recipes']`, depending on its global configuration.
      wrapper: an optional list of strings which are a command and its arguments
        to wrap around `cmd`. If set, the builder will run `<wrapper> -- <cmd>`.
        The 0th argument of the wrapper may be an absolute path. It is up to the
        owner of the builder to ensure that the wrapper executable is distributed
        to whatever machines this executable may run on.
    """
    return {
        "cipd_package": validate.string(
            "cipd_package",
            cipd_package,
            default = ctx.defaults.cipd_package.get(),
            required = ctx.defaults.cipd_package.get() == None,
        ),
        "cipd_version": validate.string(
            "cipd_version",
            cipd_version,
            default = ctx.defaults.cipd_version.get() or "refs/heads/main",
            required = False,
        ),
        "recipe": validate.string(
            "recipe",
            recipe,
            required = False,
        ),
        "recipes_py3": validate.bool(
            "recipes_py3",
            recipes_py3,
            required = False,
        ),
        "cmd": validate.str_list(
            "cmd",
            cmd,
            required = False,
        ),
        "wrapper": validate.str_list(
            "wrapper",
            wrapper,
            required = False,
        ),
    }

def _executable(
        ctx,
        name = None,
        cipd_package = None,
        cipd_version = None,
        cmd = None,
        wrapper = None):
    """Defines an executable.

    Builders refer to such executables in their `executable` field, see
    luci.builder(...). Multiple builders can execute the same executable
    (perhaps passing different properties to it).

    Executables must be available as cipd packages.

    The cipd version to fetch is usually a lower-cased git ref (like
    `refs/heads/main`), or it can be a cipd tag (like `git_revision:abc...`).

    A luci.executable(...) with some particular name can be redeclared many
    times as long as all fields in all declaration are identical. This is
    helpful when luci.executable(...) is used inside a helper function that at
    once declares a builder and an executable needed for this builder.

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      name: name of this executable entity, to refer to it from builders.
        Required.
      cipd_package: a cipd package name with the executable. Supports the
        module-scoped default.
      cipd_version: a version of the executable package to fetch, default
        is `refs/heads/main`. Supports the module-scoped default.
      cmd: a list of strings which are the command line to use for this
        executable. If omitted, either `('recipes',)` or `('luciexe',)` will be
        used by Buildbucket, according to its global configuration. The special
        value of `('recipes',)` indicates that this executable should be run
        under the legacy kitchen runtime. All other values will be executed
        under the go.chromium.org/luci/luciexe protocol.
      wrapper: an optional list of strings which are a command and its arguments
        to wrap around `cmd`. If set, the builder will run `<wrapper> -- <cmd>`.
        The 0th argument of the wrapper may be an absolute path. It is up to the
        owner of the builder to ensure that the wrapper executable is distributed
        to whatever machines this executable may run on.
    """
    name = validate.string("name", name)
    key = keys.executable(name)
    props = _executable_props(
        ctx,
        cipd_package = cipd_package,
        cipd_version = cipd_version,
        cmd = cmd,
        wrapper = wrapper,
    )
    graph.add_node(key, idempotent = True, props = props)
    return graph.keyset(key)

def _recipe(
        ctx,
        *,
        name = None,
        cipd_package = None,
        cipd_version = None,
        recipe = None,
        use_bbagent = None,  # transitional for crbug.com/1015181
        use_python3 = None):
    """Defines an executable that runs a particular recipe.

    Recipes are python-based DSL for defining what a builder should do, see
    [recipes-py](https://chromium.googlesource.com/infra/luci/recipes-py/).

    Builders refer to such executable recipes in their `executable` field, see
    luci.builder(...). Multiple builders can execute the same recipe (perhaps
    passing different properties to it).

    Recipes are located inside cipd packages called "recipe bundles". Typically
    the cipd package name with the recipe bundle will look like:

        infra/recipe_bundles/chromium.googlesource.com/chromium/tools/build

    Recipes bundled from internal repositories are typically under

        infra_internal/recipe_bundles/...

    But if you're building your own recipe bundles, they could be located
    elsewhere.

    The cipd version to fetch is usually a lower-cased git ref (like
    `refs/heads/main`), or it can be a cipd tag (like `git_revision:abc...`).

    A luci.recipe(...) with some particular name can be redeclared many times as
    long as all fields in all declaration are identical. This is helpful when
    luci.recipe(...) is used inside a helper function that at once declares
    a builder and a recipe needed for this builder.

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      name: name of this recipe entity, to refer to it from builders. If
        `recipe` is None, also specifies the recipe name within the bundle.
        Required.
      cipd_package: a cipd package name with the recipe bundle. Supports the
        module-scoped default.
      cipd_version: a version of the recipe bundle package to fetch, default
        is `refs/heads/main`. Supports the module-scoped default.
      recipe: name of a recipe inside the recipe bundle if it differs from
        `name`. Useful if recipe names clash between different recipe bundles.
        When this happens, `name` can be used as a non-ambiguous alias, and
        `recipe` can provide the actual recipe name. Defaults to `name`.
      use_bbagent: a boolean to override Buildbucket's global configuration. If
        True, then builders with this recipe will always use bbagent. If False,
        then builders with this recipe will temporarily stop using bbagent (note
        that all builders are expected to use bbagent by ~2020Q3). Defaults to
        unspecified, which will cause Buildbucket to pick according to it's own
        global configuration. See [this bug](crbug.com/1015181) for the global
        bbagent rollout. Supports the module-scoped default.
      use_python3: a boolean to use python3 to run the recipes. If set, also
        implies use_bbagent=True. This is equivalent to setting the
        'luci.recipes.use_python3' experiment on the builder to 100%.
        Supports the module-scoped default.
    """
    name = validate.string("name", name)
    use_bbagent = validate.bool(
        "use_bbagent",
        use_bbagent,
        required = False,
        default = ctx.defaults.use_bbagent.get(),
    )
    use_python3 = validate.bool(
        "use_python3",
        use_python3,
        required = False,
        default = ctx.defaults.use_python3.get(),
    )
    key = keys.executable(name)

    if use_python3:
        use_bbagent = True

    cmd = None
    if use_bbagent != None:
        if use_bbagent:
            cmd = ["luciexe"]
        else:
            cmd = ["recipes"]

    props = _executable_props(
        ctx,
        cipd_package = cipd_package,
        cipd_version = cipd_version,
        recipe = recipe or name,
        recipes_py3 = use_python3,
        cmd = cmd,
    )
    graph.add_node(key, idempotent = True, props = props)
    return graph.keyset(key)

executable = lucicfg.rule(
    impl = _executable,
    defaults = validate.vars_with_validators({
        "cipd_package": validate.string,
        "cipd_version": validate.string,
    }),
)

recipe = lucicfg.rule(
    impl = _recipe,
    defaults = validate.vars_with_validators({
        "cipd_package": validate.string,
        "cipd_version": validate.string,
        "use_bbagent": validate.bool,
        "use_python3": validate.bool,
    }),
)
