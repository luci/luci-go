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
load('@stdlib//internal/luci/common.star', 'builder_ref', 'keys', 'triggerer')
load('@stdlib//internal/luci/lib/validate.star', 'validate')


# TODO(vadimsh): Add actual implementation. For now we just setup general
# structure of nodes and their relations.


def builder(name, bucket, triggers=None, triggered_by=None):
  """Defines a generic builder.

  Args:
    name: name of the builder, to refer to it from other rules.
    bucket: name of the bucket the builder belongs to.
    triggers: names of builders it triggers.
    triggered_by: names of builders or pollers it is triggered by.
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
  # name and via a global (perhaps ambiguous) name. Ambiguity is checked during
  # the graph traversal via builder_ref.follow.
  builder_ref_key = builder_ref.add('%s/%s' % (bucket, name), builder_key)
  builder_ref.add(name, builder_key)

  # Setup nodes that indicates this builder can be referenced in 'triggered_by'
  # relations (either via its bucket-scoped name or via its global name).
  triggerer_key = triggerer.add('%s/%s' % (bucket, name), builder_key)
  triggerer.add(name, builder_key)

  # Link to builders triggered by this builder.
  for t in validate.list('triggers', triggers):
    graph.add_edge(
        parent = triggerer_key,
        child = keys.builder_ref(validate.string('triggers', t)),
        title = 'triggers',
    )

  # And link to nodes this builder is triggered by.
  for t in validate.list('triggered_by', triggered_by):
    graph.add_edge(
        parent = keys.triggerer(t),
        child = builder_ref_key,
        title = 'triggered_by',
    )
