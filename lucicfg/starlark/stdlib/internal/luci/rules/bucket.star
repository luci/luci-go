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

load('@stdlib//internal/graph.star', 'graph')
load('@stdlib//internal/validate.star', 'validate')

load('@stdlib//internal/luci/common.star', 'keys')
load('@stdlib//internal/luci/lib/acl.star', 'aclimpl')


def bucket(*, name=None, acls=None):
  """Defines a bucket: a container for LUCI resources that share the same ACL.

  Args:
    name: name of the bucket, e.g. `ci` or `try`. Required.
    acls: list of acl.entry(...) objects.
  """
  name = validate.string('name', name)
  key = keys.bucket(name)
  graph.add_node(key, props = {
      'name': name,
      'acls': aclimpl.validate_acls(acls, project_level=False),
  })
  graph.add_edge(keys.project(), key)
  return graph.keyset(key)
