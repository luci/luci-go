def test_meta_version():
  ver = meta.version()
  assert.eq(type(ver), 'tuple')
  assert.eq(len(ver), 3)
  for i in ver:
    assert.eq(type(i), 'int')

test_meta_version()
