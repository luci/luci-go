load('@stdlib//internal/strutil.star', 'strutil')


def test_expand_int_set():
  # Most tests are on the go side. Here we test only Starlark integration.
  assert.eq(strutil.expand_int_set('a{01..03}b'), ['a01b', 'a02b', 'a03b'])
  assert.fails(lambda: strutil.expand_int_set('}{'),
      'expand_int_set: bad expression - "}" must appear after "{"')


def test_json_to_yaml():
  test = lambda i, o: assert.eq(strutil.json_to_yaml(i), o)
  test('', 'null\n')
  test('\"abc\"', 'abc\n')
  test('\"abc: def\"', '\'abc: def\'\n')
  test('123.0', '123\n')
  test('{}', '{}\n')
  test('{"abc": {"def": [1, "2", null, 3]}, "zzz": []}', """abc:
  def:
  - 1
  - "2"
  - null
  - 3
zzz: []
""")


test_expand_int_set()
test_json_to_yaml()
