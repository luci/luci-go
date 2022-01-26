load("@stdlib//internal/experiments.star", "experiments")

def test_happy_path():
    exp1 = experiments.register("unittest.exp_id1")
    exp2 = experiments.register("unittest.exp_id2")
    exp3 = experiments.register("unittest.exp_id3")

    assert.true(not exp1.is_enabled())
    assert.true(not exp2.is_enabled())
    assert.true(not exp3.is_enabled())

    lucicfg.enable_experiment("unittest.exp_id1")
    lucicfg.enable_experiment("unittest.exp_id2")

    assert.true(exp1.is_enabled())
    assert.true(exp2.is_enabled())
    assert.true(not exp3.is_enabled())

    assert.eq(__native__.list_enabled_experiments(), ["unittest.exp_id1", "unittest.exp_id2"])

    exp1.require()  # doesn't fail
    exp2.require()
    assert.fails(exp3.require, 'requires enabling the experiment "unittest.exp_id3"')

def test_double_registration():
    exp1 = experiments.register("unittest.exp_id")
    exp2 = experiments.register("unittest.exp_id")
    lucicfg.enable_experiment("unittest.exp_id")
    assert.true(exp1.is_enabled())
    assert.true(exp2.is_enabled())

def test_unknown_id():
    exp1 = experiments.register("unittest.exp_id1")
    exp2 = experiments.register("unittest.exp_id2")
    lucicfg.enable_experiment("unittest.zzz")

    # Still can be potentially registered later, should be enabled once queried.
    zzz = experiments.register("unittest.zzz")
    assert.true(zzz.is_enabled())
    assert.eq(__native__.list_enabled_experiments(), ["unittest.zzz"])

def test_bad_types():
    assert.fails(lambda: experiments.register(None), "got NoneType, want string")
    assert.fails(lambda: lucicfg.enable_experiment(None), "got NoneType, want string")

def with_clean_state(cb):
    __native__.clear_state()
    cb()
    __native__.clear_state()

with_clean_state(test_happy_path)
with_clean_state(test_double_registration)
with_clean_state(test_unknown_id)
with_clean_state(test_bad_types)
