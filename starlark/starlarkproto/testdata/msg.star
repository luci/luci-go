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

load("go.chromium.org/luci/starlark/starlarkproto/testprotos/test.proto", "testprotos")

m = testprotos.MessageFields()

# The default value is zero value of the corresponding type (as we check by
# grabbing a field from it, since == for proto messages is not implemented yet).
assert.eq(m.single.i, 0)

# Setter works.
m.single = testprotos.Simple(i=123)
assert.eq(m.single.i, 123)

# We set by reference, not by value.
ref = testprotos.Simple(i=456)
m.single = ref
assert.eq(m.single.i, 456)
ref.i = 789
assert.eq(m.single.i, 789)

# Clearing resets the field to its default zero value.
m.single = None
assert.eq(m.single.i, 0)

# Setting wrong type is forbidden.
def set_as_int():
  m.single = 123
assert.fails(set_as_int, 'can\'t assign integer to a value of kind "ptr"')

# Setting to a message of a wrong type is also forbidden.
def set_as_msg():
  m.single = testprotos.MessageFields()
assert.fails(set_as_msg, 'incompatible types "Simple" and "MessageFields"')

# The full type-correctness of the inner message is checked only during
# serialization.
m.single = testprotos.Simple(many_i=[None])
def serialize():
  proto.to_pbtext(m)
assert.fails(serialize, 'can\'t assign nil to a value of kind "int64"')

# Serialization works.
text = proto.to_pbtext(testprotos.MessageFields(single=testprotos.Simple(i=999)))
assert.eq(text, """single: <
  i: 999
>
""")
