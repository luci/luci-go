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
def pos_int_to_str(x):
  if type(x) == 'int' and x < 0:
    fail('nope')
  return str(x)
def typed(l):
  return typed_list(pos_int_to_str, l)
def roundtrip(l):
  return list(typed(l))
def another(x):
  return str(x)
def another_typed(l):
  return typed_list(another, l)


# NewList constructor.
def test_new_list_constructor():
  assert.eq(roundtrip([]), [])
  assert.eq(roundtrip(['1', '2']), ['1', '2'])
  assert.eq(roundtrip([1, '2']), ['1', '2'])
  assert.fails(lambda: roundtrip([0, 1, -1]), 'item #2: fail: nope')
test_new_list_constructor()


# Typed list implements same methods as a regular list. If this test fails, it
# means go.starlark.net added a new list method that should be added to typed
# lists as well.
def test_same_dir_as_list():
  assert.eq(dir(typed([])), dir([]))
test_same_dir_as_list()


# Value interface.
def test_basic_value_interface():
  assert.eq(str(typed([1, 2])), 'list<T>(["1", "2"])')
  assert.eq(type(typed([])), 'list<T>')  # see starlark_test.go for T
  assert.eq(bool(typed([])), False)
  assert.eq(bool(typed([0])), True)
test_basic_value_interface()


# Indexing and slices.
def test_indexing_and_slices():
  a = typed([0, 1, 2, 3])

  assert.eq(a[0], '0')
  a[0] = 666
  assert.eq(a[0], '666')

  assert.eq(a[3], '3')
  a[-1] = 666
  assert.eq(a[3], '666')

  slc = a[:3]
  assert.eq(type(slc), 'list<T>')
  assert.eq(list(slc), ['666', '1', '2'])

  def set_index_bad_type():
    a[1] = -1
  assert.fails(set_index_bad_type, 'item #1: fail: nope')
test_indexing_and_slices()


# Supported operators.
def test_supported_operators():
  a = typed([0, 1])
  b = typed([2])
  assert.eq(len(a), 2)
  assert.eq('0' in a, True)
  assert.eq('0' not in a, False)
  assert.eq(0 in a, False)  # strict check, no type coercion

  assert.eq(type(2*a), 'list<T>')
  assert.eq(list(2*a), ['0', '1', '0', '1'])
  assert.eq(type(a*2), 'list<T>')
  assert.eq(list(a*2), ['0', '1', '0', '1'])

  assert.eq(type(a + b), 'list<T>')
  assert.eq(list(a + b), ['0', '1', '2'])
  assert.eq(type(b + a ), 'list<T>')
  assert.eq(list(b + a), ['2', '0', '1'])

  assert.eq(type(a + [2]), 'list<T>')
  assert.eq(list(a + [2]), ['0', '1', '2'])
  assert.eq(type([2] + a ), 'list<T>')
  assert.eq(list([2] + a), ['2', '0', '1'])

  assert.fails(lambda: a + [0,-1], 'item #1: fail: nope')
  assert.fails(lambda: [0,-1] + a, 'item #1: fail: nope')

  assert.fails(lambda: a + another_typed([]), 'unknown binary op: list<T> \+ list<K>')
  assert.fails(lambda: another_typed([]) + a, 'unknown binary op: list<K> \+ list<T>')

  c = a
  c += [2]
  assert.eq(list(c), ['0', '1', '2'])
  assert.eq(list(a), ['0', '1'])  # 'c' is a copy! += is not an alias for 'extend'
test_supported_operators()


# Unsupported operators.
def test_unsupported_operators():
  a = typed([0, 1])
  assert.eq(a == ['0', '1'], False)  # there's no way to make this an error :(
  assert.eq(a == typed([0, 1]), False)
  assert.fails(lambda: a*a, 'unknown binary op: list<T> \* list<T>')
test_unsupported_operators()


# 'append' builtin.
def test_append():
  a = typed([])
  a.append('0')
  a.append(1)
  assert.eq(list(a), ['0', '1'])
  assert.fails(lambda: a.append(-1), 'append: fail: nope')
test_append()


# 'extend' builtin.
def test_extend():
  a = typed([])

  # Slow path.
  a.extend([1, 2])
  assert.eq(list(a), ['1', '2'])

  # Fast path.
  a.extend(typed([3, 4]))
  assert.eq(list(a), ['1', '2', '3', '4'])

  # Conversion error.
  assert.fails(lambda: a.extend([5, -1]), 'item #1: fail: nope')
  assert.eq(list(a), ['1', '2', '3', '4', '5'])
test_extend()


# 'insert' builtin.
def test_insert():
  a = typed([])
  a.insert(0, '0')
  a.insert(0, 1)
  assert.eq(list(a), ['1', '0'])
  assert.fails(lambda: a.insert(0, -1), 'insert: fail: nope')
test_insert()


# Other builtins we didn't touch.
def test_builtins():
  a = typed([0])
  a.clear()
  assert.eq(len(a), 0)

  a = typed([0, 1, 2])
  assert.eq(a.index('1'), 1)
  assert.eq(a.pop(), '2')
  assert.eq(list(a), ['0', '1'])

  # Need a direct match, no type coercion.
  a.remove('0')
  assert.eq(list(a), ['1'])
  assert.fails(lambda: a.remove(1), 'remove: element not found')
test_builtins()
