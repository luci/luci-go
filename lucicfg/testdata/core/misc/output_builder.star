load("//misc/support/test_pb.star", "testproto")

def test_output_builder():
    cs = __native__.new_output_builder()
    assert.eq(type(cs), "output")

    # Setters work.
    cs["hello"] = "world"
    cs["proto"] = testproto.Msg(i = 123)
    assert.eq(cs["hello"], "world")
    assert.eq(cs["proto"].i, 123)

    # Only string keys are allowed.
    def try_set():
        cs[1] = "..."

    assert.fails(try_set, "should be a string, not int")

    # Only string or proto values are allowed.
    def try_set():
        cs["1"] = 1

    assert.fails(try_set, "should be either a string or a proto message, not int")

def test_config_generators():
    def gen1(ctx):
        ctx.output["hello"] = "world"

    lucicfg.generator(impl = gen1)

    def gen2(ctx):
        assert.eq(ctx.output["hello"], "world")
        ctx.output["hello"] = "nope"

    lucicfg.generator(impl = gen2)

    ctx = __native__.new_gen_ctx()
    __native__.call_generators(ctx)
    assert.eq(ctx.output["hello"], "nope")

def with_clean_state(cb):
    __native__.clear_state()
    cb()
    __native__.clear_state()

with_clean_state(test_output_builder)
with_clean_state(test_config_generators)
