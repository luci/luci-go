load('//testdata/misc/support/shared_vars.star', 'shared_vars')


def test_vars_basics():
  a = lucicfg.var()
  a.set(123)
  assert.eq(a.get(), 123)
  # Setting again is forbidden.
  assert.fails(lambda: a.set(123), 'variable reassignment is forbidden')


def test_vars_defaults():
  a = lucicfg.var(default=123)
  assert.eq(a.get(), 123)
  # Setting a var after it was auto-initialized is forbidden.
  assert.fails(lambda: a.set(None), 'variable reassignment is forbidden')


def test_vars_validator():
  a = lucicfg.var(validator=lambda v: v+1)
  a.set(1)
  assert.eq(a.get(), 2)


def test_propagation_down_exec_stack():
  # Set 'a' only.
  shared_vars.a.set('from root')

  # The execed script should be able to read 'a' and set 'b'.
  out = exec('//testdata/misc/support/uses_vars.star')
  assert.eq(out.sees_vars, ['from root', 'from inner'])

  # But the mutation to 'b' didn't propagate back to us.
  assert.eq(shared_vars.b.get(), None)


test_vars_basics()
test_vars_defaults()
test_vars_validator()
test_propagation_down_exec_stack()
