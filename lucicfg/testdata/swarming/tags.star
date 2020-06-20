load("@stdlib//internal/luci/lib/swarming.star", "swarming")

def test_validate_tags():
    call = lambda t: swarming.validate_tags("tags", t)

    assert.eq(call(None), [])
    assert.eq(call(["k1:v1", "k2:v2"]), ["k1:v1", "k2:v2"])
    assert.eq(call(["k1:v1", "k2:v2", "k1:v1", "k3:v3"]), ["k1:v1", "k2:v2", "k3:v3"])

    assert.fails(lambda: call("k:v"), "got string, want list")
    assert.fails(lambda: call(["not_kv"]), "should match")

test_validate_tags()
