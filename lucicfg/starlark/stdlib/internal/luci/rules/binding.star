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

"""Defines luci.binding(...) rule."""

load("@stdlib//internal/graph.star", "graph")
load("@stdlib//internal/lucicfg.star", "lucicfg")
load("@stdlib//internal/validate.star", "validate")
load("@stdlib//internal/luci/common.star", "keys", "kinds")
load("@stdlib//internal/luci/lib/realms.star", "realms")

def _binding(
        ctx,
        *,
        realm = None,
        roles = None,
        groups = None,
        users = None,
        projects = None):
    """Binding assigns roles in a realm to individuals, groups or LUCI projects.

    A role can either be predefined (if its name starts with `role/`) or custom
    (if its name starts with `customRole/`).

    Predefined roles are declared in the LUCI deployment configs, see **TODO**
    for the up-to-date list of available predefined roles and their meaning.

    Custom roles are defined in the project configs via luci.custom_role(...).
    They can be used if none of the predefined roles represent the desired set
    of permissions.

    DocTags:
      Experimental.

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      realm: a single realm or a list of realms to add the binding to. Can be
        omitted if the binding is used inline inside some luci.realm(...)
        declaration.
      roles: a single role or a list of roles to assign. Required.
      groups: a single group name or a list of groups to assign the role to.
      users: a single user email or a list of emails to assign the role to.
      projects: a single LUCI project name or a list of project names to assign
        the role to.
    """
    realms.experiment.require()

    realm_keys = []
    if realm != None:
        if type(realm) != "list":
            realm = [realm]
        realm_keys = [keys.realm(r) for r in realm]

    if roles != None and type(roles) != "list":
        roles = [roles]

    # Each element of `roles` is either a string 'role/...' or 'customRole/...',
    # or a reference to luci.custom_role(...) node. We link to all custom role
    # nodes to make sure they are defined.
    roles_strs = []  # 'role/...' and 'customRole/...'
    roles_keys = []  # keys of custom roles only
    for r in validate.list("roles", roles, required = True):
        if graph.is_keyset(r) and r.has(kinds.CUSTOM_ROLE):
            key = r.get(kinds.CUSTOM_ROLE)
            roles_strs.append(key.id)  # grab its string name
            roles_keys.append(key)
        elif type(r) == "string":
            if not r.startswith("role/") and not r.startswith("customRole/"):
                fail('bad "roles": %r should start with "role/" or "customeRole/"' % (r,))
            roles_strs.append(r)
            if r.startswith("customRole/"):
                roles_keys.append(keys.custom_role(r))
        else:
            fail('bad "roles": %r is not a string nor a luci.custom_role(...)' % (r,))
    roles_strs = sorted(set(roles_strs))

    groups = _validate_str_or_list("groups", groups)
    users = _validate_str_or_list("users", users)
    projects = _validate_str_or_list("projects", projects)

    # Note: the second argument here is irrelevant. It will appear when this key
    # is printed but otherwise it is unused.
    key = keys.unique(kinds.BINDING, ",".join(roles_strs))
    graph.add_node(key, props = {
        "roles": roles_strs,
        "groups": groups,
        "users": users,
        "projects": projects,
    })

    # This adds the binding to the realm(s).
    for r in realm_keys:
        graph.add_edge(r, key)

    # This makes sure all referenced custom roles are defined.
    for r in roles_keys:
        graph.add_edge(r, key, title = "roles")

    # This is used to detect luci.binding that are not added to any realm.
    graph.add_node(keys.bindings_root(), idempotent = True)
    graph.add_edge(keys.bindings_root(), key)

    return graph.keyset(key)

def _validate_str_or_list(name, val, required = False):
    if type(val) == "string":
        val = [val]
    elif val != None and type(val) != "list":
        validate.string(name, val)
    val = validate.list(name, val, required = required)
    for v in val:
        validate.string(name, v)
    return sorted(set(val))

binding = lucicfg.rule(impl = _binding)
