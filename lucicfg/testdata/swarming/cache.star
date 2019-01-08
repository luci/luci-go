load('@stdlib//internal/luci/lib/swarming.star', 'swarming', 'swarmingimpl')


def test_cache_ctor():
  def eq(c, name, path, wait):
    assert.eq(c.name, name)
    assert.eq(c.path, path)
    assert.eq(c.wait_mins, wait)

  eq(swarming.cache('name'), 'name', 'name', None)
  eq(swarming.cache('name', 'path', 5*time.minute), 'name', 'path', 5)

  # Name validation.
  assert.fails(lambda: swarming.cache(''), 'should match')
  assert.fails(lambda: swarming.cache('ABC'), 'should match')

  # Path validation.
  assert.fails(lambda: swarming.cache('n', ''), 'path must not be empty')
  assert.fails(lambda: swarming.cache('n', 'a\\b'), r'must not contain "\\"')
  assert.fails(lambda: swarming.cache('n', 'a/../b'), 'must not contain ".."')
  assert.fails(lambda: swarming.cache('n', '/a'), 'must not start with "/"')

  # Duration validation.
  assert.fails(lambda: swarming.cache('n', 'p', 300), 'got int, want duration')
  assert.fails(lambda: swarming.cache('n', 'p', time.zero), '0s should be >= 1m0s')
  assert.fails(lambda: swarming.cache('n', 'p', 61*time.second), 'losing precision when truncating 1m1s to 1m0s units')


def test_validate_caches():
  call = lambda c: swarmingimpl.validate_caches('caches', c)

  c1 = swarming.cache('c1')
  c2 = swarming.cache('c2', path='p')
  c3 = swarming.cache('c3', path='p')

  assert.eq(call([c1, c2]), [c1, c2])
  assert.eq(call(None), [])

  assert.fails(lambda: call([c1, c1]), 'have same name')
  assert.fails(lambda: call([c2, c3]), 'use same path')


test_cache_ctor()
test_validate_caches()
