# Copyright 2018 The LUCI Authors.
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

"""Defines luci.project(...) rule."""

load("@stdlib//internal/graph.star", "graph")
load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/validate.star", "validate")
load("@stdlib//internal/luci/common.star", "keys")
load("@stdlib//internal/luci/lib/acl.star", "aclimpl")
load("@stdlib//internal/luci/lib/service.star", "service")
load("@stdlib//internal/luci/rules/binding.star", "binding")
load("@stdlib//internal/luci/rules/realm.star", "realm")

def _project(
        ctx,  # @unused
        *,
        name = None,
        config_dir = None,
        dev = False,
        buildbucket = None,
        logdog = None,
        milo = None,
        notify = None,
        scheduler = None,
        swarming = None,
        change_verifier = None,
        tricium = None,
        acls = None,
        bindings = None,
        enforce_realms_in = None,
        omit_lucicfg_metadata = False):
    """Defines a LUCI project.

    There should be exactly one such definition in the top-level config file.

    This rule also implicitly defines the `@root` realm of the project. It can
    be used to setup permissions that apply to all resources in the project. See
    luci.realm(...).

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      name: full name of the project. Required.
      config_dir: a subdirectory of the config output directory (see
        `config_dir` in lucicfg.config(...)) to place generated LUCI configs
        under. Default is `.`. A custom value is useful when using `lucicfg`
        to generate LUCI and non-LUCI configs at the same time.
      dev: set to True if this project belongs to a development or a staging
        LUCI deployment. This is rare. Default is False.
      buildbucket: appspot hostname of a Buildbucket service to use (if any).
      logdog: appspot hostname of a LogDog service to use (if any).
      milo: appspot hostname of a Milo service to use (if any).
      notify: appspot hostname of a LUCI Notify service to use (if any).
      scheduler: appspot hostname of a LUCI Scheduler service to use (if any).
      swarming: appspot hostname of a Swarming service to use by default
        (if any).
      change_verifier: appspot hostname of a LUCI Change Verifier (LUCI CV)
        service to use by default (if any).
      tricium: appspot hostname of a Tricium service to use by default
        (if any).
      acls: list of acl.entry(...) objects, will be inherited by all buckets.
        Being gradually replaced by luci.binding(...) in `bindings`.
      bindings: a list of luci.binding(...) to add to the root realm. They will
        be inherited by all realms in the project. Will eventually replace
        `acls`.
      enforce_realms_in: a list of LUCI service IDs that should enforce realms
        permissions across all realms. Used only during Realms migration to
        gradually roll out the enforcement. Can also be enabled realm-by-realm
        via `enforce_in` in luci.realm(...).
      omit_lucicfg_metadata: if True, do not generate `lucicfg {...}` block
        with lucicfg invocation details in `project.cfg`. This may be useful if
        you pass frequently changing `-var ...` when generating configs and
        the resulting generated `lucicfg { vars {...} }` metadata causes
        frequent merge conflicts. This option is **strongly discouraged** as it
        makes it impossible to reproducibly regenerate project configs in
        the LUCI automation (it doesn't know what var values to use). If you use
        this option, your project may be left out of automatic config
        migrations. If this happens, you'll need to manually complete the
        migration on-schedule in order to have your LUCI project continue to
        function.
    """
    key = keys.project()
    graph.add_node(key, props = {
        "name": validate.string("name", name),
        "config_dir": validate.relative_path("config_dir", config_dir, required = False, default = "."),
        "dev": validate.bool("dev", dev, required = False, default = False),
        "buildbucket": service.from_host("buildbucket", buildbucket),
        "logdog": service.from_host("logdog", logdog),
        "milo": service.from_host("milo", milo),
        "notify": service.from_host("notify", notify),
        "scheduler": service.from_host("scheduler", scheduler),
        "swarming": service.from_host("swarming", swarming),
        "change_verifier": service.from_host("change_verifier", change_verifier),
        "tricium": service.from_host("tricium", tricium),
        "acls": aclimpl.validate_acls(acls, project_level = True),
        "omit_lucicfg_metadata": validate.bool("omit_lucicfg_metadata", omit_lucicfg_metadata, required = False, default = False),
    })

    # Convert legacy `acls` entries into binding(...) too.
    bindings = bindings[:] if bindings else []
    bindings.extend([binding(**d) for d in aclimpl.binding_dicts(acls)])

    # Add all bindings to the root realm.
    realm(name = "@root", bindings = bindings, enforce_in = enforce_realms_in)

    return graph.keyset(key)

project = lucicfg.rule(impl = _project)
