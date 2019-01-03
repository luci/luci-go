def test_sorting():
  g = new_graph()

  key = lambda name: g.key('node', name)
  add = lambda name: g.add_node(key(name))
  node = lambda name: g.node(key(name))

  add('c')
  add('b')
  add('a')

  g.finalize()

  nodes = [node('b'), node('a'), node('c')]

  assert.eq(g.sorted_nodes(nodes, order_by='key'), [
    node('a'),
    node('b'),
    node('c'),
  ])

  assert.eq(g.sorted_nodes(nodes, order_by='def'), [
    node('c'),
    node('b'),
    node('a'),
  ])


test_sorting()
