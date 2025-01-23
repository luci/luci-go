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

"""Helper library for defining LUCI ACLs."""

load("@stdlib//internal/validate.star", "validate")

# TODO(vadimsh): Add support for 'anonymous' when/if needed.

# A constructor for acl.role structs.
#
# Such structs are seen through public API as predefined symbols, e.g.
# acl.LOGDOG_READER. There's no way for an end-user to define a new role.
#
# Expected to be used as roles in acl.entry(role=...) definitions, and maybe
# printed (when debugging).
#
# Fields:
#   name: name of the role.
#   project_level_only: True if the role can be set only in project(...) rule.
#   groups_only: True if the role should be assigned only to groups, not users.
_role_ctor = __native__.genstruct("acl.role")

# A constructor for acl.entry structs.
#
# Such structs are created via public acl.entry(...) API. To make their
# printable representation useful and not confusing to end users, their
# structure somewhat resembles acl.entry(...) arguments list.
#
# They are not convenient though when generating configs. For that reason
# there's another representation of ACLs: as a list of elementary
# (role, principal) tuples, where principals can be of few different types
# (e.g. groups or users). Internal API function 'normalize_acls' converts
# from the user-friendly acl.entry representation to the generator-friendly
# acl.elementary representation.
#
# Fields:
#   roles: a list of acl.role in the entry, at least one.
#   users: a list of user emails to apply roles to, may be empty.
#   groups: a list of group names to apply roles to, may be empty.
#   projects: a list of project names to apply roles to, may be empty.
_entry_ctor = __native__.genstruct("acl.entry")

# A constructor for acl.elementary structs.
#
# This is conceptually a sum type: (Role, User | Group | Project). For
# convenience it is represented as a tuple where only one of 'user', 'group' or
# 'project' is set.
#
# Fields:
#   role: an acl.role, always set.
#   user: an user email.
#   group: a group name.
#   project: a project name.
_elementary_ctor = __native__.genstruct("acl.elementary")

def _role(
        name,
        *,
        realms_role,
        project_level_only = False,
        groups_only = False):
    """Defines a role.

    Internal API. Only predefined roles are available publicly, see the bottom
    of this file.

    Args:
      name: string name of the role.
      realms_role: matching predefined Realms role.
      project_level_only: True if it can be used only in project(...) ACLs.
      groups_only: True if role supports only group-based ACL (not user-based).

    Returns:
      acl.role struct.
    """
    return _role_ctor(
        name = name,
        realms_role = realms_role,
        project_level_only = project_level_only,
        groups_only = groups_only,
    )

def _entry(
        roles,
        *,
        groups = None,
        users = None,
        projects = None):
    """Returns a new ACL binding.

    It assign the given role (or roles) to given individuals, groups or LUCI
    projects.

    Lists of acl.entry structs are passed to `acls` fields of luci.project(...)
    and luci.bucket(...) rules.

    An empty ACL binding is allowed. It is ignored everywhere. Useful for things
    like:

    ```python
    luci.project(
        acls = [
            acl.entry(acl.PROJECT_CONFIGS_READER, groups = [
                # TODO: members will be added later
            ])
        ]
    )
    ```

    Args:
      roles: a single role or a list of roles to assign. Required.
      groups: a single group name or a list of groups to assign the role to.
      users: a single user email or a list of emails to assign the role to.
      projects: a single LUCI project name or a list of project names to assign
        the role to.

    Returns:
      acl.entry object, should be treated as opaque.
    """
    if __native__.ctor(roles) == _role_ctor:
        roles = [roles]
    elif roles != None and type(roles) != "list":
        validate.struct("roles", roles, _role_ctor)

    if type(groups) == "string":
        groups = [groups]
    elif groups != None and type(groups) != "list":
        validate.string("groups", groups)

    if type(users) == "string":
        users = [users]
    elif users != None and type(users) != "list":
        validate.string("users", users)

    if type(projects) == "string":
        projects = [projects]
    elif projects != None and type(projects) != "list":
        validate.string("projects", projects)

    roles = validate.list("roles", roles, required = True)
    groups = validate.list("groups", groups)
    users = validate.list("users", users)
    projects = validate.list("projects", projects)

    for r in roles:
        validate.struct("roles", r, _role_ctor)
    for g in groups:
        validate.string("groups", g)
    for u in users:
        validate.string("users", u)
    for p in projects:
        validate.string("projects", p)

    # Some ACLs (e.g. LogDog) can be formulated only in terms of groups,
    # check this.
    for r in roles:
        if r.groups_only and (users or projects):
            fail("role %s can be assigned only to groups" % r.name)

    return _entry_ctor(
        roles = roles,
        groups = groups,
        users = users,
        projects = projects,
    )

def _validate_acls(
        acls,
        *,
        project_level = False,
        allowed_roles = None):
    """Validates the given list of acl.entry structs.

    Checks that project level roles are set only on the project level.

    Args:
      acls: an iterable of acl.entry structs to validate, or None.
      project_level: True to accept project_level_only=True roles.
      allowed_roles: an optional whitelist of roles to accept.

    Returns:
      A list of validated acl.entry structs or [], never None.
    """
    acls = validate.list("acls", acls)
    for e in acls:
        validate.struct("acls", e, _entry_ctor)
        for r in e.roles:
            if r.project_level_only and not project_level:
                fail('bad "acls": role %s can only be set at the project level' % r.name)
            if allowed_roles and r not in allowed_roles:
                fail('bad "acls": role %s is not allowed in this context' % r.name)
    return acls

