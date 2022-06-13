load("@stdlib//internal/experiments.star", "experiments")
load("@stdlib//internal/lucicfg.star", "lucicfg")

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

def test_auto_enable():
    # Note: in tests lucicfg has phony version "1.1.1" (see starlark_test.go),
    # so all check_version checks below should not exceed it.

    experiments.register("unitest.new1", "1.1.1")
    experiments.register("unitest.old1", "1.0.0")
    experiments.register("unitest.manual1")
    assert.eq(__native__.list_enabled_experiments(), [])

    lucicfg.check_version("1.0.5")
    assert.eq(__native__.list_enabled_experiments(), ["unitest.old1"])

    experiments.register("unitest.new2", "1.1.1")
    experiments.register("unitest.old2", "1.0.0")
    experiments.register("unitest.manual2")
    assert.eq(__native__.list_enabled_experiments(), ["unitest.old1", "unitest.old2"])

    lucicfg.check_version("1.1.1")
    assert.eq(__native__.list_enabled_experiments(), [
        "unitest.new1",
        "unitest.new2",
        "unitest.old1",
        "unitest.old2",
    ])

    # No "rollbacks" to enabled experiments.
    lucicfg.check_version("1.0.5")
    assert.eq(__native__.list_enabled_experiments(), [
        "unitest.new1",
        "unitest.new2",
        "unitest.old1",
        "unitest.old2",
    ])

    # Remembers highest version ever passed to check_version.
    exp = experiments.register("unitest.new3", "1.1.1")
    assert.true(exp.is_enabled())

def with_clean_state(cb):
    __native__.clear_state()
    cb()
    __native__.clear_state()

with_clean_state(test_happy_path)
with_clean_state(test_double_registration)
with_clean_state(test_unknown_id)
with_clean_state(test_bad_types)
with_clean_state(test_auto_enable)
