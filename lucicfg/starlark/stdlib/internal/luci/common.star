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

load('@stdlib//internal/error.star', 'error')
load('@stdlib//internal/graph.star', 'graph')


# Node edges (parent -> child):
#   core.project: root
#   core.project -> core.logdog
#   core.project -> [core.bucket]
#   core.bucket -> [core.builder]
#   core.builder_ref -> core.builder


def _bucket_scoped_key(kind, name):
  """Returns either a bucket-scoped or a global key of the given kind.

  Bucket-scoped keys have (BUCKET, <name>) as the first component of the key.

  Args:
    kind: kind of the key.
    name: either "<bucket>/<name>" or just "<name>".
  """
  chunks = name.split('/', 1)
  if len(chunks) == 1:
    return graph.key(kind, chunks[0])
  return graph.key(kinds.BUCKET, chunks[0], kind, chunks[1])


# Kinds is a enum-like struct with node kinds of various LUCI config nodes.
kinds = struct(
    # Publicly declarable nodes.
    PROJECT = 'core.project',
    LOGDOG = 'core.logdog',
    BUCKET = 'core.bucket',
    BUILDER = 'core.builder',

    # Internal nodes (declared internally as dependency of other nodes).
    BUILDER_REF = 'core.builder_ref',
)


# Keys is a collection of key constructors for various LUCI config nodes.
keys = struct(
    # Publicly declarable nodes.
    project = lambda: graph.key(kinds.PROJECT, '...'),  # singleton
    logdog = lambda: graph.key(kinds.LOGDOG, '...'),  # singleton
    bucket = lambda name: graph.key(kinds.BUCKET, name),
    builder = lambda bucket, name: graph.key(kinds.BUCKET, bucket, kinds.BUILDER, name),

    # Internal nodes.
    builder_ref = lambda name: _bucket_scoped_key(kinds.BUILDER_REF, name),
)


################################################################################
## builder_ref implementation.


def _builder_ref_add(target):
  """Adds two builder_ref nodes ("<bucket>/<name>" and "<name>") that have
  'target' as a child.

  Builder refs are named pointers to builders. Each builder has two such refs:
  a bucket-scoped one (for when the builder is referenced using its full name
  "<bucket>/<name>"), and a global one (when the builder is referenced just as
  "<name>").

  Global refs can have more than one child (which means there are multiple
  builders with the same name in different buckets). Such refs are reported as
  ambiguous by builder_ref.follow(...).

  Args:
    target: a graph.key (of BUILDER kind) to setup refs for.

  Returns:
    graph.key of added bucket-scoped BUILDER_REF node ("<bucket>/<name>" one).
  """
  if target.kind != kinds.BUILDER:
    fail('got %s, expecting a builder key' % (target,))

  bucket = target.container.id  # name of the bucket, as string
  builder = target.id           # name of the builder, as string

  short = keys.builder_ref(builder)
  graph.add_node(short, idempotent=True)  # there may be such node already
  graph.add_edge(short, target)

  full = keys.builder_ref("%s/%s" % (bucket, builder))
  graph.add_node(full)
  graph.add_edge(full, target)

  return full


def _builder_ref_follow(ref_node, context_node):
  """Given a BUILDER_REF node, returns a BUILDER graph.node the ref points to.

  Emits an error and returns None if the reference is ambiguous (i.e. 'ref_node'
  has more than one child). Such reference can't be used to refer to a single
  builder.

  Note that the emitted error doesn't stop the generation phase, but marks it
  as failed. This allows to collect more errors before giving up.

  Args:
    ref_node: graph.node with the ref.
    context_node: graph.node where this ref is used, for error messages.

  Returns:
    graph.node of BUILDER kind.
  """
  if ref_node.key.kind != kinds.BUILDER_REF:
    fail('%s is not builder_ref' % ref_node)

  # builder_ref nodes are always linked to something, see _builder_ref_add.
  variants = graph.children(ref_node.key)
  if not variants:
    fail('%s is unexpectedly unconnected' % ref_node)

  # No ambiguity.
  if len(variants) == 1:
    return variants[0]

  error(
      'ambiguous reference %r in %s, possible variants:\n  %s',
      ref_node.key.id, context_node, '\n  '.join([str(v) for v in variants]),
      trace=context_node.trace,
  )
  return None


# Additional API for dealing with builder_refs.
builder_ref = struct(
    add = _builder_ref_add,
    follow = _builder_ref_follow,
)
