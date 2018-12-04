trace =  stacktrace()


def test_add_edge_ok():
  g = new_graph()
  k1 = g.key('t1', 'id1')
  k2 = g.key('t1', 'id2')

  g.add_edge(parent=k1, child=k2)

  # Nodes aren't "visible" yet.
  assert.true(g.node(k1) == None)

  # They are after being added explicitly.
  g.add_node(k1, {})
  assert.true(g.node(k1) != None)

test_add_edge_ok()


def test_edge_redeclaration():
  g = new_graph()
  k1 = g.key('t1', 'id1')
  k2 = g.key('t1', 'id2')

  g.add_edge(parent=k1, child=k2, title='blah')

  assert.fails(
      lambda: g.add_edge(parent=k1, child=k2, title='blah'),
      'Relation "blah" between .* is redeclared')

  # Using a different title is fine.
  g.add_edge(parent=k1, child=k2, title='zzz')

test_edge_redeclaration()


def test_cycle_detection():
  g = new_graph()
  edge = lambda par, ch: g.add_edge(g.key('t', par), g.key('t', ch))

  edge('1', '2')
  edge('2', '3')
  edge('1', '3')
  edge('3', '4')

  assert.fails(lambda: edge('4', '1'), 'introduces a cycle')

test_cycle_detection()
