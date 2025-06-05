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

load("@stdlib//internal/error.star", "error")
load("@stdlib//internal/graph.star", "graph")
load("@stdlib//internal/sequence.star", "sequence")
load("@stdlib//internal/validate.star", "validate")

# Node edges (parent -> child):
#   luci.project: root
#   luci.project -> [luci.realm]
#   luci.project -> [luci.custom_role]
#   luci.realm -> [luci.realm]
#   luci.custom_role -> [luci.custom_role]
#   luci.bindings_root -> [luci.binding]
#   luci.realm -> [luci.binding]
#   luci.custom_role -> [luci.binding]
#   luci.project -> luci.logdog
#   luci.project -> luci.milo
#   luci.project -> luci.cq
#   luci.project -> [luci.bucket]
#   luci.project -> [luci.milo_view]
#   luci.project -> [luci.cq_group]
#   luci.project -> [luci.notifiable]
#   luci.project -> [luci.notifier_template]
#   luci.project -> [luci.buildbucket_notification_topic]
#   luci.project -> [luci.builder_health_notifier]
#   luci.bucket -> [luci.builder]
#   luci.bucket -> [luci.gitiles_poller]
#   luci.bucket -> [luci.bucket_constraints]
#   luci.bucket_constraints_root -> [luci.bucket_constraints]
#   luci.bucket -> [luci.dynamic_builder_template]
#   luci.builder_ref -> luci.builder
#   luci.builder -> [luci.triggerer]
#   luci.builder -> luci.executable
#   luci.builder -> luci.task_backend
#   luci.dynamic_builder_template -> luci.executable
#   luci.gitiles_poller -> [luci.triggerer]
#   luci.triggerer -> [luci.builder_ref]
#   luci.milo_entries_root -> [luci.list_view_entry]
#   luci.milo_entries_root -> [luci.console_view_entry]
#   luci.milo_view -> luci.list_view
#   luci.list_view -> [luci.list_view_entry]
#   luci.list_view_entry -> list.builder_ref
#   luci.milo_view -> luci.console_view
#   luci.milo_view -> luci.external_console_view
#   luci.console_view -> [luci.console_view_entry]
#   luci.console_view_entry -> list.builder_ref
#   luci.cq_verifiers_root -> [luci.cq_tryjob_verifier]
#   luci.cq_group -> [luci.cq_tryjob_verifier]
#   luci.cq_tryjob_verifier -> luci.builder_ref
#   luci.cq_tryjob_verifier -> luci.cq_equivalent_builder
#   luci.cq_equivalent_builder -> luci.builder_ref
#   luci.notifiable -> [luci.builder_ref]
#   luci.notifiable -> luci.notifier_template

def _namespaced_key(*pairs):
    """Returns a key namespaced to the current project.

    Args:
      *pairs: key path inside the current project namespace.

    Returns:
      graph.key.
    """
    return graph.key(kinds.LUCI_NS, "", *pairs)

def _project_scoped_key(kind, attr, ref):
    """Returns a key either by grabbing it from the keyset or constructing.

    Args:
      kind: kind of the key to return.
      attr: field name that supplies 'ref', for error messages.
      ref: either a keyset or a string "<name>".

    Returns:
      graph.key of the requested kind.
    """
    if graph.is_keyset(ref):
        return ref.get(kind)
    return graph.key(kinds.LUCI_NS, "", kind, validate.string(attr, ref))

def _bucket_scoped_key(kind, attr, ref, allow_external = False):
    """Returns either a bucket-scoped or a project-scoped key of the given kind.

    Args:
      kind: kind of the key to return.
      attr: field name that supplies 'ref', for error messages.
      ref: either a keyset, a string "<bucket>/<name>" or just "<name>". If it
        is a string, it may optionally be prefixed with "<project>:" prefix to
        indicate that this is a reference to a node in another project.
      allow_external: True if it is OK to return a key from another project.

    Returns:
      graph.key of the requested kind.
    """
    if graph.is_keyset(ref):
        key = ref.get(kind)
        if key.root.kind != kinds.LUCI_NS:
            fail("unexpected key %s" % key)
        if key.root.id and not allow_external:
            fail("reference to %s in external project %r is not allowed here" % (kind, key.root.id))
        return key

    chunks = validate.string(attr, ref).split(":", 1)
    if len(chunks) == 2:
        proj, ref = chunks[0], chunks[1]
        if proj:
            if not allow_external:
                fail("reference to %s in external project %r is not allowed here" % (kind, proj))
            if proj == "*":
                fail("buildbot builders no longer allowed here")
    else:
        proj, ref = "", chunks[0]

    chunks = ref.split("/", 1)
    if len(chunks) == 1:
        return graph.key(kinds.LUCI_NS, proj, kind, chunks[0])
    return graph.key(kinds.LUCI_NS, proj, kinds.BUCKET, chunks[0], kind, chunks[1])