def _normalize_acls(acls):
    """Expands, dedups and sorts ACLs from the given list of acl.entry structs.

    Expands plural 'roles', 'groups', 'users' and 'projects' fields in acl.entry
    into multiple acl.elementary structs: elementary pairs of (role, principal),
    where principal is either a user, a group or a project.

    Args:
      acls: an iterable of acl.entry structs to expand, assumed to be validated.

    Returns:
      A sorted deduped list of acl.elementary structs.
    """
    out = []
    for e in (acls or []):
        for r in e.roles:
            for u in e.users:
                out.append(_elementary_ctor(role = r, user = u, group = None, project = None))
            for g in e.groups:
                out.append(_elementary_ctor(role = r, user = None, group = g, project = None))
            for p in e.projects:
                out.append(_elementary_ctor(role = r, user = None, group = None, project = p))
    return sorted(set(out), key = _sort_key)

def _sort_key(e):
    """acl.elementary -> tuple to sort it by."""
    if e.user:
        order, ident = 0, e.user
    elif e.group:
        order, ident = 1, e.group
    elif e.project:
        order, ident = 2, e.project
    else:
        fail("impossible")
    return (e.role.name, order, ident)

def _binding_dicts(acls):
    """Takes a list of validated acl.entry structs and returns a list of dicts.

    Each dict contains keyword arguments for a luci.binding(...) rule. Together
    they represent the same ACL entries as `acls`.
    """
    per_role = {}  # role -> {roles: [role], groups: [], users: [], projects: []}.
    for e in _normalize_acls(acls):
        role = e.role.realms_role
        if not role:
            continue

        binding = per_role.get(role)
        if not binding:
            binding = {
                "roles": [role],
                "groups": [],
                "users": [],
                "projects": [],
            }
            per_role[role] = binding

        # `e` is acl.elementary which is a "union" struct: one and only one field is
        # set.
        if e.user:
            binding["users"].append(e.user)
        elif e.group:
            binding["groups"].append(e.group)
        elif e.project:
            binding["projects"].append(e.project)

    return per_role.values()

################################################################################

acl = struct(
    entry = _entry,

    # Note: the information in the comments is extracted by the documentation
    # generator. That's the reason there's a bit of repetition here.

    # Reading contents of project configs through LUCI Config API/UI.
    #
    # DocTags:
    #   project_level_only.
    PROJECT_CONFIGS_READER = _role(
        "PROJECT_CONFIGS_READER",
        realms_role = "role/configs.reader",
        project_level_only = True,
    ),

    # Reading logs under project's logdog prefix.
    #
    # DocTags:
    #   project_level_only, groups_only.
    LOGDOG_READER = _role(
        "LOGDOG_READER",
        realms_role = "role/logdog.reader",
        project_level_only = True,
        groups_only = True,
    ),

    # Writing logs under project's logdog prefix.
    #
    # DocTags:
    #   project_level_only, groups_only.
    LOGDOG_WRITER = _role(
        "LOGDOG_WRITER",
        realms_role = "role/logdog.writer",
        project_level_only = True,
        groups_only = True,
    ),

    # Fetching info about a build, searching for builds in a bucket.
    BUILDBUCKET_READER = _role(
        "BUILDBUCKET_READER",
        realms_role = "role/buildbucket.reader",
    ),
    # Same as `BUILDBUCKET_READER` + scheduling and canceling builds.
    BUILDBUCKET_TRIGGERER = _role(
        "BUILDBUCKET_TRIGGERER",
        realms_role = "role/buildbucket.triggerer",
    ),
    # Full access to the bucket (should be used rarely).
    BUILDBUCKET_OWNER = _role(
        "BUILDBUCKET_OWNER",
        realms_role = "role/buildbucket.owner",
    ),

    # Viewing Scheduler jobs, invocations and their debug logs.
    SCHEDULER_READER = _role(
        "SCHEDULER_READER",
        realms_role = "role/scheduler.reader",
    ),
    # Same as `SCHEDULER_READER` + ability to trigger jobs.
    SCHEDULER_TRIGGERER = _role(
        "SCHEDULER_TRIGGERER",
        realms_role = "role/scheduler.triggerer",
    ),
    # Full access to Scheduler jobs, including ability to abort them.
    SCHEDULER_OWNER = _role(
        "SCHEDULER_OWNER",
        realms_role = "role/scheduler.owner",
    ),

    # Committing approved CLs via CQ.
    #
    # DocTags:
    #  cq_role, groups_only.
    CQ_COMMITTER = _role(
        "CQ_COMMITTER",
        groups_only = True,
        realms_role = "role/cq.committer",
    ),

    # Executing presubmit tests for CLs via CQ.
    #
    # DocTags:
    #  cq_role, groups_only.
    CQ_DRY_RUNNER = _role(
        "CQ_DRY_RUNNER",
        groups_only = True,
        realms_role = "role/cq.dryRunner",
    ),

    # Having LUCI CV run tryjobs (e.g. static analyzers) on new patchset upload.
    #
    # DocTags:
    #  cq_role, groups_only.
    CQ_NEW_PATCHSET_RUN_TRIGGERER = _role(
        "CQ_NEW_PATCHSET_RUN_TRIGGERER",
        groups_only = True,
        realms_role = None,
    ),
)

aclimpl = struct(
    validate_acls = _validate_acls,
    normalize_acls = _normalize_acls,
    binding_dicts = _binding_dicts,
)
