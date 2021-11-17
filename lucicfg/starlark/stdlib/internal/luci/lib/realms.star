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

"""Helper library for working with LUCI Realms.

Contains all logic to define realms and generate realms.cfg. It is formulated
as a (mostly) self-contained library so it can be reused to generate internal
realms. The code that does this lives elsewhere.
"""

load("@stdlib//internal/error.star", "error")
load("@stdlib//internal/experiments.star", "experiments")
load("@stdlib//internal/graph.star", "graph")
load("@stdlib//internal/luci/common.star", "keys", "kinds")
load("@stdlib//internal/luci/proto.star", "realms_pb")
load("@stdlib//internal/validate.star", "validate")

def _implementation_api(
        *,

        # Key kinds.
        kinds_realm,
        kinds_custom_role,
        kinds_binding,

        # Key constructors.
        keys_realms_root,
        keys_bindings_root,
        keys_custom_role,
        keys_realm):
    """Returns a struct with key constructors for realm graph nodes."""
    return struct(
        kinds = struct(
            realm = kinds_realm,
            custom_role = kinds_custom_role,
            binding = kinds_binding,
        ),
        keys = struct(
            realms_root = keys_realms_root,
            bindings_root = keys_bindings_root,
            custom_role = keys_custom_role,
            realm = keys_realm,
        ),
    )

# Default keys and kinds used by stdlib lucicfg rules.
_default_impl = _implementation_api(
    # Key kinds.
    kinds_realm = kinds.REALM,
    kinds_custom_role = kinds.CUSTOM_ROLE,
    kinds_binding = kinds.BINDING,

    # Key constructors.
    keys_realms_root = keys.project,
    keys_bindings_root = keys.bindings_root,
    keys_custom_role = keys.custom_role,
    keys_realm = keys.realm,
)

def _realm(
        *,
        impl,
        name,
        extends = None,
        bindings = None,
        enforce_in_service = None):
    """Implementation of luci.realm(...) in terms of `impl`."""
    name = validate.string(
        "name",
        name,
        regexp = r"^([a-z0-9_\.\-/]{1,400}|@root|@legacy)$",
    )

    # Implicitly add '@root' to parents (unless we are defining it).
    if extends and type(extends) != "list":
        extends = [extends]
    parents = [impl.keys.realm(r) for r in extends or []]
    if name != "@root":
        parents.append(impl.keys.realm("@root"))

    # '@root' must be at the root of the inheritance tree.
    if name == "@root" and len(parents):
        fail("@root realm can't extend other realms")

    key = impl.keys.realm(name)
    graph.add_node(key, props = {
        "name": name,
        "enforce_in_service": sorted(set(enforce_in_service or [])),
    }, idempotent = True)
    graph.add_edge(impl.keys.realms_root(), key)
    for parent in parents:
        graph.add_edge(parent, key, title = "extends")

    for b in validate.list("bindings", bindings):
        if graph.is_keyset(b) and b.has(impl.kinds.binding):
            graph.add_edge(key, b.get(impl.kinds.binding))
        else:
            fail('bad "bindings": %s is not a luci.binding(...)' % (b,))

    return graph.keyset(key)

def _custom_role(
        *,
        impl,
        name,
        extends = None,
        permissions = None):
    """Implement luci.custom_role(...) in terms of `impl`."""
    name = validate.string("name", name)
    if not name.startswith("customRole/"):
        fail('bad "name": %r must start with "customRole/"' % (name,))

    # Each element of `extends` is either a string 'role/...' or
    # 'customRole/...', or a reference to luci.custom_role(...) node. We link to
    # all custom role nodes to make sure they are defined and to check for
    # cycles.
    extends_strs = []  # 'role/...' and 'customRole/...'
    parents_keys = []  # keys of custom roles only
    for r in validate.list("extends", extends):
        if graph.is_keyset(r) and r.has(impl.kinds.custom_role):
            key = r.get(impl.kinds.custom_role)
            extends_strs.append(key.id)  # grab its string name
            parents_keys.append(key)
        elif type(r) == "string":
            if not r.startswith("role/") and not r.startswith("customRole/"):
                fail('bad "extends": %r should start with "role/" or "customeRole/"' % (r,))
            extends_strs.append(r)
            if r.startswith("customRole/"):
                parents_keys.append(impl.keys.custom_role(r))
        else:
            fail('bad "extends": %r is not a string nor a luci.custom_role(...)' % (r,))

    key = impl.keys.custom_role(name)
    graph.add_node(key, props = {
        "name": name,
        "extends": sorted(set(extends_strs)),
        "permissions": sorted(set(validate.str_list("permissions", permissions))),
    }, idempotent = True)
    graph.add_edge(impl.keys.realms_root(), key)
    for parent in parents_keys:
        graph.add_edge(parent, key, title = "extends")

    return graph.keyset(key)