def _builder_ref_key(ref, attr = "triggers", allow_external = False):
    """Returns luci.builder_ref key, auto-instantiating external nodes."""
    if allow_external:
        _maybe_add_external_builder(ref)
    return _bucket_scoped_key(kinds.BUILDER_REF, attr, ref, allow_external = allow_external)

# Kinds is a enum-like struct with node kinds of various LUCI config nodes.
kinds = struct(
    # This kind is used to namespace nodes from different projects:
    #   * ("@luci.ns", "", ...) - keys of nodes in the current project.
    #   * ("@luci.ns", "<name>", ...) - keys of nodes in another project.
    LUCI_NS = "@luci.ns",

    # Publicly declarable nodes.
    PROJECT = "luci.project",
    REALM = "luci.realm",
    CUSTOM_ROLE = "luci.custom_role",
    BINDING = "luci.binding",
    LOGDOG = "luci.logdog",
    BUCKET = "luci.bucket",
    EXECUTABLE = "luci.executable",
    TASK_BACKEND = "luci.task_backend",
    BUILDER = "luci.builder",
    GITILES_POLLER = "luci.gitiles_poller",
    MILO = "luci.milo",
    LIST_VIEW = "luci.list_view",
    LIST_VIEW_ENTRY = "luci.list_view_entry",
    CONSOLE_VIEW = "luci.console_view",
    CONSOLE_VIEW_ENTRY = "luci.console_view_entry",
    EXTERNAL_CONSOLE_VIEW = "luci.external_console_view",
    CQ = "luci.cq",
    CQ_GROUP = "luci.cq_group",
    CQ_TRYJOB_VERIFIER = "luci.cq_tryjob_verifier",
    NOTIFY = "luci.notify",
    NOTIFIABLE = "luci.notifiable",  # either luci.notifier or luci.tree_closer
    BUILDER_HEALTH_NOTIFIER = "luci.builder_health_notifier",
    NOTIFIER_TEMPLATE = "luci.notifier_template",
    SHADOWED_BUCKET = "luci.shadowed_bucket",
    SHADOW_OF = "luci.shadow_of",
    BUCKET_CONSTRAINTS = "luci.bucket_constraints",
    BUILDBUCKET_NOTIFICATION_TOPIC = "luci.buildbucket_notification_topic",
    DYNAMIC_BUILDER_TEMPLATE = "luci.dynamic_builder_template",

    # Internal nodes (declared internally as dependency of other nodes).
    BUILDER_REF = "luci.builder_ref",
    TRIGGERER = "luci.triggerer",
    BINDINGS_ROOT = "luci.bindings_root",
    MILO_ENTRIES_ROOT = "luci.milo_entries_root",
    MILO_VIEW = "luci.milo_view",
    CQ_VERIFIERS_ROOT = "luci.cq_verifiers_root",
    CQ_EQUIVALENT_BUILDER = "luci.cq_equivalent_builder",
    BUCKET_CONSTRAINTS_ROOT = "luci.bucket_constraints_root",
)

