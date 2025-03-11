load("@stdlib//internal/luci/lib/swarming.star", "swarming")

def test_dimension_ctor():
    def eq(dim, value, expiration):
        assert.eq(dim.value, value)
        assert.eq(dim.expiration, expiration)

    eq(swarming.dimension("abc"), "abc", None)
    eq(swarming.dimension("abc", expiration = time.minute), "abc", time.minute)
    eq(swarming.dimension("abc", expiration = 5 * time.minute), "abc", 5 * time.minute)

    # 'value' validation.
    assert.fails(lambda: swarming.dimension(123), "got int, want string")
    assert.fails(lambda: swarming.dimension(""), "must not be empty")

    # 'expiration' validation.
    assert.fails(lambda: swarming.dimension("a", expiration = 300), "got int, want duration")
    assert.fails(lambda: swarming.dimension("a", expiration = time.zero), "0s should be >= 1m0s")
    assert.fails(lambda: swarming.dimension("a", expiration = 61 * time.second), "losing precision when truncating 1m1s to 1m0s units")

def test_validate_dimensions():
    call = lambda dims: swarming.validate_dimensions("dims", dims)

    v1 = swarming.dimension("v1")
    v2 = swarming.dimension("v2", expiration = 5 * time.minute)

    assert.eq(call(None), {})
    assert.eq(call({"a": "v1", "b": v2}), {"a": [v1], "b": [v2]})
    assert.eq(call({"a": ["v1", v2]}), {"a": [v1, v2]})

    assert.fails(lambda: call({123: "v1"}), "got int key, want string")
    assert.fails(lambda: call({"a": 123}), "got int, want swarming.dimension")

test_dimension_ctor()
test_validate_dimensions()
