load("@stdlib//internal/graph.star", "graph")


def test_works():
  k1 = graph.key('t1', 'i1', 't2', 'i2')
  k2 = graph.key('t1', 'i1', 't2', 'i2')
  assert.eq(k1, k2)


test_works()