# Keys is a collection of key constructors for various LUCI config nodes.
keys = struct(
    # Publicly declarable nodes.
    project = lambda: _namespaced_key(kinds.PROJECT, "..."),
    realm = lambda ref: _project_scoped_key(kinds.REALM, "realm", ref),
    custom_role = lambda ref: _project_scoped_key(kinds.CUSTOM_ROLE, "role", ref),
    logdog = lambda: _namespaced_key(kinds.LOGDOG, "..."),
    bucket = lambda ref: _project_scoped_key(kinds.BUCKET, "bucket", ref),
    executable = lambda ref: _project_scoped_key(kinds.EXECUTABLE, "executable", ref),
    buildbucket_notification_topic = lambda ref: _project_scoped_key(kinds.BUILDBUCKET_NOTIFICATION_TOPIC, "buildbucket_notification_topic", ref),
    task_backend = lambda ref: _project_scoped_key(kinds.TASK_BACKEND, "task_backend", ref),

    # TODO(vadimsh): Make them accept keysets if necessary. These currently
    # require strings, not keysets. They are currently not directly used by
    # anything, only through 'builder_ref' and 'triggerer' nodes. As such, they
    # are never consumed via keysets.
    builder = lambda bucket, name: _namespaced_key(kinds.BUCKET, bucket, kinds.BUILDER, name),
    gitiles_poller = lambda bucket, name: _namespaced_key(kinds.BUCKET, bucket, kinds.GITILES_POLLER, name),
    milo = lambda: _namespaced_key(kinds.MILO, "..."),
    list_view = lambda ref: _project_scoped_key(kinds.LIST_VIEW, "list_view", ref),
    console_view = lambda ref: _project_scoped_key(kinds.CONSOLE_VIEW, "console_view", ref),
    external_console_view = lambda ref: _project_scoped_key(kinds.EXTERNAL_CONSOLE_VIEW, "external_console_view", ref),
    cq = lambda: _namespaced_key(kinds.CQ, "..."),
    cq_group = lambda ref: _project_scoped_key(kinds.CQ_GROUP, "cq_group", ref),
    notify = lambda: _namespaced_key(kinds.NOTIFY, "..."),
    notifiable = lambda ref: _project_scoped_key(kinds.NOTIFIABLE, "notifies", ref),
    builder_health_notifier = lambda ref: _project_scoped_key(kinds.BUILDER_HEALTH_NOTIFIER, "builder_health_notifier", ref),
    notifier_template = lambda ref: _project_scoped_key(kinds.NOTIFIER_TEMPLATE, "template", ref),
    shadowed_bucket = lambda bucket_key: _namespaced_key(kinds.SHADOWED_BUCKET, bucket_key.id),
    shadow_of = lambda bucket_key: _namespaced_key(kinds.SHADOW_OF, bucket_key.id),

    # Internal nodes (declared internally as dependency of other nodes).
    builder_ref = _builder_ref_key,
    triggerer = lambda ref, attr = "triggered_by": _bucket_scoped_key(kinds.TRIGGERER, attr, ref),
    bindings_root = lambda: _namespaced_key(kinds.BINDINGS_ROOT, "..."),
    milo_entries_root = lambda: _namespaced_key(kinds.MILO_ENTRIES_ROOT, "..."),
    milo_view = lambda name: _namespaced_key(kinds.MILO_VIEW, name),
    cq_verifiers_root = lambda: _namespaced_key(kinds.CQ_VERIFIERS_ROOT, "..."),
    bucket_constraints_root = lambda: _namespaced_key(kinds.BUCKET_CONSTRAINTS_ROOT, "..."),

    # Generates a key of the given kind and name within some auto-generated
    # unique container key.
    #
    # Used with LIST_VIEW_ENTRY, CONSOLE_VIEW_ENTRY, CQ_TRYJOB_VERIFIER,
    # BUCKET_CONSTRAINTS and DYNAMIC_BUILDER_TEMPLATE helper nodes.
    # They don't really represent any "external" entities, and their names don't
    # really matter, other than for error messages.
    #
    # Note that IDs of keys whose kind stars with '_' (like '_UNIQUE' here),
    # are skipped when printing the key in error messages. Thus the meaningless
    # confusing auto-generated part of the key isn't showing up in error
    # messages.
    unique = lambda kind, name: graph.key("_UNIQUE", str(sequence.next(kind)), kind, name),
)

################################################################################
## builder_ref implementation.

