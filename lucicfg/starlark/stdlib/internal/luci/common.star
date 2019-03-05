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
load('@stdlib//internal/sequence.star', 'sequence')
load('@stdlib//internal/validate.star', 'validate')


# Node edges (parent -> child):
#   luci.project: root
#   luci.project -> luci.logdog
#   luci.project -> luci.milo
#   luci.project -> luci.cq
#   luci.project -> [luci.bucket]
#   luci.project -> [luci.milo_view]
#   luci.project -> [luci.cq_group]
#   luci.bucket -> [luci.builder]
#   luci.bucket -> [luci.gitiles_poller]
#   luci.builder_ref -> luci.builder
#   luci.builder -> [luci.triggerer]
#   luci.builder -> luci.recipe
#   luci.gitiles_poller -> [luci.triggerer]
#   luci.triggerer -> [luci.builder_ref]
#   luci.milo_entries_root -> [luci.list_view_entry]
#   luci.milo_entries_root -> [luci.console_view_entry]
#   luci.milo_view -> luci.list_view
#   luci.list_view -> [luci.list_view_entry]
#   luci.list_view_entry -> list.builder_ref
#   luci.milo_view -> luci.console_view
#   luci.console_view -> [luci.console_view_entry]
#   luci.console_view_entry -> list.builder_ref
#   luci.cq_verifiers_root -> [luci.cq_tryjob_verifier]
#   luci.cq_group -> [luci.cq_tryjob_verifier]


def _global_key(kind, attr, ref):
  """Returns a key either by grabbing it from the keyset or constructing.

  Args:
    kind: kind of the key to return;
    attr: field name that supplies 'ref', for error messages.
    ref: either a keyset or a string "<name>".

  Returns:
    graph.key of the requested kind.
  """
  if graph.is_keyset(ref):
    return ref.get(kind)
  return graph.key(kind, validate.string(attr, ref))


def _bucket_scoped_key(kind, attr, ref):
  """Returns either a bucket-scoped or a global key of the given kind.

  Bucket-scoped keys have (BUCKET, <name>) as the first component of the key.

  Args:
    kind: kind of the key to return.
    attr: field name that supplies 'ref', for error messages.
    ref: either a keyset, a string "<bucket>/<name>" or just "<name>".

  Returns:
    graph.key of the requested kind.
  """
  if graph.is_keyset(ref):
    return ref.get(kind)
  chunks = validate.string(attr, ref).split('/', 1)
  if len(chunks) == 1:
    return graph.key(kind, chunks[0])
  return graph.key(kinds.BUCKET, chunks[0], kind, chunks[1])


# Kinds is a enum-like struct with node kinds of various LUCI config nodes.
kinds = struct(
    # Publicly declarable nodes.
    PROJECT = 'luci.project',
    LOGDOG = 'luci.logdog',
    BUCKET = 'luci.bucket',
    RECIPE = 'luci.recipe',
    BUILDER = 'luci.builder',
    GITILES_POLLER = 'luci.gitiles_poller',
    MILO = 'luci.milo',
    LIST_VIEW = 'luci.list_view',
    LIST_VIEW_ENTRY = 'luci.list_view_entry',
    CONSOLE_VIEW = 'luci.console_view',
    CONSOLE_VIEW_ENTRY = 'luci.console_view_entry',
    CQ = 'luci.cq',
    CQ_GROUP = 'luci.cq_group',
    CQ_TRYJOB_VERIFIER = 'luci.cq_tryjob_verifier',

    # Internal nodes (declared internally as dependency of other nodes).
    BUILDER_REF = 'luci.builder_ref',
    TRIGGERER = 'luci.triggerer',
    MILO_ENTRIES_ROOT = 'luci.milo_entries_root',
    MILO_VIEW = 'luci.milo_view',
    CQ_VERIFIERS_ROOT = 'luci.cq_verifiers_root',
)


