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

"""Definitions imported by all other luci/**/*.star modules.

Should not import other LUCI modules to avoid dependency cycles.
"""

load('@stdlib//internal/graph.star', 'graph')


# Node dependencies (parent -> child):
#   core.project: root
#   core.project -> core.logdog
#   core.project -> [core.bucket]
#   core.project -> [core.builder_group]
#   core.bucket -> [core.builder]
#   core.builder with global key -> [core.builder with bucket-scoped keys]
#   core.builder_group -> [core.builder (either global or bucket-scoped key)]


# Kinds is a enum-like struct with node kinds of various LUCI config nodes.
kinds = struct(
    PROJECT = 'core.project',
    LOGDOG = 'core.logdog',
    BUCKET = 'core.bucket',
    BUILDER = 'core.builder',
    BUILDER_GROUP = 'core.builder_group',
)


def _builder_key(name):
  """Returns either a bucket-scoped or a global builder key.

  Args:
    name: either "<bucket>/<name>" or just "<name>".
  """
  chunks = name.split('/', 1)
  if len(chunks) == 1:
    return graph.key(kinds.BUILDER, chunks[0])
  return graph.key(kinds.BUCKET, chunks[0], kinds.BUILDER, chunks[1])


# Keys is a collection of key constructors for various LUCI config nodes.
keys = struct(
    project = lambda: graph.key(kinds.PROJECT, '...'),  # singleton
    logdog = lambda: graph.key(kinds.LOGDOG, '...'),  # singleton
    bucket = lambda name: graph.key(kinds.BUCKET, name),
    builder = _builder_key,
    builder_group = lambda name: graph.key(kinds.BUILDER_GROUP, name),
)