def _builder_ref_add(target):
    """Adds two builder_ref nodes that have 'target' as a child.

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
        fail("got %s, expecting a builder key" % (target,))

    bucket = target.container.id  # name of the bucket, as string
    builder = target.id  # name of the builder, as string

    short = keys.builder_ref(builder)
    graph.add_node(short, idempotent = True)  # there may be such node already
    graph.add_edge(short, target)

    full = keys.builder_ref("%s/%s" % (bucket, builder))
    graph.add_node(full)
    graph.add_edge(full, target)

    return full

def _builder_ref_follow(ref_node, context_node):
    """Given a BUILDER_REF node, returns a BUILDER graph.node the ref points to.

    Emits an error and returns None if the reference is ambiguous (i.e.
    'ref_node' has more than one child). Such reference can't be used to refer
    to a single builder.

    Note that the emitted error doesn't stop the generation phase, but marks it
    as failed. This allows to collect more errors before giving up.

    Args:
      ref_node: graph.node with the ref.
      context_node: graph.node where this ref is used, for error messages.

    Returns:
      graph.node of BUILDER kind.
    """
    if ref_node.key.kind != kinds.BUILDER_REF:
        fail("%s is not builder_ref" % ref_node)

    # builder_ref nodes are always linked to something, see _builder_ref_add.
    variants = graph.children(ref_node.key)
    if not variants:
        fail("%s is unexpectedly unconnected" % ref_node)

    # No ambiguity.
    if len(variants) == 1:
        return variants[0]

    error(
        "ambiguous reference %r in %s, possible variants:\n  %s",
        ref_node.key.id,
        context_node,
        "\n  ".join([str(v) for v in variants]),
        trace = context_node.trace,
    )
    return None

# Additional API for dealing with builder_refs.
builder_ref = struct(
    add = _builder_ref_add,
    follow = _builder_ref_follow,
)

################################################################################
## Rudimentary hacky support for externally-defined builders.

def _maybe_add_external_builder(ref):
    """If 'ref' points to an external builder, defines corresponding nodes.

    Kicks in if 'ref' is a string that has form "<project>:<bucket>/<name>".
    Declares necessary luci.builder(...) and luci.builder_ref(...) nodes
    namespaced to the corresponding external project.
    """
    if type(ref) != "string" or ":" not in ref:
        return
    proj, rest = ref.split(":", 1)
    if "/" not in rest:
        return
    bucket, name = rest.split("/", 1)

    # TODO(vadimsh): This is very tightly coupled to the implementation of
    # luci.builder(...) rule. It basically sets up same structure of nodes,
    # except it doesn't fully populate luci.builder props (because we don't know
    # them), and doesn't link the builder node to the project root (because we
    # don't want to generate cr-buildbucket.cfg entry for it).

    builder = graph.key(kinds.LUCI_NS, proj, kinds.BUCKET, bucket, kinds.BUILDER, name)
    graph.add_node(builder, idempotent = True, props = {
        "name": name,
        "bucket": bucket,
        "project": proj,
    })

    # This is roughly what _builder_ref_add does, except namespaced to 'proj'.
    refs = [
        graph.key(kinds.LUCI_NS, proj, kinds.BUILDER_REF, name),
        graph.key(kinds.LUCI_NS, proj, kinds.BUCKET, bucket, kinds.BUILDER_REF, name),
    ]
    for ref in refs:
        graph.add_node(ref, idempotent = True)
        graph.add_edge(ref, builder)

################################################################################
## triggerer implementation.

def _triggerer_add(owner, idempotent = False):
    """Adds two 'triggerer' nodes that have 'owner' as a parent.

    Triggerer nodes are essentially nothing more than a way to associate a node
    of arbitrary kind ('owner') to a list of builder_ref's of builders it
    triggers (children of added 'triggerer' node).

    We need this indirection to make 'triggered_by' relation work: when a
    builder 'B' is triggered by something named 'T', we don't know whether 'T'
    is another builder or a gitiles poller ('T' may not even be defined yet).
    So instead all things that can trigger builders have an associated
    'triggerer' node and 'T' names such a node.

    To allow omitting bucket name when it is not important, each triggering
    entity actually defines two 'triggerer' nodes: a bucket-scoped one and a
    global one. During the graph traversal phase, children of both nodes are
    merged.

    If a globally named 'triggerer' node has more than one parent, it means
    there are multiple things in different buckets that have the same name.
    Using such references in 'triggered_by' relation is ambiguous. This
    situation is detected during the graph traversal phase, see
    triggerer.targets(...).

    Args:
      owner: a graph.key to setup triggerers for.
      idempotent: if True, allow the triggerer node to be redeclared.

    Returns:
      graph.key of added bucket-scoped TRIGGERER node ("<bucket>/<name>" one).
    """
    if not owner.container or owner.container.kind != kinds.BUCKET:
        fail("got %s, expecting a bucket-scoped key" % (owner,))

    bucket = owner.container.id  # name of the bucket, as string
    name = owner.id  # name of the builder or poller, as string

    # Short (not scoped to a bucket) keys are not unique, even for nodes that
    # are marked as idempotent=False. Make sure it is OK to re-add them by
    # marking them as idempotent. Dups are checked in triggerer.targets(...).
    short = keys.triggerer(name)
    graph.add_node(short, idempotent = True)
    graph.add_edge(owner, short)

    full = keys.triggerer("%s/%s" % (bucket, name))
    graph.add_node(full, idempotent = idempotent)
    graph.add_edge(owner, full)

    return full

def _triggerer_targets(root):
    """Collects all BUILDER nodes triggered by the given node.

    Enumerates all TRIGGERER children of 'root', and collects all BUILDER nodes
    they refer to, following BUILDER_REF references.

    Various ambiguities are reported as errors (which marks the generation
    phase as failed). Corresponding nodes are skipped, to collect as many
    errors as possible before giving up.

    Args:
      root: a graph.node that represents the triggering entity: something that
            has a triggerer as a child.

    Returns:
      List of graph.node of BUILDER kind, sorted by key.
    """
    out = set()

    for t in graph.children(root.key, kinds.TRIGGERER):
        parents = graph.parents(t.key)

        for ref in graph.children(t.key, kinds.BUILDER_REF):
            # Resolve builder_ref to a concrete builder. This may return None
            # if the ref is ambiguous.
            builder = builder_ref.follow(ref, root)
            if not builder:
                continue

            # If 't' has multiple parents, it can't be used in 'triggered_by'
            # relations, since it is ambiguous. Report this situation.
            if len(parents) != 1:
                error(
                    "ambiguous reference %r in %s, possible variants:\n  %s",
                    t.key.id,
                    builder,
                    "\n  ".join([str(v) for v in parents]),
                    trace = builder.trace,
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
    # as a string) or a dict with parameters to *_view_entry rules. In the
    # latter two cases, we add a *_view_entry for it automatically. This allows
    # to somewhat reduce verbosity of list definitions.
    for entry in validate.list("entries", entries):
        if type(entry) == "dict":
            entry = entry_ctor(**entry)
        elif type(entry) == "string" or (graph.is_keyset(entry) and entry.has(kinds.BUILDER_REF)):
            entry = entry_ctor(builder = entry)
        graph.add_edge(key, entry.get(entry_kind))

    return graph.keyset(key, milo_view_key)

def _view_add_entry(kind, view, builder, props = None):
    """Adds *_view_entry node.

    Common implementation for list_view_node and console_view_node. Allows
    referring to builders defined in other projects.

    Args:
      kind: a kind of the node to add (e.g. LIST_VIEW_ENTRY).
      view: a key of the parent *_view to add the entry to, if known.
      builder: a reference to builder.
      props: properties for the added node.

    Returns:
      A keyset with the added key.
    """
    if builder == None:
        fail("'builder' is required")
    builder = keys.builder_ref(builder, attr = "builder", allow_external = True)

    # Note: name of this node is important only for error messages. It isn't
    # showing up in any generated files and by construction it can't
    # accidentally collide with some other name.
    key = keys.unique(kind, builder.id)
    graph.add_node(key, props)
    if view != None:
        graph.add_edge(parent = view, child = key)
    if builder != None:
        graph.add_edge(parent = key, child = builder)

    # This is used to detect *_view_entry nodes that aren't connected to any
    # *_view. Such orphan nodes aren't allowed.
    graph.add_node(keys.milo_entries_root(), idempotent = True)
    graph.add_edge(parent = keys.milo_entries_root(), child = key)

    return graph.keyset(key)

view = struct(
    add_view = _view_add_view,
    add_entry = _view_add_entry,
)

################################################################################
## Helpers for defining luci.notifiable nodes.

def _notifiable_add(name, props, template, notified_by):
    """Adds a luci.notifiable node.

    This is a shared portion of luci.notifier and luci.tree_closer
    implementation. Nodes defined here are traversed by gen_notify_cfg in
    generators.star.

    Args:
      name: name of the notifier or the tree closer.
      props: a dict with node props.
      template: an optional reference to a luci.notifier_template to link to.
      notified_by: builders to link to.

    Returns:
      A keyset with the luci.notifiable key.
    """
    key = keys.notifiable(validate.string("name", name))
    graph.add_node(key, idempotent = True, props = props)
    graph.add_edge(keys.project(), key)

    for b in validate.list("notified_by", notified_by):
        graph.add_edge(
            parent = key,
            child = keys.builder_ref(b, attr = "notified_by"),
            title = "notified_by",
        )

    if template != None:
        graph.add_edge(key, keys.notifier_template(template))

    return graph.keyset(key)

notifiable = struct(
    add = _notifiable_add,
)


################################################################################
## Helpers for defining luci.builder_health_notifier nodes.

def _builder_health_notifier_add(owner_email, props):
    """Adds a luci.builder_health_notifier node.

    Nodes defined here are traversed by gen_notify_cfg in
    generators.star.

    Args:
      owner_email: E-mail address of the owner of the BuilderHealthNotifier.
        Each owner may only have one BuilderHealthNotifier per LUCI project.
        Messages about the monitored builders will be sent here.
      props: a dict with node props.

    Returns:
      A keyset with the luci.builder_health_notifier key.
    """
    key = keys.builder_health_notifier(validate.string("owner_email", owner_email))
    graph.add_node(key, idempotent = True, props = props)
    graph.add_edge(keys.project(), key)

    return graph.keyset(key)

bhn = struct(
    add = _builder_health_notifier_add,
)