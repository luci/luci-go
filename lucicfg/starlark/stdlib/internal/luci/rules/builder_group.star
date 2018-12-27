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
load('@stdlib//internal/luci/common.star', 'builder_ref', 'keys')
load('@stdlib//internal/luci/lib/validate.star', 'validate')


def builder_group(name, builders):
  """A named group of builders.

  Can be used to logically group builders together. A builder group can contain
  builders and other builder groups.

  Args:
    name: name of this group, must be unique.
    builders: list of names of included builders and builder groups.
  """
  # Add the builder group node itself. It has no parameters.
  key = keys.builder_group(validate.string('name', name))
  graph.add_node(key)

  # Include given builders and builder groups transitively though builder_refs
  # that point to them. We need this indirection because we don't know what
  # kind of node 'b' refers to.
  for b in builders:
    graph.add_edge(key, keys.builder_ref(b))

  # Allow this builder group to be referenced from other nodes (e.g. other
  # builder groups) via its unique global name.
  builder_ref.add(name, key)
