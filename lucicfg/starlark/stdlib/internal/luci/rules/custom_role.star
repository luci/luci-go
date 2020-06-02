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

load('@stdlib//internal/graph.star', 'graph')
load('@stdlib//internal/lucicfg.star', 'lucicfg')
load('@stdlib//internal/validate.star', 'validate')

load('@stdlib//internal/luci/common.star', 'keys', 'kinds')
load('@stdlib//internal/luci/lib/realms.star', 'realms')


def _custom_role(
      ctx,
      *,
      name,
      extends=None,
      permissions=None,
  ):
  """Defines a custom role.

  It can be used in luci.binding(...) if predefined roles are too broad or do
  not map well to the desired set of permissions.

  Custom roles are scoped to the project (i.e. different projects may have
  identically named, but semantically different custom roles).

  DocTags:
    Experimental.

  Args:
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
  realms.experiment.require()

  name = validate.string('name', name)
  if not name.startswith('customRole/'):
    fail('bad "name": %r must start with "customRole/"' % (name,))

  # Each element of `extends` is either a string 'role/...' or 'customRole/...',
  # or a reference to luci.custom_role(...) node. We link to all custom role
  # nodes to make sure they are defined and to check for cycles.
  extends_strs = []  # 'role/...' and 'customRole/...'
  parents_keys = []  # keys of custom roles only
  for r in validate.list('extends', extends):
    if graph.is_keyset(r) and r.has(kinds.CUSTOM_ROLE):
      key = r.get(kinds.CUSTOM_ROLE)
      extends_strs.append(key.id)  # grab its string name
      parents_keys.append(key)
    elif type(r) == 'string':
      if not r.startswith('role/') and not r.startswith('customRole/'):
        fail('bad "extends": %r should start with "role/" or "customeRole/"' % (r,))
      extends_strs.append(r)
      if r.startswith('customRole/'):
        parents_keys.append(keys.custom_role(r))
    else:
      fail('bad "extends": %r is not a string nor a luci.custom_role(...)' % (r,))

  key = keys.custom_role(name)
  graph.add_node(key, props={
      'name': name,
      'extends': sorted(set(extends_strs)),
      'permissions': sorted(set(validate.str_list('permissions', permissions))),
  }, idempotent=True)
  graph.add_edge(keys.project(), key)
  for parent in parents_keys:
    graph.add_edge(parent, key, title='extends')

  return graph.keyset(key)


custom_role = lucicfg.rule(impl = _custom_role)
