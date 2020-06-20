load("@stdlib//internal/graph.star", "graph")
load("@stdlib//internal/luci/common.star", "builder_ref", "keys")

def test_follow_refs():
    def builder(bucket, name):
        k = keys.builder(bucket, name)
        graph.add_node(k)
        builder_ref.add(k)
        return k

    builder1 = builder("bucket1", "builder")
    builder2 = builder("bucket2", "builder")
    builder3 = builder("bucket", "builder3")

    referrer = graph.key("some_node", "some name")
    graph.add_node(referrer)

    __native__.graph().finalize()

    def follow(ref_name):
        ref_node = graph.node(keys.builder_ref(ref_name))
        assert.true(ref_node != None)
        return builder_ref.follow(ref_node, graph.node(referrer))

    # Follows non-ambiguous refs.
    assert.eq(follow("bucket1/builder"), graph.node(builder1))
    assert.eq(follow("bucket2/builder"), graph.node(builder2))
    assert.eq(follow("builder3"), graph.node(builder3))

    # Detects ambiguities.
    assert.fails(lambda: follow("builder"), "ambiguous reference")

def with_clean_state(cb):
    __native__.clear_state()
    __native__.fail_on_errors()
    cb()

with_clean_state(test_follow_refs)
