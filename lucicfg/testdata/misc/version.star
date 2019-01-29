def test_lucicfg_version():
  ver = lucicfg.version()
  assert.eq(type(ver), 'tuple')
  assert.eq(len(ver), 3)
  for i in ver:
    assert.eq(type(i), 'int')
  assert.true(ver >= (1, 0, 0))

test_lucicfg_version()
