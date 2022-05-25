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

"""Defines luci.bucket(...) rule."""

load("@stdlib//internal/graph.star", "graph")
load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/validate.star", "validate")
load("@stdlib//internal/luci/common.star", "keys", "kinds")
load("@stdlib//internal/luci/lib/acl.star", "aclimpl")
load("@stdlib//internal/luci/rules/binding.star", "binding")
load("@stdlib//internal/luci/rules/realm.star", "realm")

def _bucket(
        ctx,  # @unused
        *,
        name = None,
        acls = None,
        extends = None,
        bindings = None):
    """Defines a bucket: a container for LUCI builds.

    This rule also implicitly defines the realm to use for the builds in this
    bucket. It can be used to specify permissions that apply to all builds in
    this bucket and all resources these builds produce. See luci.realm(...).

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      name: name of the bucket, e.g. `ci` or `try`. Required.
      acls: list of acl.entry(...) objects. Being gradually replaced by
        luci.binding(...) in `bindings`.
      extends: a reference or a list of references to realms to inherit
        permission from. Note that buckets themselves are realms for this
        purpose. Optional. Default (and implicit) is `@root`.
      bindings: a list of luci.binding(...) to add to the bucket's realm.
        Will eventually replace `acls`.
    """
    name = validate.string("name", name)
    if name.startswith("luci."):
        fail('long bucket names aren\'t allowed, please use e.g. "try" instead of "luci.project.try"')

    key = keys.bucket(name)
    graph.add_node(key, props = {
        "name": name,
        "acls": aclimpl.validate_acls(acls, project_level = False),
    })
    graph.add_edge(keys.project(), key)

    # Convert legacy `acls` entries into binding(...) too.
    bindings = bindings[:] if bindings else []
    bindings.extend([binding(**d) for d in aclimpl.binding_dicts(acls)])

    # Define the realm and grab its keyset.
    realm_ref = realm(
        name = name,
        extends = extends,
        bindings = bindings,
    )

    # Return both the bucket and the realm keys, so callers can pick the one
    # they need based on its kind.
    return graph.keyset(key, realm_ref.get(kinds.REALM))

bucket = lucicfg.rule(impl = _bucket)
