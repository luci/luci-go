load("@stdlib//internal/graph.star", "graph")

# Detailed unit tests for graph are in 'graph' go package. Here we only test
# that graph.star wrapper and integration with the generator state works.


root = graph.key('root', 'root')
graph.add_node(root)

def child(id, msg):
  k = graph.key('child', id)
  graph.add_node(k, props = {'msg': msg})
  graph.add_edge(parent = root, child = k)

child('c1', 'hello')
child('c2', 'world')


def gen(ctx):
  msg = []
  for c in graph.children(root, 'child'):
    msg.append(c.props.msg)
  ctx.config_set['output.txt'] = ' '.join(msg) + '\n'
core.generator(impl = gen)


# Expect configs:
#
# === output.txt
# hello world
# ===
