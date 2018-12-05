trace =  stacktrace()


def test_add_edge_ok():
  g = new_graph()
  k1 = g.key('t1', 'id1')
  k2 = g.key('t1', 'id2')

  g.add_edge(parent=k1, child=k2)
  g.add_node(k1, {})
  g.add_node(k2, {})

  # Can be successfully finalized.
  assert.true(g.finalize() == [])

test_add_edge_ok()


def test_edge_redeclaration():
  g = new_graph()
  k1 = g.key('t1', 'id1')
  k2 = g.key('t1', 'id2')

  g.add_edge(parent=k1, child=k2, title='blah')

  assert.fails(
      lambda: g.add_edge(parent=k1, child=k2, title='blah'),
      'relation "blah" between .* is redeclared')

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


def test_dangling_edges():
  g = new_graph()

  exists = g.key('t', 'exists')
  g.add_node(exists, {})

  miss = lambda id: g.key('t', str(id))

  g.add_edge(miss(0), miss(1), title='edge1')
  g.add_edge(miss(2), exists, title='edge2')
  g.add_edge(exists, miss(3), title='edge3')

  errs = g.finalize()
  assert.eq(errs, [
      'relation "edge1": refers to [t("0")] and [t("1")], neither is defined',
      '[t("exists")] in "edge2" refers to undefined [t("2")]',
      '[t("exists")] in "edge3" refers to undefined [t("3")]',
  ])

test_dangling_edges()