# Keys is a collection of key constructors for various LUCI config nodes.
keys = struct(
    # Publicly declarable nodes.
    project = lambda: graph.key(kinds.PROJECT, '...'),  # singleton
    logdog = lambda: graph.key(kinds.LOGDOG, '...'),  # singleton
    bucket = lambda ref: _global_key(kinds.BUCKET, 'bucket', ref),
    recipe = lambda ref: _global_key(kinds.RECIPE, 'recipe', ref),

    # TODO(vadimsh): Make them accept keysets if necessary. These currently
    # require strings, not keysets. They are currently not directly used by
    # anything, only through 'builder_ref' and 'triggerer' nodes. As such, they
    # are never consumed via keysets.
    builder = lambda bucket, name: graph.key(kinds.BUCKET, bucket, kinds.BUILDER, name),
    gitiles_poller = lambda bucket, name: graph.key(kinds.BUCKET, bucket, kinds.GITILES_POLLER, name),

    milo = lambda: graph.key(kinds.MILO, '...'),  # singleton
    list_view = lambda ref: _global_key(kinds.LIST_VIEW, 'list_view', ref),
    console_view = lambda ref: _global_key(kinds.CONSOLE_VIEW, 'console_view', ref),

    cq = lambda: graph.key(kinds.CQ, '...'),  # singleton
    cq_group = lambda ref: _global_key(kinds.CQ_GROUP, 'cq_group', ref),

    # Internal nodes (declared internally as dependency of other nodes).
    builder_ref = lambda ref, attr='triggers': _bucket_scoped_key(kinds.BUILDER_REF, attr, ref),
    triggerer = lambda ref, attr='triggered_by': _bucket_scoped_key(kinds.TRIGGERER, attr, ref),
    milo_entries_root = lambda: graph.key(kinds.MILO_ENTRIES_ROOT, '...'),
    milo_view = lambda name: graph.key(kinds.MILO_VIEW, name),
    cq_verifiers_root = lambda: graph.key(kinds.CQ_VERIFIERS_ROOT, '...'),

    # Generates a key of the given kind and name within some auto-generated
    # unique container key.
    #
    # Used with LIST_VIEW_ENTRY, CONSOLE_VIEW_ENTRY and CQ_TRYJOB_VERIFIER
    # helper nodes. They don't really represent any "external" entities, and
    # their names don't really matter, other than for error messages.
    #
    # Note that IDs of keys whose kind stars with '_' (like '_UNIQUE' here),
    # are skipped when printing the key in error messages. Thus the meaningless
    # confusing auto-generated part of the key isn't showing up in error
    # messages.
    unique = lambda kind, name: graph.key('_UNIQUE', str(sequence.next(kind)), kind, name),
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


################################################################################
## triggerer implementation.


def _triggerer_add(owner):
  """Adds two 'triggerer' nodes ("<bucket>/<name>" and "<name>") that have
  'owner' as a parent.

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
    owner: a graph.key to setup triggerers for.

  Returns:
    graph.key of added bucket-scoped TRIGGERER node ("<bucket>/<name>" one).
  """
  if not owner.container or owner.container.kind != kinds.BUCKET:
    fail('got %s, expecting a bucket-scoped key' % (owner,))

  bucket = owner.container.id  # name of the bucket, as string
  name = owner.id              # name of the builder or poller, as string

  short = keys.triggerer(name)
  graph.add_node(short, idempotent=True)  # there may be such node already
  graph.add_edge(owner, short)

  full = keys.triggerer("%s/%s" % (bucket, name))
  graph.add_node(full)
  graph.add_edge(owner, full)

  return full


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
    List of graph.node of BUILDER kind, sorted by keys.
  """
  out = set()

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
      else:
        out = out.union([builder])

  return graph.sorted_nodes(out)


triggerer = struct(
    add = _triggerer_add,
    targets = _triggerer_targets,
)



################################################################################
## Helpers for defining milo views (lists or consoles).


def _view_add_view(key, entry_kind, entry_ctor, entries, props):
  """Adds a *_view node, ensuring it doesn't clash with some other view.

  Args:
    key: a key of *_view node to add.
    entry_kind: a kind of corresponding *_view_entry node.
    entry_ctor: a corresponding *_view_entry rule.
    entries: a list of *_view_entry, builder refs or dict.
    props: properties for the added node.

  Returns:
    A keyset with added keys.
  """
  graph.add_node(key, props)

  # If there's some other view of different kind with the same name this will
  # cause an error.
  milo_view_key = keys.milo_view(key.id)
  graph.add_node(milo_view_key)

  # project -> milo_view -> *_view. Indirection through milo_view allows to
  # treat different kinds of views uniformly.
  graph.add_edge(keys.project(), milo_view_key)
  graph.add_edge(milo_view_key, key)

  # 'entry' is either a *_view_entry keyset, a builder_ref (perhaps given
  # as a string) or a dict with parameters to *_view_entry rules. In the latter
  # two cases, we add a *_view_entry for it automatically. This allows to
  # somewhat reduce verbosity of list definitions.
  for entry in validate.list('entries', entries):
    if type(entry) == 'dict':
      entry = entry_ctor(**entry)
    elif type(entry) == 'string' or (graph.is_keyset(entry) and entry.has(kinds.BUILDER_REF)):
      entry = entry_ctor(builder = entry)
    graph.add_edge(key, entry.get(entry_kind))

  return graph.keyset(key, milo_view_key)


def _view_add_entry(kind, view, builder, buildbot, props=None):
  """Adds *_view_entry node.

  Common implementation for list_view_node and console_view_node.

  Args:
    kind: a kind of the node to add (e.g. LIST_VIEW_ENTRY).
    view: a key of the parent *_view to add the entry to, if known.
    builder: a reference to builder (or None if unknown).
    buildbot: a string with a reference to equivalent buildbot builder.
    props: properties for the added node, mutated by adding 'buildbot' key.

  Returns:
    A keyset with the added key.
  """
  if builder != None:
    builder = keys.builder_ref(builder, attr='builder')
  buildbot = validate.string('buildbot', buildbot, required=False)

  if builder == None and buildbot == None:
    fail('either "builder" or "buildbot" are required')

  props = props or {}
  props['buildbot'] = buildbot

  # Note: name of this node is important only for error messages. It isn't
  # showing up in any generated files and by construction it can't accidentally
  # collide with some other name.
  key = keys.unique(kind, builder.id if builder else buildbot)
  graph.add_node(key, props)
  if view != None:
    graph.add_edge(parent=view, child=key)
  if builder != None:
    graph.add_edge(parent=key, child=builder)

  # This is used to detect *_view_entry nodes that aren't connected to any
  # *_view. Such orphan nodes aren't allowed.
  graph.add_node(keys.milo_entries_root(), idempotent=True)
  graph.add_edge(parent=keys.milo_entries_root(), child=key)

  return graph.keyset(key)


view = struct(
    add_view = _view_add_view,
    add_entry = _view_add_entry,
)
