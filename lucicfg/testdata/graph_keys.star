load("@stdlib//internal/graph.star", "graph")

def test_keys_work():
  k1 = graph.key('t1', 'id1')
  assert.eq(type(k1), 'graph.key')
  assert.eq(str(k1), '[t1(id1)]')
  assert.true(k1)

  k2 = graph.key('t1', 'id1')  # exact same as k1
  assert.true(k1 == k2)

  k3 = graph.key('t1', 'id1', 't2', 'id2')
  assert.true(k1 != k3)
  assert.eq(str(k3), '[t1(id1), t2(id2)]')

  # Can be used as key in dicts.
  d = {}
  d[k1] = '1'
  assert.eq(d[k2], '1')


def test_keys_fail():
  assert.fails(lambda: graph.key(), 'empty key path')
  assert.fails(lambda: graph.key('t1'), 'has odd number of components')
  assert.fails(
      lambda: graph.key('t1', None),
      'all arguments must be strings, arg #1 was NoneType')


test_keys_work()
test_keys_fail()
