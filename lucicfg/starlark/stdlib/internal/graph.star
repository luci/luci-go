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


_KEY_ORDER = 'key'
_DEFINITION_ORDER = 'def'


def _check_interpreter_context():
  """Verifies add_node and add_edge are used only from 'exec' context."""
  ctx = __native__.interpreter_context()
  if ctx == 'EXEC':
    return  # allowed
  if ctx == 'LOAD':
    fail('code executed via "load" must be side-effect free, consider using "exec" instead')
  elif ctx == 'GEN':
    fail('generators aren\'t allowed to modify the graph, only query it')
  else:
    fail('cannot modify the graph from interpreter context %s' % ctx)


def _key(*args):
  """Returns a key with given [(kind, name)] path.

  The kind in the last pair is considered the principal kind: when keys or nodes
  are filtered by kind, they are filtered by the kind from the last pair.

  Args:
    *args: even number of strings: kind1, name1, kind2, name2, ...

  Returns:
    graph.key object representing this path.
  """
  return __native__.graph().key(*args)


def _add_node(key, props=None, idempotent=False, trace=None):
  """Adds a node to the graph.

  If such node already exists, either fails right away (if 'idempotent' is
  false), or verifies the existing node has also been marked as idempotent and
  has exact same props dict as being passed here.

  Can be used only from code that was loaded via some exec(...). Fails if run
  from a module being loaded via load(...). Such library-like modules must not
  have side effects during their loading.

  Also fails if used from a generator callback: at this point the graph is
  frozen and can't be extended.

  Args:
    key: a node key, see graph.key(...).
    props: a dict with node properties, will be frozen.
    idempotent: True if this node can be redeclared, but only with same props.
    trace: a stack trace to associate with the node.

  Returns:
    graph.node object representing the node.
  """
  _check_interpreter_context()
  return __native__.graph().add_node(
      key, props or {}, bool(idempotent), trace or stacktrace(skip=1))


def _add_edge(parent, child, title=None, trace=None):
  """Adds an edge to the graph.

  Neither of the nodes have to exist yet: it is OK to declare nodes and edges
  in arbitrary order as long as at the end of the script execution (when the
  graph is finalized) the graph is complete.

  Fails if there's already an edge between the given nodes with exact same title
  or the new edge introduces a cycle.

  Can be used only from code that was loaded via some exec(...). Fails if run
  from a module being loaded via load(...). Such library-like modules must not
  have side effects during their loading.

  Also fails if used from a generator callback: at this point the graph is
  frozen and can't be extended.

  Args:
    parent: a parent node key, see graph.key(...).
    child: a child node key, see graph.key(...).
    title: a title for the edge, used in error messages.
    trace: a stack trace to associate with the edge.
  """
  _check_interpreter_context()
  return __native__.graph().add_edge(
      parent, child, title or '', trace or stacktrace(skip=1))


def _node(key):
  """Returns a node by the key or None if there's no such node.

  Fails if called not from a generator callback: a graph under construction
  can't be queried.

  Args:
    key: a node key, see graph.key(...).

  Returns:
    graph.node object representing the node.
  """
  return __native__.graph().node(key)


def _children(parent, kind=None, order_by=_KEY_ORDER):
  """Returns direct children of a node (given by its key), optionally filtering
  them by kind.

  Depending on 'order_by', the children are either ordered lexicographically by
  their keys or by the order edges to them were defined.

  Fails if called not from a generator callback: a graph under construction
  can't be queried.

  Args:
    parent: a key of the parent node, see graph.key(...).
    kind: a string with a kind of children to return or None for all.
    order_by: either KEY_ORDER or DEFINITION_ORDER, default KEY_ORDER.

  Returns:
    List of graph.node objects.
  """
  out = __native__.graph().children(parent, order_by)
  if kind:
    return [n for n in out if n.key.kind == kind]
  return out


def _descendants(root, visitor=None, order_by=_KEY_ORDER):
  """Recursively visits 'root' (given by its key) and all its children, in
  breadth first order, ordering edges by 'order_by'.

  Returns the list of all visited nodes, in order they were visited. Fails if
  called not from a generator callback: a graph under construction can't be
  queried.

  Each node is visited only once, even if it is reachable through multiple
  paths. Note that the graph has no cycles (by construction).

  The visitor callback (if not None) is called for each visited node. It decides
  what children to visit next. The callback always sees all children of the
  node, even if some of them (or all) have already been visited. Visited nodes
  will be skipped even if the visitor returns them.

  Args:
    root: a key of the node to start the traversal from, see graph.key(...).
    visitor: func(node: graph.node, children: []graph.node): []graph.node.
    order_by: either KEY_ORDER or DEFINITION_ORDER, default KEY_ORDER.

  Returns:
    List of visited graph.node objects, starting with the root.
  """
  return __native__.graph().descendants(root, visitor, order_by)


def _parents(child, kind=None, order_by=_KEY_ORDER):
  """Returns direct parents of a node (given by its key), optionally filtering
  them by kind.

  Depending on 'order_by', the parents are either ordered lexicographically by
  their key or by the order edges from them were defined.

  Fails if called not from a generator callback: a graph under construction
  can't be queried.

  Args:
    child: a key of the node to find parents of, see graph.key(...).
    kind: a string with a kind of parents to return or None for all.
    order_by: either KEY_ORDER or DEFINITION_ORDER, default KEY_ORDER.

  Returns:
    List of graph.node objects.
  """
  out = __native__.graph().parents(child, order_by)
  if kind:
    return [n for n in out if n.key.kind == kind]
  return out


def _sorted_nodes(nodes, order_by=_KEY_ORDER):
  """Returns a new sorted list of nodes.

  Depending on 'order_by', the nodes are either ordered lexicographically by
  their keys or by the order they were defined in the graph.

  Args:
    nodes: an iterable of graph.node objects.
    order_by: either KEY_ORDER or DEFINITION_ORDER, default KEY_ORDER.

  Returns:
    List of graph.node objects.
  """
  return __native__.graph().sorted_nodes(nodes, order_by)


# Public API of this module.
graph = struct(
    KEY_ORDER = _KEY_ORDER,
    DEFINITION_ORDER = _DEFINITION_ORDER,

    key = _key,
    add_node = _add_node,
    add_edge = _add_edge,
    node = _node,
    children = _children,
    descendants = _descendants,
    parents = _parents,
    sorted_nodes = _sorted_nodes,
)
