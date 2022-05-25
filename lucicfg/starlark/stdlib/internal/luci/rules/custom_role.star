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

"""Defines luci.custom_role(...) rule."""

load("@stdlib//internal/luci/lib/realms.star", "realms")
load("@stdlib//internal/lucicfg.star", "lucicfg")

def _custom_role(
        ctx,  # @unused
        *,
        name,
        extends = None,
        permissions = None):
    """Defines a custom role.

    It can be used in luci.binding(...) if predefined roles are too broad or do
    not map well to the desired set of permissions.

    Custom roles are scoped to the project (i.e. different projects may have
    identically named, but semantically different custom roles).

    Args:
      ctx: the implicit rule context, see lucicfg.rule(...).
      name: name of the custom role. Must start with `customRole/`. Required.
      extends: optional list of roles whose permissions will be included in this
        role. Each entry can either be a predefined role (if it is a string that
        starts with `role/`) or another custom role (if it is a string that
        starts with `customRole/` or a luci.custom_role(...) key).
      permissions: optional list of permissions to include in the role. Each
        permission is a symbol that has form `<service>.<subject>.<verb>`, which
        describes some elementary action (`<verb>`) that can be done to some
        category of resources (`<subject>`), managed by some particular kind of
        LUCI service (`<service>`). See **TODO** for the up-to-date list of
        available permissions and their meaning.
    """
    return realms.custom_role(
        impl = realms.default_impl,
        name = name,
        extends = extends,
        permissions = permissions,
    )

custom_role = lucicfg.rule(impl = _custom_role)
