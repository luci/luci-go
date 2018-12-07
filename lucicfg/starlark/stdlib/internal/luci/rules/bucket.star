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
load('@stdlib//internal/luci/common.star', 'keys')
load('@stdlib//internal/luci/lib/validate.star', 'validate')


def bucket(name):
  """Defines a bucket: a container for LUCI resources that share the same ACL.

  Args:
    name: name of the bucket, e.g. 'ci' or 'try'.
  """
  name = validate.string('name', name)
  graph.add_node(keys.bucket(name), props = {
      'name': name,
  })
  graph.add_edge(keys.project(), keys.bucket(name))
