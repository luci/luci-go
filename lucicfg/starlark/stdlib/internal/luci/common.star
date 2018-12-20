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


def _bucket_scoped(kind, name):
  """Returns either a bucket-scoped or a global key of the given kind.

  Args:
    kind: kind of the key.
    name: either "<bucket>/<name>" or just "<name>".
  """
  chunks = name.split('/', 1)
  if len(chunks) == 1:
    return graph.key(kind, chunks[0])
  return graph.key(kinds.BUCKET, chunks[0], kind, chunks[1])


def _declare_alias(key):
  """Declares an alias node in the global namespace.

  Given a key [(BUCKET, b), (KIND, id)], adds a node with the key [(KIND, id)],
  and links it back to the original node.

  This allows to later discover all nodes with given (KIND, id) pair, without
  knowing their full keys.

  Args:
    key: graph.key to setup an alias for.
  """
  alias_key = graph.key(key.kind, key.id)
  graph.add_node(alias_key, idempotent=True)
  graph.add_edge(alias_key, key)


def _resolve_alias(node):
  """Given an alias node, finds the single original node.

  Errors if there are multiple nodes with the same alias.

  Args:
    node: either a concrete node, or a global alias node.
  """
  if node.key.container != None:
    return node  # already resolved
  variants = graph.children(node.key, node.key.kind)
  if not variants:
    fail('alias.resolve: unexpectedly 0 matching nodes')
  elif len(variants) != 1:
    fail('ambiguous reference %s, possible variants:\n  %s' % (
        node,
        '\n  '.join([
            '%s/%s' % (b.key.container.id, b.key.id) for b in variants
        ]),
    ))
  return variants[0]


# Utilities for working with alias nodes.
alias = struct(
    declare = _declare_alias,
    resolve = _resolve_alias,
)


# Kinds is a enum-like struct with node kinds of various LUCI config nodes.
kinds = struct(
    PROJECT = 'core.project',
    LOGDOG = 'core.logdog',
    BUCKET = 'core.bucket',
    BUILDER = 'core.builder',
    BUILDER_GROUP = 'core.builder_group',
)


# Keys is a collection of key constructors for various LUCI config nodes.
keys = struct(
    project = lambda: graph.key(kinds.PROJECT, '...'),  # singleton
    logdog = lambda: graph.key(kinds.LOGDOG, '...'),  # singleton
    bucket = lambda name: graph.key(kinds.BUCKET, name),
    builder = lambda name: _bucket_scoped(kinds.BUILDER, name),
    builder_group = lambda name: graph.key(kinds.BUILDER_GROUP, name),
)
