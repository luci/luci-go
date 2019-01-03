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


# Node dependencies (parent -> child):
#   core.project: root
#   core.project -> core.logdog
#   core.project -> [core.bucket]
#   core.bucket -> [core.builder]
#   core.bucket -> [core.gitiles_poller]
#   core.builder_ref -> core.builder
#   core.builder -> [core.triggerer]         # global and bucket-scoped
#   core.gitiles_poller -> [core.triggerer]  # global and bucket-scoped
#   core.triggerer -> [core.builder_ref]


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
    PROJECT = 'core.project',
    LOGDOG = 'core.logdog',
    BUCKET = 'core.bucket',
    BUILDER = 'core.builder',
    BUILDER_REF = 'core.builder_ref',  # internal
    TRIGGERER = 'core.triggerer',      # internal
    GITILES_POLLER = 'core.gitiles_poller',
)


# Keys is a collection of key constructors for various LUCI config nodes.
keys = struct(
    project = lambda: graph.key(kinds.PROJECT, '...'),  # singleton
    logdog = lambda: graph.key(kinds.LOGDOG, '...'),  # singleton
    bucket = lambda name: graph.key(kinds.BUCKET, name),
    builder = lambda name: _bucket_scoped_key(kinds.BUILDER, name),
    builder_ref = lambda name: _bucket_scoped_key(kinds.BUILDER_REF, name),
    triggerer = lambda name: _bucket_scoped_key(kinds.TRIGGERER, name),
    gitiles_poller = lambda name: _bucket_scoped_key(kinds.GITILES_POLLER, name),
)


################################################################################
## builder_ref implementation.


def _builder_ref_add(name, target):
  """Adds a builder_ref node that has 'target' (given via its key) as a child.

  Builder refs are named pointers to builders. Each builder has two such refs:
  a bucket-scoped one (for when the builder is referenced using its full name
  "<bucket>/<name>"), and a global one (when the builder is referenced just as
  "<name>").

  Global refs can have more than one child (which means there are multiple
  builders with same name in different buckets). Such refs are reported as
  ambiguous by builder_ref.follow.

  Args:
    name: name of the builder_ref node (either bucket-scoped or global).
    target: a graph.key (of BUILDER kind) the ref points to.

  Returns:
    graph.key of added BUILDER_REF node.
  """
  if target.kind != kinds.BUILDER:
    fail('got %s, expecting a builder key' % (target,))
  k = keys.builder_ref(name)
  graph.add_node(k, idempotent=True)
  graph.add_edge(k, target)
  return k


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


################################################################################
## triggerer implementation.


def _triggerer_add(name, owner):
  """Adds a triggerer node that has 'owner' (given via its key) as a parent.

  Triggerer nodes are essentially nothing more than a way to associate a node
  of arbitrary kind ('owner') to a list of builder_ref's of builders it
  triggers (children of added 'triggerer' node).

  We need this indirection to make 'triggered_by' relation work: when a builder
  'B' is triggered by something named 'T', we don't know whether 'T' is another
  builder or a gitiles poller ('T' may not even be defined yet). So instead
  all things that can trigger builders have an associated 'triggerer' node and
  'T' names such a node.

  To allow omitting bucket name when it is not important, each triggering entity
  actually defines two 'triggerer' nodes: a bucket-scoped one and a global one.
  During the graph traversal phase, children of both nodes are merged.

  If a globally named 'triggerer' node has more than one parent, it means there
  are multiple things in different buckets that have the same name. Using such
  references in 'triggered_by' relation is ambiguous. This situation is detected
  during the graph traversal phase, see triggerer.targets(...).

  Args:
    name: name of the triggerer node (either bucket-scoped or global)
    owner: a graph.key of node that owns this triggerer.

  Returns:
    graph.key of added TRIGGERER node.
  """
  k = keys.triggerer(name)
  graph.add_node(k, idempotent=True)
  graph.add_edge(owner, k)
  return k


def _triggerer_targets(root):
  """Collects all BUILDER nodes triggered by the given node.

  Enumerates all TRIGGERER children of 'root', and collects all BUILDER nodes
  they refer to, following BUILDER_REF references.

  Various ambiguities are reported as errors (which marks the generation phase
  as failed). Corresponding nodes are skipped, to collect as many errors as
  possible before giving up.

  Args:
    root: a graph.node that represents the triggering entity: something that has
        a triggerer as a child.

  Returns:
    List of graph.node of BUILDER kind (may be empty).
  """
  out = []
  seen = set()

  for t in graph.children(root.key, kinds.TRIGGERER):
    parents = graph.parents(t.key)

    for ref in graph.children(t.key, kinds.BUILDER_REF):
      # Resolve builder_ref to a concrete builder. This may return None if the
      # ref is ambiguous.
      builder = builder_ref.follow(ref, root)
      if not builder:
        continue

      # If 't' has multiple parents, it can't be used in 'triggered_by'
      # relations, since it is ambiguous. Report this situation.
      if len(parents) != 1:
        error(
            'ambiguous reference %r in %s, possible variants:\n  %s',
            t.key.id, builder, '\n  '.join([str(v) for v in parents]),
            trace=builder.trace,
        )
      elif builder not in seen:
        out.append(builder)
        seen = seen.union([builder])

  # TODO(vadimsh): Sort this. Right now 'out' is a concatenation of two sorted
  # lists. This leaks the existence of two internal 'triggerer' nodes to the
  # outside world.
  return out


triggerer = struct(
    add = _triggerer_add,
    targets = _triggerer_targets,
)