def _binding(
        *,
        impl,
        realm = None,
        roles = None,
        groups = None,
        users = None,
        projects = None):
    """Implements luci.binding(...) in terms of `impl`."""
    realm_keys = []
    if realm != None:
        if type(realm) != "list":
            realm = [realm]
        realm_keys = [impl.keys.realm(r) for r in realm]

    if roles != None and type(roles) != "list":
        roles = [roles]

    # Each element of `roles` is either a string 'role/...' or 'customRole/...',
    # or a reference to luci.custom_role(...) node. We link to all custom role
    # nodes to make sure they are defined.
    roles_strs = []  # 'role/...' and 'customRole/...'
    roles_keys = []  # keys of custom roles only
    for r in validate.list("roles", roles, required = True):
        if graph.is_keyset(r) and r.has(impl.kinds.custom_role):
            key = r.get(impl.kinds.custom_role)
            roles_strs.append(key.id)  # grab its string name
            roles_keys.append(key)
        elif type(r) == "string":
            if not r.startswith("role/") and not r.startswith("customRole/"):
                fail('bad "roles": %r should start with "role/" or "customeRole/"' % (r,))
            roles_strs.append(r)
            if r.startswith("customRole/"):
                roles_keys.append(impl.keys.custom_role(r))
        else:
            fail('bad "roles": %r is not a string nor a luci.custom_role(...)' % (r,))
    roles_strs = sorted(set(roles_strs))

    groups = _validate_str_or_list("groups", groups)
    users = _validate_str_or_list("users", users)
    projects = _validate_str_or_list("projects", projects)

    # Note: the second argument here is irrelevant. It will appear when this key
    # is printed but otherwise it is unused.
    key = keys.unique(impl.kinds.binding, ",".join(roles_strs))
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
    graph.add_node(impl.keys.bindings_root(), idempotent = True)
    graph.add_edge(impl.keys.bindings_root(), key)

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

def _generate_realms_cfg(impl):
    """Implementation of realms.cfg generation."""

    # Discover bindings not attached to any realm. They likely indicate a bug.
    for b in graph.children(impl.keys.bindings_root()):
        if not graph.parents(b.key, impl.kinds.realm):
            error("the binding %s is not added to any realm" % b, trace = b.trace)

    return realms_pb.RealmsCfg(
        realms = [
            _generate_realm(impl, r)
            for r in graph.children(impl.keys.realms_root(), impl.kinds.realm)
        ],
        custom_roles = [
            _generate_custom_role(impl, r)
            for r in graph.children(impl.keys.realms_root(), impl.kinds.custom_role)
        ],
    )

def _generate_realm(impl, realm):
    """Given a REALM node returns realms_pb.Realm."""
    per_role = {}
    for b in graph.children(realm.key, impl.kinds.binding):
        for role in b.props.roles:
            principals = []
            principals.extend(["group:" + g for g in b.props.groups])
            principals.extend(["user:" + u for u in b.props.users])
            principals.extend(["project:" + p for p in b.props.projects])
            if role in per_role:
                per_role[role] = per_role[role].union(principals)
            else:
                per_role[role] = set(principals)

    # Convert into a sorted list of pairs (role, set of principals).
    per_role = sorted(per_role.items(), key = lambda x: x[0])

    parents = graph.parents(realm.key, impl.kinds.realm)
    return realms_pb.Realm(
        name = realm.props.name,
        extends = [p.props.name for p in parents if p.props.name != "@root"],
        bindings = [
            realms_pb.Binding(
                role = role,
                principals = sorted(bindings),
            )
            for role, bindings in per_role
            if bindings
        ],
        enforce_in_service = realm.props.enforce_in_service,
    )

def _generate_custom_role(impl, role):
    """Given a CUSTOM_ROLE node returns realms_pb.CustomRole."""
    return realms_pb.CustomRole(
        name = role.props.name,
        extends = role.props.extends,
        permissions = role.props.permissions,
    )

realms = struct(
    # Internal API to reuse realms generator logic outside of lucicfg stdlib.
    implementation_api = _implementation_api,
    default_impl = _default_impl,

    # Implementation of individual rules.
    realm = _realm,
    custom_role = _custom_role,
    binding = _binding,

    # The generator (to run from lucicfg.generator).
    generate_realms_cfg = _generate_realms_cfg,
)
