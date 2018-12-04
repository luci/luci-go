load("@stdlib//internal/graph.star", "graph")

# Detailed unit tests for graph are in 'graph' go package. Here we only test
# that graph.star wrapper and integration with the generator state works.


k1 = graph.key('t1', 'i1', 't2', 'i2')
k2 = graph.key('t1', 'i1', 't2', 'i2')
assert.eq(k1, k2)
graph.add_node(key = k1, props = {'prop': 'val\n'})


def gen(ctx):
  k = graph.key('t1', 'i1', 't2', 'i2')
  ctx.config_set['output.txt'] = graph.node(k).props.prop
core.generator(impl = gen)


# Expect configs:
#
# === output.txt
# val
# ===
