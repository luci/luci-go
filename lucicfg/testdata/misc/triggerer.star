load("@stdlib//internal/graph.star", "graph")
load("@stdlib//internal/luci/common.star", "builder_ref", "keys", "triggerer")

### Toy builder+poller model.

def builder(bucket, name, triggered_by = [], triggers = []):
    k = keys.builder(bucket, name)
    graph.add_node(k)
    ref = builder_ref.add(k)
    trg = triggerer.add(k)
    for t in triggered_by:
        graph.add_edge(keys.triggerer(t), ref)
    for t in triggers:
        graph.add_edge(trg, keys.builder_ref(t))
    return k

def poller(bucket, name, triggers = []):
    k = keys.gitiles_poller(bucket, name)
    graph.add_node(k)
    trg = triggerer.add(k)
    for t in triggers:
        graph.add_edge(trg, keys.builder_ref(t))
    return k

def targets(poller_key):
    return [n.key for n in triggerer.targets(graph.node(poller_key))]

### Test cases.

def test_triggers():
    p1 = poller("buck", "p1", triggers = ["buck1/b1", "b2"])
    p2 = poller("buck", "p2", triggers = ["b2"])
    p3 = poller("buck", "p3")
    b1 = builder("buck1", "b1")
    b2 = builder("buck2", "b2")
    __native__.graph().finalize()
    assert.eq(targets(p1), [b1, b2])
    assert.eq(targets(p2), [b2])
    assert.eq(targets(p3), [])

def test_triggered_by():
    p1 = poller("buck", "p1")
    p2 = poller("buck", "p2")
    b1 = builder("buck1", "b1", triggered_by = ["buck/p1"])
    b2 = builder("buck2", "b2", triggered_by = ["p1", "p2"])
    __native__.graph().finalize()
    assert.eq(targets(p1), [b1, b2])
    assert.eq(targets(p2), [b2])

def test_both_directions_not_an_error():
    p1 = poller("buck", "p1", triggers = ["buck1/b1", "b2"])
    b1 = builder("buck1", "b1", triggered_by = ["p1"])
    b2 = builder("buck2", "b2", triggered_by = ["buck/p1"])
    __native__.graph().finalize()
    assert.eq(targets(p1), [b1, b2])

def test_ambiguous_builder_ref():
    p = poller("buck", "p", triggers = ["b"])
    b1 = builder("buck1", "b")
    b2 = builder("buck2", "b")
    __native__.graph().finalize()
    assert.fails(
        lambda: targets(p),
        r'ambiguous reference "b" in luci.gitiles_poller\("buck/p"\), possible variants:\n' +
        r'  luci.builder\("buck1/b"\)\n' +
        r'  luci.builder\("buck2/b"\)',
    )

def test_ambiguous_triggerer():
    p1 = poller("buck1", "p")
    p2 = poller("buck2", "p")
    bp = builder("buck3", "p")
    b = builder("buck", "b", triggered_by = ["p"])
    __native__.graph().finalize()
    err_re = (
        r'ambiguous reference "p" in luci.builder\("buck/b"\), possible variants:\n' +
        r'  luci.gitiles_poller\("buck1/p"\)\n' +
        r'  luci.gitiles_poller\("buck2/p"\)\n' +
        r'  luci.builder\("buck3/p"\)'
    )
    assert.fails(lambda: targets(p1), err_re)
    assert.fails(lambda: targets(p2), err_re)

def test_triggering_cycle():
    builder("buck", "b1", triggers = ["b2"])
    assert.fails(
        lambda: builder("buck", "b2", triggers = ["b1"]),
        "introduces a cycle",
    )

### Test runner.

def with_clean_state(cbs):
    for cb in cbs:
        __native__.clear_state()
        __native__.fail_on_errors()
        cb()

with_clean_state([
    test_triggers,
    test_triggered_by,
    test_both_directions_not_an_error,
    test_ambiguous_builder_ref,
    test_ambiguous_triggerer,
    test_triggering_cycle,
])
