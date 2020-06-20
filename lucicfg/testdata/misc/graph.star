load("@stdlib//internal/graph.star", "graph")

# Detailed unit tests for graph are in 'graph' go package. Here we only test
# that graph.star wrapper and integration with the generator state works.

root = graph.key("root", "root")
graph.add_node(root, props = {"msg": "Hey"})

def child(id, msg, parent = root):
    k = graph.key("child", id)
    graph.add_node(k, props = {"msg": msg})
    graph.add_edge(parent, k)
    return k

c1 = child("c1", "hello")
c2 = child("c2", "world")

c3 = child("c3", "deeper", parent = c1)
c4 = child("c4", "more deeper", parent = c3)
c5 = child("c5", "deeeep", parent = c4)

def gen1(ctx):
    msg = []
    for c in graph.children(root, "child"):
        msg.append("%s: %s" % (c.props.msg, graph.parents(c.key, "root")))
    ctx.output["children_parents.txt"] = "\n".join(msg) + "\n"

lucicfg.generator(impl = gen1)

def gen2(ctx):
    ctx.output["recursive.txt"] = " ".join([
        n.props.msg
        for n in graph.sorted_nodes(
            graph.descendants(root),
            graph.DEFINITION_ORDER,
        )
    ]) + "\n"

lucicfg.generator(impl = gen2)

# Expect configs:
#
# === children_parents.txt
# hello: [root("root")]
# world: [root("root")]
# ===
#
# === recursive.txt
# Hey hello world deeper more deeper deeeep
# ===
