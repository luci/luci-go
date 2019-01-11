load('@stdlib//internal/luci/lib/validate.star', 'validate')
load('@stdlib//internal/time.star', 'time')


def test_validate_string():
  call = validate.string
  assert.eq(call('a', 'zzz'), 'zzz')
  assert.eq(call('a', 'abc', regexp='a.c$'), 'abc')
  assert.eq(call('a', '', default='zzz', required=False), '')
  assert.eq(call('a', None, default='zzz', required=False), 'zzz')
  assert.eq(call('a', None, required=False), None)
  assert.fails(lambda: call('a', None), 'missing required field "a"')
  assert.fails(lambda: call('a', 1), 'bad "a": got int, want string')
  assert.fails(lambda: call('a', []), 'bad "a": got list, want string')
  assert.fails(lambda: call('a', None, default=1, required=False), 'bad "a": got int, want string')
  assert.fails(lambda: call('a', 'abcd', regexp='a.c$'), 'bad "a": "abcd" should match "a\.c\$"')


def test_validate_int():
  call = validate.int
  assert.eq(call('a', 123, min=123, max=123), 123)
  assert.eq(call('a', 0, default=123, required=False), 0)
  assert.eq(call('a', None, default=123, required=False), 123)
  assert.eq(call('a', None, required=False), None)
  assert.fails(lambda: call('a', None), 'missing required field "a"')
  assert.fails(lambda: call('a', '1'), 'bad "a": got string, want int')
  assert.fails(lambda: call('a', []), 'bad "a": got list, want int')
  assert.fails(lambda: call('a', None, default='1', required=False), 'bad "a": got string, want int')
  assert.fails(lambda: call('a', 123, min=124), 'bad "a": 123 should be >= 124')
  assert.fails(lambda: call('a', 123, max=122), 'bad "a": 123 should be <= 122')


def test_validate_bool():
  call = validate.bool
  assert.eq(call('a', True), True)
  assert.eq(call('a', 1), True)
  assert.eq(call('a', [1]), True)
  assert.eq(call('a', []), False)
  assert.eq(call('a', None, default=True, required=False), True)
  assert.eq(call('a', None, default=False, required=False), False)
  assert.eq(call('a', None, required=False), None)
  assert.fails(lambda: call('a', None), 'missing required field "a"')


def test_validate_duration():
  call = validate.duration
  m = time.minute
  assert.eq(call('a', m), m)
  assert.eq(call('a', None, default=m, required=False), m)
  assert.eq(call('a', None, required=False), None)
  assert.fails(lambda: call('a', None), 'missing required field "a"')
  assert.fails(lambda: call('a', 1), 'bad "a": got int, want duration')
  assert.fails(lambda: call('a', []), 'bad "a": got list, want duration')
  assert.fails(lambda: call('a', -m), 'bad "a": -1m0s should be >= 0s')
  assert.fails(lambda: call('a', 2*m, max=m), 'bad "a": 2m0s should be <= 1m0s')
  assert.fails(
      lambda: call('a', 500 * time.millisecond),
      'bad "a": losing precision when truncating 500ms to 1s units, use ' +
      'time.truncate\(\.\.\.\) to acknowledge')


def test_validate_list():
  call = validate.list
  assert.eq(call('a', [1, 'a']), [1, 'a'])
  assert.eq(call('a', []), [])
  assert.eq(call('a', None), [])
  assert.fails(lambda: call('a', 0), 'bad "a": got int, want list')
  assert.fails(lambda: call('a', {}), 'bad "a": got dict, want list')
  assert.fails(lambda: call('a', None, required=True), 'missing required field "a"')
  assert.fails(lambda: call('a', [], required=True), 'missing required field "a"')


def test_validate_str_dict():
  call = validate.str_dict
  assert.eq(call('a', {'k': 1}), {'k': 1})
  assert.eq(call('a', {}), {})
  assert.eq(call('a', None), {})
  assert.fails(lambda: call('a', 0), 'bad "a": got int, want dict')
  assert.fails(lambda: call('a', []), 'bad "a": got list, want dict')
  assert.fails(lambda: call('a', {1: 1}), 'bad "a": got int key, want string')
  assert.fails(lambda: call('a', {'': 1}), 'bad "a": got empty key')
  assert.fails(lambda: call('a', None, required=True), 'missing required field "a"')
  assert.fails(lambda: call('a', {}, required=True), 'missing required field "a"')


def test_validate_struct():
  call = validate.struct
  s = struct(a=1, b=2)
  assert.eq(call('a', s, 'struct'), s)
  assert.eq(call('a', None, 'struct', default=s, required=False), s)
  assert.eq(call('a', None, 'struct', required=False), None)
  assert.fails(lambda: call('a', None, 'struct'), 'missing required field "a"')
  assert.fails(lambda: call('a', 1, 'struct'), 'bad "a": got int, want struct')
  assert.fails(lambda: call('a', [], 'struct'), 'bad "a": got list, want struct')
  assert.fails(lambda: call('a', None, 'struct', default=1, required=False), 'bad "a": got int, want struct')


test_validate_string()
test_validate_int()
test_validate_bool()
test_validate_duration()
test_validate_list()
test_validate_str_dict()
test_validate_struct()
