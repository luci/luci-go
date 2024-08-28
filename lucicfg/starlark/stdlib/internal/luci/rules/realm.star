# Copyright 2020 The LUCI Authors.
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

"""Defines luci.realm(...) rule."""

load("@stdlib//internal/luci/lib/realms.star", "realms")
load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/validate.star", "validate")

def _realm(
        ctx,  # @unused
        *,
        name,
        extends = None,
        bindings = None,
        enforce_in = None):
    """Defines a realm.

    Realm is a named collection of `(<principal>, <permission>)` pairs.

    A LUCI resource can point to exactly one realm by referring to its full
    name (`<project>:<realm>`). We say that such resource "belongs to the realm"
    or "lives in the realm" or is just "in the realm". We also say that such
    resource belongs to the project `<project>`. The corresponding
    luci.realm(...) definition then describes who can do what to the resource.

    The logic of how resources get assigned to realms is a part of the public
    API of the service that owns resources. Some services may use a static realm
    assignment via project configuration files, others may do it dynamically by
    accepting a realm when a resource is created via an RPC.

    A realm can "extend" one or more other realms. If a realm `A` extends `B`,
    then all permissions defined in `B` are also in `A`. Remembering that a
    realm is just a set of `(<principal>, <permission>)` pairs, the "extends"
    relation is just a set inclusion.

    There are three special realms that a project can have: "@root", "@legacy"
    and "@project".

    The root realm is implicitly included into all other realms (including
    "@legacy"), and it is also used as a fallback when a resource points to a
    realm that no longer exists. Without the root realm, such resources become
    effectively inaccessible and this may be undesirable. Permissions in the
    root realm apply to all realms in the project (current, past and future),
    and thus the root realm should contain only administrative-level bindings.
    If you are not sure whether you should use the root realm or not, err on
    the side of not using it.

    The legacy realm is used for existing resources created before the realms
    mechanism was introduced. Such resources usually are not associated with any
    realm at all. They are implicitly placed into the legacy realm to allow
    reusing realms' machinery for them.

    Note that the details of how resources are placed in the legacy realm are up
    to a particular service implementation. Some services may be able to figure
    out an appropriate realm for a legacy resource based on resource's existing
    attributes. Some services may not have legacy resources at all. The legacy
    realm is not used in these case. Refer to the service documentation.

    The project realm should be used as the realm for 'project global' resources,
    for example, the project configuration itself, or derivations thereof. Some
    LUCI services may use bindings in this realm to allow federation of
    administration responsibilities to the project (rather than relying on
    exclusively LUCI service administrators).

    The primary way of populating the permission set of a realm is via bindings.
    Each binding assigns a role to a set of principals (individuals, groups or
    LUCI projects). A role is just a set of permissions. A binding grants these
    permissions to all principals listed in it.

    Binding can be specific either right here:

        luci.realm(
            name = 'try',
            bindings = [
                luci.binding(
                    roles = 'role/a',
                    groups = ['group-a'],
                ),
                luci.binding(
                    roles = 'role/b',
                    groups = ['group-b'],
                ),
            ],
        )

    Or separately one by one via luci.binding(...) declarations:

        luci.binding(
            realm = 'try',
            roles = 'role/a',
            groups = ['group-a'],
        )
        luci.binding(
            realm = 'try',
            roles = 'role/b',
            groups = ['group-b'],
        )

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      name: name of the realm. Must match `[a-z0-9_\\.\\-/]{1,400}` or be
        `@root` or `@legacy`. Required.
      extends: a reference or a list of references to realms to inherit
        permission from. Optional. Default (and implicit) is `@root`.
      bindings: a list of luci.binding(...) to add to the realm.
      enforce_in: a list of LUCI service IDs that should enforce this realm's
        permissions. Children realms inherit and extend this list. Used only
        during Realms migration to gradually roll out the enforcement realm by
        realm, service by service.
    """
    return realms.realm(
        impl = realms.default_impl,
        name = name,
        extends = extends,
        bindings = bindings,
        enforce_in_service = validate.str_list("enforce_in", enforce_in),
    )

realm = lucicfg.rule(impl = _realm)
