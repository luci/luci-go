def test_finalization():
  g = new_graph()
  k1 = g.key('t1', 'id1')
  k2 = g.key('t1', 'id2')
  k3 = g.key('t1', 'id3')

  # Before the finalization the graph can be modified.
  g.add_node(k1)
  g.add_node(k2)
  g.add_edge(k1, k2)

  # But not queried.
  assert.fails(lambda: g.node(k1), 'cannot query a graph under construction')
  assert.fails(lambda: g.children(k1, 't1'), 'cannot query a graph under construction')

  # Can be finalized. Refinalizing is noop.
  assert.eq(g.finalize(), [])
  assert.eq(g.finalize(), [])

  # The graph is no longer mutable.
  assert.fails(lambda: g.add_node(k3), 'cannot modify a finalized graph')
  assert.fails(lambda: g.add_edge(k1, k3), 'cannot modify a finalized graph')

  # But is it queryable now.
  assert.true(g.node(k1) != None)
  assert.true(g.children(k1, 't1'), [g.node(k2)])

test_finalization()
