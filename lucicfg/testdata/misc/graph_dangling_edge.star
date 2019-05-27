load("@stdlib//internal/graph.star", "graph")

graph.add_node(key = graph.key('node', 'name'))
graph.add_edge(graph.key('node', 'name'), graph.key('node', 'missing'), 'rel')


def gen(ctx):
  fail('must not be called')
lucicfg.generator(impl = gen)


# Expect errors:
#
# Traceback (most recent call last):
#   //testdata/misc/graph_dangling_edge.star:4: in <toplevel>
#   @stdlib//internal/graph.star:161: in _add_edge
# Error: node("name") in "rel" refers to undefined node("missing")
