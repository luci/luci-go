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

load('@stdlib//internal/luci/lib/validate.star', 'validate')


# TODO(vadimsh): Add support for 'anonymous' when/if needed.
# TODO(vadimsh): Check groups and users strings against regexps in acl.entry().


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
_role_ctor = genstruct('acl.role')


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
_entry_ctor = genstruct('acl.entry')


# A constructor for acl.elementary structs.
#
# This is conceptually a sum type: (Role, User | Group). For convenience it is
# represented as a triple where either 'user' or 'group' is set, but not both.
#
# Fields:
#   role: an acl.role, always set.
#   user: an user email.
#   group: a group name.
_elementary_ctor = genstruct('acl.elementary')


def _role(name, project_level_only=False, groups_only=False):
  """Defines a role.

  Internal API. Only predefined roles are available publicly, see the bottom of
  this file.

  Args:
    name: string name of the role.
    project_level_only: True if it can be used only in project(...) ACLs.
    groups_only: True if role supports only group-based ACL (not user-based).

  Returns:
    acl.role struct.
  """
  return _role_ctor(
      name = name,
      project_level_only = project_level_only,
      groups_only = groups_only,
  )


def _entry(roles, groups=None, users=None):
  """An ACL entry: assigns given role (or roles) to given individuals or groups.

  Specifying an empty ACL entry is allowed. It is ignored everywhere. Useful for
  things like:

      core.project(
          acl = [
              acl.entry(acl.PROJECT_CONFIGS_READER, groups = [
                  # TODO: fill me in
              ])
          ]
      )

  Args:
    roles: a single role (as acl.role) or a list of roles to assign.
    groups: a single group name or a list of groups to assign the role to.
    users: a single user email or a list of emails to assign the role to.

  Returns:
    acl.entry struct, consider it opaque.
  """
  if ctor(roles) == _role_ctor:
    roles = [roles]
  elif roles != None and type(roles) != 'list':
    validate.struct('roles', roles, _role_ctor)

  if type(groups) == 'string':
    groups = [groups]
  elif groups != None and type(groups) != 'list':
    validate.string('groups', groups)

  if type(users) == 'string':
    users = [users]
  elif users != None and type(users) != 'list':
    validate.string('users', users)

  roles = validate.list('roles', roles, required=True)
  groups = validate.list('groups', groups)
  users = validate.list('users', users)

  for r in roles:
    validate.struct('roles', r, _role_ctor)
  for g in groups:
    validate.string('groups', g)
  for u in users:
    validate.string('users', u)

  # Some ACLs (e.g. LogDog) can be formulated only in terms of groups,
  # check this.
  for r in roles:
    if r.groups_only and users:
      fail('role %s can be assigned only to groups, not individual users' % r.name)

  return _entry_ctor(
      roles = roles,
      groups = groups,
      users = users,
  )


def _validate_acls(acls, project_level=False):
  """Validates the given list of acl.entry structs.

  Checks that project level roles are set only on the project level.

  Args:
    acls: an iterable of acl.entry structs to validate, or None.
    project_level: True to accept project_level_only=True roles.

  Returns:
    A list of validated acl.entry structs or [], never None.
  """
  acls = validate.list('acls', acls)
  for e in acls:
    validate.struct('acls', e, _entry_ctor)
    for r in e.roles:
      if r.project_level_only and not project_level:
        fail('bad "acls": role %s can only be set at the project level' % r.name)
  return acls


def _normalize_acls(acls):
  """Expands, dedups and sorts ACLs from the given list of acl.entry structs.

  Expands plural 'roles', 'groups' and 'users' fields in acl.entry into multiple
  acl.elementary structs: elementary pairs of (role, principal), where principal
  is either a user or a group.

  Args:
    acls: an iterable of acl.entry structs to expand, assumed to be validated.

  Returns:
    A sorted deduped list of acl.elementary structs.
  """
  out = []
  for e in acls:
    for r in e.roles:
      for u in e.users:
        out.append(_elementary_ctor(role=r, user=u, group=None))
      for g in e.groups:
        out.append(_elementary_ctor(role=r, user=None, group=g))
  return sorted(set(out), key=_sort_key)


def _sort_key(e):
  """acl.elementary -> tuple to sort it by."""
  return (e.role.name, 'u:' + e.user if e.user else 'g:' + e.group)


################################################################################


# Helper to avoid retyping role names.
def _roles_dict(l):
  return {r.name: r for r in l}


# Public API exposed to end-users and to other LUCI modules.
acl = struct(
    entry = _entry,

    # All predefined roles.
    **_roles_dict([
        # Reading contents of project configs through LUCI Config API/UI.
        _role('PROJECT_CONFIGS_READER', project_level_only=True),

        # Reading logs under project's logdog prefix.
        _role('LOGDOG_READER', project_level_only=True, groups_only=True),
        # Writing logs under project's logdog prefix.
        _role('LOGDOG_WRITER', project_level_only=True, groups_only=True),

        # Fetching info about a build, searching for builds in a bucket.
        _role('BUILDBUCKET_READER'),
        # Same as BUILDBUCKET_READER + scheduling and canceling builds.
        _role('BUILDBUCKET_SCHEDULER'),
        # Have full access to the bucket (should be used rarely).
        _role('BUILDBUCKET_WRITER'),
    ])
)


# Additional internal API used by other LUCI modules.
aclimpl = struct(
    validate_acls = _validate_acls,
    normalize_acls = _normalize_acls,
)
