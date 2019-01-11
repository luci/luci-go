# Copyright 2019 The LUCI Authors.
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
load('@stdlib//internal/luci/common.star', 'keys', 'triggerer')
load('@stdlib//internal/luci/lib/validate.star', 'validate')


# TODO(vadimsh): Add actual implementation. For now we just setup general
# structure of nodes and their relations.


def gitiles_poller(
      name=None,
      bucket=None,
      triggers=None,
  ):
  """Defines a gitiles poller which can trigger builders on git commits.

  Args:
    name: name of the poller, to refer to it from other rules. Required.
    bucket: name of the bucket the poller belongs to. Required.
    triggers: names of builders it triggers.
  """
  name = validate.string('name', name)
  bucket = validate.string('bucket', bucket)

  # Node that carries the full definition of the poller.
  poller_key = keys.gitiles_poller(bucket, name)
  graph.add_node(poller_key, props = {
      'name': name,
      'bucket': bucket,
  })
  graph.add_edge(keys.bucket(bucket), poller_key)

  # Setup nodes that indicate this poller can be referenced in 'triggered_by'
  # relations (either via its bucket-scoped name or via its global name).
  triggerer_key = triggerer.add(poller_key)

  # Link to builders triggered by this builder.
  for t in validate.list('triggers', triggers):
    graph.add_edge(
        parent = triggerer_key,
        child = keys.builder_ref(validate.string('triggers', t)),
        title = 'triggers',
    )
