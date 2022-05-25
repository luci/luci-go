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

load("@stdlib//internal/luci/lib/realms.star", "realms")
load("@stdlib//internal/lucicfg.star", "lucicfg")

def _binding(
        ctx,  # @unused
        *,
        realm = None,
        roles = None,
        groups = None,
        users = None,
        projects = None,
        conditions = None):
    """Binding assigns roles in a realm to individuals, groups or LUCI projects.

    A role can either be predefined (if its name starts with `role/`) or custom
    (if its name starts with `customRole/`).

    Predefined roles are declared in the LUCI deployment configs, see **TODO**
    for the up-to-date list of available predefined roles and their meaning.

    Custom roles are defined in the project configs via luci.custom_role(...).
    They can be used if none of the predefined roles represent the desired set
    of permissions.

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
      conditions: a list of conditions (ANDed together) that define when this
        binding is active. Currently only a list of luci.restrict_attribute(...)
        conditions is supported. See luci.restrict_attribute(...) for more
        details. This is an experimental feature.
    """
    return realms.binding(
        impl = realms.default_impl,
        realm = realm,
        roles = roles,
        groups = groups,
        users = users,
        projects = projects,
        conditions = conditions,
    )

binding = lucicfg.rule(impl = _binding)
