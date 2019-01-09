load('@stdlib//internal/luci/lib/swarming.star', 'swarming')


def test_cache_ctor():
  def eq(c, path, name, wait):
    assert.eq(c.path, path)
    assert.eq(c.name, name)
    assert.eq(c.wait_for_warm_cache, wait)

  eq(swarming.cache('path'), 'path', 'path', None)
  eq(swarming.cache('path', 'name', 5*time.minute), 'path', 'name', 5*time.minute)

  # Path validation.
  assert.fails(lambda: swarming.cache('', 'n'), 'must not be empty')
  assert.fails(lambda: swarming.cache('a\\b', 'n'), r'must not contain "\\"')
  assert.fails(lambda: swarming.cache('a/../b', 'n'), 'must not contain ".."')
  assert.fails(lambda: swarming.cache('/a', 'n'), 'must not start with "/"')

  # Name validation.
  assert.fails(lambda: swarming.cache('p', ''), 'should match')
  assert.fails(lambda: swarming.cache('p', 'ABC'), 'should match')

  # Duration validation.
  assert.fails(lambda: swarming.cache('p', 'n', 300), 'got int, want duration')
  assert.fails(lambda: swarming.cache('p', 'n', time.zero), '0s should be >= 1m0s')
  assert.fails(lambda: swarming.cache('p', 'n', 61*time.second), 'losing precision when truncating 1m1s to 1m0s units')


def test_validate_caches():
  call = lambda c: swarming.validate_caches('caches', c)

  c1 = swarming.cache('c1')
  c2 = swarming.cache('c2', name='n')
  c3 = swarming.cache('c3', name='n')

  assert.eq(call([c1, c2]), [c1, c2])
  assert.eq(call(None), [])

  assert.fails(lambda: call([c1, c1]), 'use same path')
  assert.fails(lambda: call([c2, c3]), 'use same name')


test_cache_ctor()
test_validate_caches()
