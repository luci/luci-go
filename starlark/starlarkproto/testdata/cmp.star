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

l = proto.new_loader(read('./testprotos/all.pb'))
testprotos = l.module('go.chromium.org/luci/starlark/starlarkproto/testprotos/test.proto')

m1 = testprotos.Complex()

# Trivial cases.
assert.true(m1 == m1)
assert.true(m1 == testprotos.Complex())
assert.true(m1 != testprotos.Simple())
assert.true(m1 != testprotos.Complex(i64=123))
assert.true(testprotos.Complex(enum_val=1) != testprotos.Complex(i64=123))

# Comparison operation has no observable side effects (like appearance of
# optional message-valued fields).
assert.eq(str(m1), '')

# Empty submessage and unset submessage are NOT the same thing.
assert.true(testprotos.Complex(msg_val={}) != testprotos.Complex())

# Singular fields are checked.
assert.true(testprotos.Complex(i64=123) == testprotos.Complex(i64=123))
assert.true(testprotos.Complex(i64=123) != testprotos.Complex(i64=456))

# Assigning a field to its default value doesn't influence the comparison.
m1.i64 = 0
assert.true(m1 == testprotos.Complex())
m1.i64_rep = []
assert.true(m1 == testprotos.Complex())
m1.msg_val_rep = []
assert.true(m1 == testprotos.Complex())
m1.mp = {}
assert.true(m1 == testprotos.Complex())

# Singular message fields are checked recursively.
assert.true(testprotos.Complex(msg_val={'i':1}) == testprotos.Complex(msg_val={'i':1}))
assert.true(testprotos.Complex(msg_val={'i':1}) != testprotos.Complex(msg_val={'i':2}))

# Repeated fields are checked recursively.
assert.true(testprotos.Complex(i64_rep=[1]) == testprotos.Complex(i64_rep=[1]))
assert.true(testprotos.Complex(i64_rep=[1]) != testprotos.Complex(i64_rep=[2]))

# Repeated message fields are checked recursively.
assert.true(testprotos.Complex(msg_val_rep=[{'i':1}]) == testprotos.Complex(msg_val_rep=[{'i':1}]))
assert.true(testprotos.Complex(msg_val_rep=[{'i':1}]) != testprotos.Complex(msg_val_rep=[{'i':2}]))

# Map fields are checked recursively.
assert.true(testprotos.Complex(mp={'1':{'i':1}}) == testprotos.Complex(mp={'1':{'i':1}}))
assert.true(testprotos.Complex(mp={'1':{'i':1}}) != testprotos.Complex(mp={'1':{'i':2}}))

# Oneof fields are tested correctly.
assert.true(testprotos.Complex(simple={}) == testprotos.Complex(simple={}))
assert.true(testprotos.Complex(simple={}) != testprotos.Complex(another_simple={}))

# Even when doing the "force" switch of the oneof or when resetting it.
m2 = testprotos.Complex(simple={})
m2.another_simple = {}
assert.true(m2 == testprotos.Complex(another_simple={}))
m2.another_simple = None
assert.true(m2 == testprotos.Complex())

# Visits all fields.
def msg(x):
  return testprotos.Complex(
      enum_val = 1,
      i64 = 123,
      i64_rep = [1, 2, 3],
      mp = {'1': {}, '2': {}},
      msg_val = {'i': 123},
      msg_val_rep = [{}, {}, {'i': x}, {}],
  )
assert.true(msg(1) == msg(1))
assert.true(msg(1) != msg(2))


# Other comparison operators are not supported.
assert.fails(lambda: msg(0) < msg(0), '"<" is not implemented for proto.Message<testprotos.Complex>')
