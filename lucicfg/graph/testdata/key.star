def test_keys_work():
  g = new_graph()

  k1 = g.key('t1', 'id1')
  assert.eq(type(k1), 'graph.key')
  assert.eq(str(k1), '[t1("id1")]')
  assert.true(k1)

  k2 = g.key('t1', 'id1')  # exact same as k1
  assert.true(k1 == k2)

  k3 = g.key('t1', 'id1', 't2', 'id2')
  assert.true(k1 != k3)
  assert.eq(str(k3), '[t1("id1"), t2("id2")]')

  # Can be used as key in dicts.
  d = {}
  d[k1] = '1'
  assert.eq(d[k2], '1')


def test_keys_fail():
  g = new_graph()

  assert.fails(lambda: g.key(), 'empty key path')
  assert.fails(lambda: g.key('t1'), 'has odd number of components')
  assert.fails(
      lambda: g.key('t1', None),
      'all arguments must be strings, arg #1 was NoneType')
  assert.fails(lambda: g.key('t1', 'i1\x00sneaky'), 'has zero byte inside')


def test_keys_incomparable():
  # Keys from different graphs are not equal, even if they have same path.
  k1 = new_graph().key('t1', 'id1')
  k2 = new_graph().key('t1', 'id1')
  assert.true(k1 != k2)


test_keys_work()
test_keys_fail()
test_keys_incomparable()
