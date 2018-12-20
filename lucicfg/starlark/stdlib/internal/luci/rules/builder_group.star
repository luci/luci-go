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

# TODO(vadimsh): This is just an example of how to refer to builders. It will
# be removed once there's something real to use instead.

load('@stdlib//internal/graph.star', 'graph')
load('@stdlib//internal/luci/common.star', 'keys')


def builder_group(name, builders):
  """A named groups of builders.

  Args:
    name: name of this group.
    builders: list of builder names.
  """
  graph.add_node(keys.builder_group(name))
  graph.add_edge(keys.project(), keys.builder_group(name))
  for b in builders:
    graph.add_edge(keys.builder_group(name), keys.builder(b))
