load('@stdlib//internal/strutil.star', 'strutil')


def test_expand_int_set():
  # Most tests are on the go side. Here we test only Starlark integration.
  assert.eq(strutil.expand_int_set('a{01..03}b'), ['a01b', 'a02b', 'a03b'])
  assert.fails(lambda: strutil.expand_int_set('}{'),
      'expand_int_set: bad expression - "}" must appear after "{"')


test_expand_int_set()
