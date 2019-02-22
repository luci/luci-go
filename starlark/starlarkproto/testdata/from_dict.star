# Copyright 2019 The LUCI Authors.
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

load("go.chromium.org/luci/starlark/starlarkproto/testprotos/test.proto", "testprotos")

def from_dict(cls, d):
  return cls(**d)

# Works.
m1 = from_dict(testprotos.MessageFields, {
  'single': {'i': 123},
  'rep': [{'i': 456}, {'i': 789}, None, testprotos.Simple(i=999)],
})
assert.eq(m1.single.i, 123)
assert.eq(type(m1.rep), 'list')
assert.eq(len(m1.rep), 4)
assert.eq(m1.rep[0].i, 456)
assert.eq(m1.rep[1].i, 789)
assert.eq(m1.rep[2].i, 0)   # fills in Nones with default values
assert.eq(m1.rep[3].i, 999)

# All Nones are converted to list of default values.
m2 = from_dict(testprotos.MessageFields, {
  'rep': [None, None],
})
assert.eq(len(m2.rep), 2)
assert.eq(m2.rep[0].i, 0)
assert.eq(m2.rep[1].i, 0)

# Tuples work too.
m3 = from_dict(testprotos.MessageFields, {
  'rep': ({'i': 456},),
})
assert.eq(type(m3.rep), 'list')  # converted to a list
assert.eq(len(m3.rep), 1)
assert.eq(m3.rep[0].i, 456)

# For oneof fields the last one wins (note that Starlark dicts are ordered).
m4 = from_dict(testprotos.Complex, {
  'simple': {'i': 1},
  'another_simple': {'j': 2},
})
assert.eq(m4.simple, None)
assert.eq(m4.another_simple.j, 2)

# Fails on wrong schema (singular field).
def wrong_schema_single():
  from_dict(testprotos.MessageFields, {
    'single': {'z': '???'},
  })
assert.fails(wrong_schema_single, 'when constructing "single" in proto "testprotos.MessageFields" - proto message "testprotos.Simple" has no field "z"')

# Fails on wrong schema (repeated field).
def wrong_schema_repeated():
  from_dict(testprotos.MessageFields, {
    'rep': [{'z': '???'}],
  })
assert.fails(wrong_schema_repeated, 'when constructing "rep" in proto "testprotos.MessageFields" - proto message "testprotos.Simple" has no field "z"')

# Fails on non-string keys.
def bad_key_type():
  from_dict(testprotos.MessageFields, {
    'single': {123: 1},
  })
assert.fails(bad_key_type, 'when constructing "single" in proto "testprotos.MessageFields" - got int dict key, expecting a string')
