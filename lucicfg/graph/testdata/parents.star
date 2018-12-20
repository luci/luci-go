def test_parents_query():
  g = new_graph()

  def mk(*args):
    k = g.key(*args)
    g.add_node(k)
    return k

  child = mk('child', 'child')

  def q(child, kind, order_by):
    keys = []
    for n in g.parents(child, kind, order_by):
      keys.append(n.key)
    return keys

  t1_a1 = mk('t1', 'id1', 'a', '1')
  t1_a2 = mk('t1', 'id1', 'a', '2')
  t1_b1 = mk('t1', 'id1', 'b', '1')
  t2_a1 = mk('t2', 'id1', 'a', '1')

  g.add_edge(t1_b1, child)
  g.add_edge(t2_a1, child)
  g.add_edge(t2_a1, child, title='another rel')
  g.add_edge(t1_a2, child)
  g.add_edge(t1_a1, child)

  g.finalize()

  # Querying in 'exec' order returns nodes in order we defined edges.
  assert.eq(q(child, 'a', 'exec'), [t2_a1, t1_a2, t1_a1])
  # Querying by key order returns nodes in lexicographical order.
  assert.eq(q(child, 'a', 'key'), [t1_a1, t1_a2, t2_a1])
  # Filtering by another kind.
  assert.eq(q(child, 'b', 'key'), [t1_b1])
  # Filtering by unknown kind is OK.
  assert.eq(q(child, 'unknown', 'key'), [])
  # Asking for parents of non-existing node is OK.
  assert.eq(q(g.key('t', 'missing'), 'a', 'key'), [])

  # order_by is validated.
  assert.fails(lambda: q(child, 'a', 'blah'), 'unknown order "blah"')

test_parents_query()
