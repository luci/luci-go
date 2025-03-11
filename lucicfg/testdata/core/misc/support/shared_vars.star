def set_b(val):
    # Can touch vars from a function called from 'exec' context.
    shared_vars.b.set(val)

shared_vars = struct(
    a = lucicfg.var(),
    b = lucicfg.var(),
    set_b = set_b,
)

# Verify we can't touch them from the top scope of a module being 'load'ed.
assert.fails(lambda: shared_vars.a.set(123), "only code that is being ...")
assert.fails(lambda: shared_vars.a.get(), "only code that is being ...")

# Even through a function.
assert.fails(lambda: set_b(123), "only code that is being ...")
