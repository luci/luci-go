load('@stdlib//internal/luci/lib/validate.star', 'validate')
load('@stdlib//internal/time.star', 'time')


def test_validate_string():
  call = validate.string
  assert.eq(call('a', 'zzz'), 'zzz')
  assert.eq(call('a', '', default='zzz'), '')
  assert.eq(call('a', None, default='zzz'), 'zzz')
  assert.fails(lambda: call('a', None), 'bad "a"\: missing')
  assert.fails(lambda: call('a', 1), 'bad "a"\: got int, want string')
  assert.fails(lambda: call('a', [], default='zzz'), 'bad "a": got list, want string')
  assert.fails(lambda: call('a', None, default=1), 'bad "a"\: got int, want string')


def test_validate_duration():
  call = validate.duration
  m = time.minute
  assert.eq(call('a', m), 60)
  assert.eq(call('a', None, default=m), 60)
  assert.fails(lambda: call('a', None), 'bad "a": missing')
  assert.fails(lambda: call('a', 1), 'bad "a": got int, want duration')
  assert.fails(lambda: call('a', [], default=m), 'bad "a": got list, want duration')
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
  assert.fails(lambda: call('a', None, required=True), 'bad "a": missing')
  assert.fails(lambda: call('a', [], required=True), 'bad "a": missing')


def test_validate_struct():
  call = validate.struct
  s = struct(a=1, b=2)
  assert.eq(call('a', s, 'struct'), s)
  assert.eq(call('a', None, 'struct', default=s), s)
  assert.fails(lambda: call('a', None, 'struct'), 'bad "a"\: missing')
  assert.fails(lambda: call('a', 1, 'struct'), 'bad "a"\: got int, want struct')
  assert.fails(lambda: call('a', [], 'struct', default=s), 'bad "a": got list, want struct')
  assert.fails(lambda: call('a', None, 'struct', default=1), 'bad "a"\: got int, want struct')


test_validate_string()
test_validate_duration()
test_validate_list()
test_validate_struct()
