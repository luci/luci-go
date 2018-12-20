load("@stdlib//internal/re.star", "re")

RE = r'^(([[:graph:]]+)/)?([[:graph:]]+)$'

def test():
  m, group = re.matches(RE, 'abc/def')
  assert.true(m)
  assert.eq(group[-2:], ('abc', 'def'))

  m, group = re.matches(RE, 'abc')
  assert.true(m)
  assert.eq(group[-2:], ('', 'abc'))

  m, _ = re.matches(RE, '')
  assert.true(not m)

test()
