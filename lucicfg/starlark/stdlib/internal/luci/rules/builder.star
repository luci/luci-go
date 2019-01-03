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


# TODO(vadimsh): Add actual implementation. For now we just setup general
# structure of nodes and their relations.


def builder(name, bucket):
  """Defines a generic builder.

  Args:
    name: name of the builder, to refer to it from other rules.
    bucket: name of the bucket the builder belongs to.
  """
  name = validate.string('name', name)
  bucket = validate.string('bucket', bucket)

  # Node that carries the full definition of the builder.
  builder_key = keys.builder(bucket, name)
  graph.add_node(builder_key, props = {
      'name': name,
      'bucket': bucket,
  })
  graph.add_edge(keys.bucket(bucket), builder_key)

  # Allow this builder to be referenced from other nodes via its bucket-scoped
  # name and via a global (perhaps ambiguous) name. See builder_ref.add(...).
  # Ambiguity is checked during the graph traversal via builder_ref.follow(...).
  builder_ref.add(builder_key)
