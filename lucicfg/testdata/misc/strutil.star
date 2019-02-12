load('@stdlib//internal/strutil.star', 'strutil')


def test_expand_int_set():
  # Most tests are on the go side. Here we test only Starlark integration.
  assert.eq(strutil.expand_int_set('a{01..03}b'), ['a01b', 'a02b', 'a03b'])
  assert.fails(lambda: strutil.expand_int_set('}{'),
      'expand_int_set: bad expression - "}" must appear after "{"')


def test_generate_name():
  assert.eq(strutil.generate_name('a %s a', ['z', 2, [3]]), 'a 1 a')
  assert.eq(strutil.generate_name('a %s a', ['zzz', 2, [3]]), 'a 2 a')
  assert.eq(strutil.generate_name('a %s a', ['z', 2, [3]]), 'a 1 a')
  assert.eq(strutil.generate_name('b %s a', ['zzz', 2, [3]]), 'b 1 a')


test_expand_int_set()
test_generate_name()
