def test_vars():
  a, b = lucicfg.var(), lucicfg.var()
  assert.eq(a.get(), None)
  assert.eq(b.get(), None)
  a.set(123)
  b.set(456)
  assert.eq(a.get(), 123)
  assert.eq(b.get(), 456)
  # Setting again is forbidden.
  assert.fails(lambda: a.set(123), 'variable reassignment is forbidden')

test_vars()
