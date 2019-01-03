def test_descendants():
  g = new_graph()

  def node(name):
    k = g.key('node', name)
    g.add_node(k, idempotent=True)
    return k

  def edge(parent, child):
    g.add_edge(node(parent), node(child))

  root = node('root')

  edge('root', '3')
  edge('root', '2')
  edge('root', '1')
  edge('1', '1_1')
  edge('1_1', 'leaf')
  edge('2', 'leaf')

  g.finalize()

  # Redefine 'node' to return graph.node now, to make assertions below look
  # less cluttered.
  def node(name):
    return g.node(g.key('node', name))

  # Works without visitor callback: just returns all descendants.

  # In 'key' order (default).
  assert.eq(g.descendants(root), [
      node('root'),
      node('1'),
      node('2'),
      node('3'),
      node('1_1'),
      node('leaf'),
  ])

  # In 'def' order.
  assert.eq(g.descendants(root, order_by='def'), [
      node('root'),
      node('3'),
      node('2'),
      node('1'),
      node('leaf'),
      node('1_1'),
  ])

  # Just visit everything and see what we get in each individual call.

  # In 'key' order (default).
  visited = []
  def visitor(node, children):
    visited.append((node, children))
    return children
  g.descendants(root, visitor)
  assert.eq(visited, [
      (node('root'), [node('1'), node('2'), node('3')]),
      (node('1'), [node('1_1')]),
      (node('2'), [node('leaf')]),
      (node('3'), []),
      (node('1_1'), [node('leaf')]),
      (node('leaf'), []),  # visited only once
  ])

  # In 'def' order.
  visited = []
  def visitor(node, children):
    visited.append((node, children))
    return children
  g.descendants(root, visitor, 'def')
  assert.eq(visited, [
      (node('root'), [node('3'), node('2'), node('1')]),
      (node('3'), []),
      (node('2'), [node('leaf')]),
      (node('1'), [node('1_1')]),
      (node('leaf'), []),  # visited only once
      (node('1_1'), [node('leaf')]),
  ])

  # Verify we can select what to visit.

  visited = []
  def visitor(node, children):
    visited.append((node, children))
    return [children[0]] if children else []
  assert.eq(g.descendants(root, visitor), [
      node('root'),
      node('1'),
      node('1_1'),
      node('leaf'),
  ])
  assert.eq(visited, [
      (node('root'), [node('1'), node('2'), node('3')]),
      (node('1'), [node('1_1')]),
      (node('1_1'), [node('leaf')]),
      (node('leaf'), []),
  ])

  # Checks the type of the return value, allows only subset of 'children'.

  def visitor(node, children):
    return 'huh'
  assert.fails(lambda: g.descendants(root, visitor),
      'unexpectedly returned string instead of a list')

  def visitor(node, children):
    return children + ['huh']
  assert.fails(lambda: g.descendants(root, visitor),
      'unexpectedly returned string as element #3 instead of a graph.node')

  def visitor(node, children):
    return [node]
  assert.fails(lambda: g.descendants(root, visitor),
      r'unexpectedly returned node\("root"\) which is not a child of node\("root"\)')


test_descendants()
