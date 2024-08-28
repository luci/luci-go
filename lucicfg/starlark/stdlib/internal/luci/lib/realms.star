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

# A construct for conditions passed to luci.binding(...).
_condition_ctor = __native__.genstruct("luci.binding_condition")

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
        regexp = r"^([a-z0-9_\.\-/]{1,400}|@root|@legacy|@project)$",
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
        projects = None,
        conditions = None):
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
    conditions = _validate_conditions("conditions", conditions)

    # Note: the second argument here is irrelevant. It will appear when this key
    # is printed but otherwise it is unused.
    key = keys.unique(impl.kinds.binding, ",".join(roles_strs))
    graph.add_node(key, props = {
        "roles": roles_strs,
        "groups": groups,
        "users": users,
        "projects": projects,
        "conditions": conditions,
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

def _restrict_attribute(attribute, values):
    """A condition for luci.binding(...) to restrict allowed attribute values.

    When a service checks a permission, it passes to the authorization library
    a string-valued dictionary of attributes that describes the context of the
    permission check. It contains things like the name of the resource being
    accessed, or parameters of the incoming RPC request that triggered the
    check.

    luci.restrict_attribute(...) condition makes the binding active only if
    the value of the given attribute is in the given set of allowed values.

    A list of available attributes and meaning of their values depends on the
    permission being checked and is documented in the corresponding service
    documentation.

    DocTags:
      Experimental.

    Args:
      attribute: name of the attribute to restrict.
      values: a list of strings with allowed values of the attribute.

    Returns:
      An opaque struct that can be passed to luci.binding(...) as a condition.
    """
    return _condition_ctor(
        restrict = struct(
            attribute = validate.string("attribute", attribute, required = True),
            values = tuple(sorted(set(validate.str_list("values", values)))),
        ),
    )

def _validate_str_or_list(name, val, required = False):
    if type(val) == "string":
        val = [val]
    elif val != None and type(val) != "list":
        validate.string(name, val)
    val = validate.list(name, val, required = required)
    for v in val:
        validate.string(name, v)
    return sorted(set(val))

def _validate_conditions(name, val):
    """Validates and normalizes a list of conditions.

    Sorts conditions according to some arbitrary (but stable) order. Returns
    a struct with conditions in their proto form and in a form of a dict key
    that can be used to group and dedup bindings that match these conditions.
    """
    conds = []  # [(key, proto)]
    for v in validate.list(name, val):
        validate.struct(name, v, _condition_ctor)
        key = None
        msg = realms_pb.Condition()
        if v.restrict:
            # Note: v.restrict.values is a sorted string tuple here already.
            key = (0, v.restrict.attribute, v.restrict.values)
            msg.restrict = realms_pb.Condition.AttributeRestriction(
                attribute = v.restrict.attribute,
                values = v.restrict.values,
            )
        else:
            fail("unexpected luci.binding_condition: %s" % v)
        conds.append((key, msg))

    # Sort and get a composite key that encodes all conditions in the list.
    protos, encoded = [], []
    for key, msg in sorted(conds, key = lambda x: x[0]):
        protos.append(msg)
        encoded.append(key)

    return struct(
        protos = protos,
        encoded = tuple(encoded),
    )

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

    # (role, encoded conditions) => [(role, [realms_pb.Condition], set(str))]
    bindings = {}
    for b in graph.children(realm.key, impl.kinds.binding):
        for role in b.props.roles:
            principals = []
            principals.extend(["group:" + g for g in b.props.groups])
            principals.extend(["user:" + u for u in b.props.users])
            principals.extend(["project:" + p for p in b.props.projects])
            if not principals:
                continue

            # Update an existing binding with the exact same role+conditions
            # or create a new one.
            binding = bindings.setdefault(
                (role, b.props.conditions.encoded),
                [role, b.props.conditions.protos, set()],
            )
            binding[2] = binding[2].union(principals)

    # Convert into a sorted list of (role, conditions, set of principals).
    bindings = [bindings[key] for key in sorted(bindings)]

    parents = graph.parents(realm.key, impl.kinds.realm)
    return realms_pb.Realm(
        name = realm.props.name,
        extends = [p.props.name for p in parents if p.props.name != "@root"],
        bindings = [
            realms_pb.Binding(
                role = role,
                principals = sorted(principals),
                conditions = conditions,
            )
            for role, conditions, principals in bindings
        ],
        enforce_in_service = realm.props.enforce_in_service,
    )

def _append_binding_pb(realms_cfg, realm, binding):
    """Appends a binding to an existing realms_pb.RealmsCfg proto.

    Args:
      realms_cfg: realms_pb.RealmsCfg to mutate.
      realm: a name of the realm to mutate or add.
      binding: a realms_pb.Binding to append.
    """
    for r in realms_cfg.realms:
        if r.name == realm:
            for b in r.bindings:
                if b.role == binding.role and b.conditions == binding.conditions:
                    b.principals.extend(binding.principals)
                    b.principals = sorted(set(b.principals))
                    return
            r.bindings.append(binding)
            return
    realms_cfg.realms.append(realms_pb.Realm(
        name = realm,
        bindings = [binding],
    ))

def _generate_custom_role(_impl, role):
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

    # Condition constructors.
    restrict_attribute = _restrict_attribute,

    # The generator helpers (to run from lucicfg.generator).
    generate_realms_cfg = _generate_realms_cfg,
    append_binding_pb = _append_binding_pb,
)
