# Prepare CLI vars as:
#
# test_var=abc
# another_var=def

load("//misc/support/shared_vars.star", "shared_vars")

def test_vars_basics():
    a = lucicfg.var()
    a.set(123)
    assert.eq(a.get(), 123)

    # Setting again is forbidden.
    assert.fails(lambda: a.set(123), "variable reassignment is forbidden")

def test_vars_defaults():
    a = lucicfg.var(default = 123)
    assert.eq(a.get(), 123)

    # Setting a var after it was auto-initialized is forbidden.
    assert.fails(lambda: a.set(None), "variable reassignment is forbidden")

def test_vars_validator():
    a = lucicfg.var(validator = lambda v: v + 1)
    a.set(1)
    assert.eq(a.get(), 2)

def test_propagation_down_exec_stack():
    # Set 'a' only.
    shared_vars.a.set("from root")

    # The execed script should be able to read 'a' and set 'b'.
    out = exec("//misc/support/uses_vars.star")
    assert.eq(out.sees_vars, ["from root", "from inner"])

    # But the mutation to 'b' didn't propagate back to us.
    assert.eq(shared_vars.b.get(), None)

def test_expose_as_works():
    # See the top of this file for where 'abc' is set.
    assert.eq(lucicfg.var(expose_as = "test_var").get(), "abc")

    # Default works.
    assert.eq(lucicfg.var(expose_as = "some", default = "zzz").get(), "zzz")

    # 'None' default also works.
    assert.eq(lucicfg.var(expose_as = "third").get(), None)

def test_expose_as_set_fails():
    assert.fails(
        lambda: lucicfg.var(expose_as = "v1").set("123"),
        "the value of the variable is controlled through CLI flag " +
        r'\"\-var v1=..." and can\'t be changed from Starlark side',
    )

def test_expose_as_bad_default_type():
    assert.fails(
        lambda: lucicfg.var(expose_as = "v2", default = 123),
        "must have a string or None default, got int 123",
    )

def test_expose_as_duplicate():
    v1 = lucicfg.var(expose_as = "another_var")
    v2 = lucicfg.var(expose_as = "another_var")
    assert.eq(v1.get(), "def")
    assert.eq(v2.get(), "def")

    # Still "physically" different vars though, just have the exact same value.
    assert.true(v1 != v2)

test_vars_basics()
test_vars_defaults()
test_vars_validator()
test_propagation_down_exec_stack()
test_expose_as_works()
test_expose_as_set_fails()
test_expose_as_bad_default_type()
test_expose_as_duplicate()
