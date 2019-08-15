# Copyright 2018 The LUCI Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Helpers.
def key_conv(x):
  if type(x) == 'int' and x < 0:
    fail('nope')
  return str(x)
def val_conv(x):
  if type(x) == 'int' and x < 0:
    fail('nope')
  return str(x)
def typed(d):
  return typed_dict(key_conv, val_conv, d)


# Typed dict implements same methods as a regular dict. If this test fails, it
# means go.starlark.net added a new dict method that should be added to typed
# dicts as well.
def test_same_dir_as_dict():
  assert.eq(dir(typed({})), dir({}))
test_same_dir_as_dict()


# Value interface.
def test_basic_value_interface():
  assert.eq(str(typed({1: 2})), 'dict<K,V>({"1": "2"})')
  assert.eq(type(typed({})), 'dict<K,V>')  # see starlark_test.go for K, V
  assert.eq(bool(typed({})), False)
  assert.eq(bool(typed({1: 2})), True)
test_basic_value_interface()


# Basic dict interface.
def test_dict():
  a = typed({1: 2, 3: 4})
  assert.eq(dict(a), {'1': '2', '3': '4'})

  # Iteration works.
  keys = []
  for k in a:
    keys.append(k)
  assert.eq(keys, ['1', '3'])

  assert.eq(len(a), 2)
  assert.eq(bool(a), True)
  assert.eq(bool(typed({})), False)

  assert.eq(a['1'], '2')
  assert.eq('1' in a, True)
  assert.eq(1 in a, False)  # no type coercion

  a['1'] = 6
  assert.eq(a['1'], '6')

  def bad_key_type():
    a[-1] = '6'
  assert.fails(bad_key_type, 'bad key: fail: nope')

  def bad_val_type():
    a[1] = -1
  assert.fails(bad_val_type, 'bad value: fail: nope')
test_dict()


# '==' and '!=' operators.
def test_equality_operators():
  assert.true(typed({}) == typed({}))
  assert.true(typed({0:1, 2:3}) == typed({0:1, 2:3}))
  assert.true(typed({0:1, 2:3}) != typed({0:1}))
  assert.true(typed({0:1, 2:3}) != typed({0:1, 9:3}))
  assert.true(typed({0:1, 2:3}) != typed({0:1, 2:9}))
  # Different key type.
  assert.true(typed({0:1}) != typed_dict(lambda x: str(x), val_conv, {0:1}))
  # Different value type.
  assert.true(typed({0:1}) != typed_dict(key_conv, lambda x: str(x), {0:1}))
test_equality_operators()


# 'setdefault' method.
def test_setdefault():
  a = typed({1: 2, 3: 4})
  assert.eq(a.setdefault(1, 666), '2')
  assert.eq(a.setdefault(5, 666), '666')
  assert.eq(a['5'], '666')
  assert.eq(a.setdefault(6), 'None')
  def bad_key_type():
    a.setdefault(-1, '6')
  assert.fails(bad_key_type, 'bad key: fail: nope')
  def bad_val_type():
    a.setdefault(1, -1)
  assert.fails(bad_val_type, 'bad value: fail: nope')
test_setdefault()


# 'update' method.
def test_update():
  # Fastest path.
  a = typed({1: 2})
  a.update(typed({3: 4, 5: 6}))
  assert.eq(dict(a), {'1': '2', '3': '4', '5': '6'})

  # Fast path.
  a = typed({1: 2})
  a.update({3: 4, 5: 6})
  assert.eq(dict(a), {'1': '2', '3': '4', '5': '6'})

  # Slow path.
  a = typed({1: 2})
  a.update([(3, 4), (5, 6)])
  assert.eq(dict(a), {'1': '2', '3': '4', '5': '6'})

  # Doesn't accept kwargs.
  assert.fails(lambda: a.update(a=1), 'update: got unexpected keyword argument')

  # Needs exactly one positional arg.
  assert.fails(lambda: a.update(), 'update: got 0 arguments, want exactly 1')
  assert.fails(lambda: a.update({}, {}), 'update: got 2 arguments, want exactly 1')

  # Need an iterable that returns pairs.
  assert.fails(lambda: a.update(1), 'update: got int, want iterable')
  assert.fails(lambda: a.update([1]), 'update: element #0 is not iterable \(int\)')
  assert.fails(lambda: a.update([(1,1,1)]), 'update: element #0 has length 3, want 2')

  # Bad conversion in fast path.
  assert.fails(lambda: a.update({-1: 0}), 'update: element #0: bad key: fail: nope')
  assert.fails(lambda: a.update({0: -1}), 'update: element #0: bad value: fail: nope')

  # Bad conversion in slow path.
  assert.fails(lambda: a.update([(-1, 0)]), 'update: element #0: bad key: fail: nope')
  assert.fails(lambda: a.update([(0, -1)]), 'update: element #0: bad value: fail: nope')
test_update()


# Other builtins we didn't touch.
def test_builtins():
  a = typed({1: 2, 3: 4})

  assert.eq(a.get('1'), '2')
  assert.eq(a.get(1), None)  # no type coercion

  assert.eq(a.items(), [('1', '2'), ('3', '4')])
  assert.eq(a.keys(), ['1', '3'])
  assert.eq(a.values(), ['2', '4'])

  assert.eq(a.pop('1'), '2')
  assert.eq(a.pop(3, None), None)  # no type coercion
  assert.eq(a.popitem(), ('3', '4'))

  a = typed({1: 2, 3: 4})
  a.clear()
  assert.eq(len(a), 0)
test_builtins()
