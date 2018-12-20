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


def builder(name, bucket):
  """Defines a generic builder.

  Args:
    name: name of the builder, will show up in UI. Can be used to refer to this
        builder from other rules.
    bucket: name of the bucket the builder belongs to.
  """
  name = validate.string('name', name)
  bucket = validate.string('bucket', bucket)

  # Node that carries the full definition of the builder.
  builder_key = keys.builder('%s/%s' % (bucket, name))
  graph.add_node(builder_key, props = {
      'name': name,
      'bucket': bucket,
  })
  graph.add_edge(keys.bucket(bucket), builder_key)

  # An alias for this builder in the global namespace, so that builders can be
  # referred to simply by they name (without specifying a bucket). If there are
  # ambiguities, the node we add here will have multiple children. This is
  # checked during the generation phase in 'resolve_builder'.
  alias_key = keys.builder(name)
  graph.add_node(alias_key, idempotent=True)
  graph.add_edge(alias_key, builder_key)
