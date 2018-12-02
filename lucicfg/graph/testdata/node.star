trace =  stacktrace()


def test_add_node_ok():
  g = new_graph()
  k = g.key('t1', 'id1')

  assert.true(g.node(k) == None)

  n = g.add_node(k, props={'prop': ['v1', 'v2']})

  assert.true(n != None)
  assert.true(n)
  assert.eq(g.node(k), n)
  assert.eq(str(n), 'graph.node')
  assert.eq(type(n), 'graph.node')
  assert.eq(n.key, k)
  assert.eq(n.props.prop, ['v1', 'v2'])
  assert.eq(str(n.trace),
      'Traceback (most recent call last):\n'
      + '  testdata/node.star:24: in <toplevel>\n'
      + '  testdata/node.star:10: in test_add_node_ok\n')

test_add_node_ok()


def test_node_can_be_used_in_sets():
  g = new_graph()
  n1 = g.add_node(g.key('t1', 'id1'))
  n2 = g.add_node(g.key('t1', 'id2'))
  s = set([n1, n2])
  assert.true(n1 in s)

test_node_can_be_used_in_sets()


def test_redeclaration():
  g = new_graph()
  k = g.key('t1', 'id1')

  g.add_node(k, {}, trace=trace)
  assert.fails(lambda: g.add_node(k, {}),
    r'\[t1\("id1"\)\] is redeclared, previous declaration:\n'
    + r'Traceback \(most recent call last\)\:\n'
    + r'  testdata/node\.star\:1\: in <toplevel>\n')

test_redeclaration()


def test_freezes_props():
  g = new_graph()
  k = g.key('t1', 'id1')

  vals = ['v1', 'v2']
  n = g.add_node(k, props={'prop': vals})

  # Note that freezing is NOT copying, original 'vals' is also frozen now.
  assert.fails(lambda: n.props.prop.append('z'), 'cannot append to frozen list')
  assert.fails(lambda: vals.append('z'), 'cannot append to frozen list')

test_freezes_props()


def test_non_string_prop():
  g = new_graph()
  k = g.key('t1', 'id1')
  assert.fails(lambda: g.add_node(k, props={1: '2'}), "non-string key 1 in 'props'")

test_non_string_prop()
