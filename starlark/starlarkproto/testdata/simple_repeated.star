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

l = proto.new_loader(proto.new_descriptor_set(blob=read('./testprotos/all.pb')))
testprotos = l.module('go.chromium.org/luci/starlark/starlarkproto/testprotos/test.proto')

# Note: this test also covers all other scalar types, since the implementation
# of repeated fields is identical for all of them.

m = testprotos.SimpleFields()

# Default value.
assert.eq(type(m.i64_rep), 'list<int64>')
assert.eq(len(m.i64_rep), 0)

# Can append to it, it is just like a list.
m.i64_rep.append(1)
assert.eq(list(m.i64_rep), [1])

# Can completely recreated the field by replacing with default.
m.i64_rep = None
assert.eq(len(m.i64_rep), 0)

# Fields of the exact same type can point to a single object.
m2 = testprotos.SimpleFields()
m2.i64_rep = m.i64_rep
assert.true(m2.i64_rep == m.i64_rep)
m.i64_rep.append(4444)
assert.eq(m2.i64_rep[-1], 4444)

# Assigning a regular list makes a copy.
lst = [1, 2, 3]
m.i64_rep = lst
assert.true(m.i64_rep != lst)
lst.append(4)
assert.eq(m.i64_rep[-1], 3)  # old one

# Rejects list<T> with different T.
def set_wrong_list():
  m.i64_rep = m.bs_rep
assert.fails(set_wrong_list, 'want list<int64> or just list')

# int64 and sint64 (etc) are actually same type on Starlark side and thus can
# be assigned to one another.
m.si64_rep = [1, 2, 3]
m.i64_rep = m.si64_rep
assert.true(m.i64_rep == m.si64_rep)

# Same with bytes and string.
m.bs_rep = ['a', 'b', 'c']
m.str_rep = m.bs_rep
assert.true(m.str_rep == m.bs_rep)

# Trying to replace with a completely wrong type is an error.
def set_int():
  m.i64_rep = 123
assert.fails(set_int, 'got int, want an iterable')

# Does type checks when updating the list.
def append_bad_value():
  m.i64_rep.append('zzz')
assert.fails(append_bad_value, 'append: got string, want int')
m.i64_rep = [0]
def set_bad_value():
  m.i64_rep[0] = None
assert.fails(set_bad_value, 'item #0: got NoneType, want int')

# Checks types of elements when constructing from a list.
def copy_bad_list():
  m.i64_rep = [1, 2, None]
assert.fails(copy_bad_list, 'when constructing list<int64>: item #2: got NoneType, want int')

# Serialization to text proto works.
text = proto.to_textpb(testprotos.SimpleFields(i64_rep=[1, 2, 3]))
assert.eq(text, """i64_rep: 1
i64_rep: 2
i64_rep: 3
""")
