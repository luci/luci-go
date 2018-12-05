load("@stdlib//internal/graph.star", "graph")

graph.add_node(key = graph.key('node', 'name'))
graph.add_edge(graph.key('node', 'name'), graph.key('node', 'missing'), 'rel')


def gen(ctx):
  fail('must not be called')
core.generator(impl = gen)


# Expect errors:
#
# Traceback (most recent call last):
#   //testdata/graph_dangling_edge.star:4: in <toplevel>
# Error: [node("name")] in "rel" refers to undefined [node("missing")]
