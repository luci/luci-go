trace =  stacktrace()


def test_add_node_ok():
  g = new_graph()
  k = g.key('parent', 'par', 'kind', 'name')
  g.add_node(k, props={'prop': ['v1', 'v2']})

  g.finalize()
  n = g.node(k)

  assert.true(n != None)
  assert.true(n)
  assert.eq(str(n), 'kind("par/name")')
  assert.eq(type(n), 'graph.node')
  assert.eq(n.key, k)
  assert.eq(n.props.prop, ['v1', 'v2'])
  assert.eq(str(n.trace),
      'Traceback (most recent call last):\n'
      + '  testdata/node.star:23: in <toplevel>\n'
      + '  testdata/node.star:7: in test_add_node_ok\n')

test_add_node_ok()


def test_stringification():
  def key_to_node_str(*pairs):
    g = new_graph()
    k = g.key(*pairs)
    g.add_node(k)
    g.finalize()
    return str(g.node(k))

  assert.eq(key_to_node_str('b', 'b1', 'kind', 'name'), 'kind("b1/name")')
  assert.eq(key_to_node_str('_skip', 'skip', 'kind', 'name'), 'kind("name")')
  assert.eq(key_to_node_str('@n', 'ns', 'b', 'b1', 'kind', 'name'), 'kind("ns:b1/name")')
  assert.eq(key_to_node_str('@n', 'ns', '_skip', 'skip', 'kind', 'name'), 'kind("ns:name")')
  assert.eq(key_to_node_str('@n', '', 'b', 'b1', 'kind', 'name'), 'kind("b1/name")')
  assert.eq(key_to_node_str('@n', '', '_skip', 'skip', 'kind', 'name'), 'kind("name")')

test_stringification()


def test_node_can_be_used_in_sets():
  g = new_graph()

  k1 = g.key('t1', 'id1')
  k2 = g.key('t1', 'id2')

  g.add_node(k1)
  g.add_node(k2)

  g.finalize()

  n1 = g.node(k1)
  n2 = g.node(k2)

  s = set([n1, n2])
  assert.true(n1 in s)

test_node_can_be_used_in_sets()


def test_redeclaration():
  g = new_graph()
  k = g.key('t1', 'id1')

  g.add_node(k, {}, trace=trace)
  assert.fails(lambda: g.add_node(k, {}),
    r't1\("id1"\) is redeclared, previous declaration:\n'
    + r'Traceback \(most recent call last\)\:\n'
    + r'  testdata/node\.star\:1\: in <toplevel>\n')

test_redeclaration()


def test_idempotent_nodes():
  g = new_graph()
  k1 = g.key('t1', 'id1')
  k2 = g.key('t1', 'id2')
  k3 = g.key('t1', 'id3')

  # Declaring idempotent node twice with exact same props is OK.
  g.add_node(k1, {'a': [1, 1]}, idempotent=True)
  g.add_node(k1, {'a': [1, 1]}, idempotent=True)
  # Redeclaring idempotent node as non-idempotent is not OK.
  assert.fails(
      lambda: g.add_node(k1, {'a': [1, 1]}, idempotent=False), 'redeclared')
  # Redeclaring idempotent node with different props is not OK.
  assert.fails(
      lambda: g.add_node(k1, {'a': [1, 2]}, idempotent=True), 'redeclared')

  # A node declared as non-idempotent cannot be redeclared as idempotent.
  g.add_node(k2, idempotent=False)
  assert.fails(lambda: g.add_node(k2, idempotent=True), 'redeclared')

  # A node declared as idempotent cannot be redeclared as non-idempotent.
  g.add_node(k3, idempotent=True)
  assert.fails(lambda: g.add_node(k3, idempotent=False), 'redeclared')

test_idempotent_nodes()


def test_freezes_props():
  g = new_graph()
  k = g.key('t1', 'id1')

  vals = ['v1', 'v2']
  g.add_node(k, props={'prop': vals})

  g.finalize()
  n = g.node(k)

  # Note that freezing is NOT copying, original 'vals' is also frozen now.
  assert.fails(lambda: n.props.prop.append('z'), 'cannot append to frozen list')
  assert.fails(lambda: vals.append('z'), 'cannot append to frozen list')

test_freezes_props()


def test_non_string_prop():
  g = new_graph()
  k = g.key('t1', 'id1')
  assert.fails(lambda: g.add_node(k, props={1: '2'}), "non-string key 1 in 'props'")

test_non_string_prop()
