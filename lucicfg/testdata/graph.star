load("@stdlib//internal/graph.star", "graph")

# Detailed unit tests for graph are in 'graph' go package. Here we only test
# that graph.star wrapper and integration with the generator state works.

def test_works():
  k1 = graph.key('t1', 'i1', 't2', 'i2')
  k2 = graph.key('t1', 'i1', 't2', 'i2')
  assert.eq(k1, k2)

test_works()
