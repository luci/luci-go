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

l = proto.new_loader(read('./testprotos/all.pb'))
testprotos = l.module('go.chromium.org/luci/starlark/starlarkprotov2/testprotos/test.proto')

# Note: this test also covers all other integer types, since their
# implementation is almost identical. The differences are tested in
# int_ranges.star.

m = testprotos.SimpleFields()

# Default value.
assert.eq(m.i64, 0)

# Setter and getter works.
m.i64 = 123
assert.eq(m.i64, 123)

# Setting through constructor works.
m2 = testprotos.SimpleFields(i64=456)
assert.eq(m2.i64, 456)

# Clearing works.
m2.i64 = None
assert.eq(m2.i64, 0)

# Setting wrong type fails.
def set_bad():
  m2.i64 = [1, 2, 3]
assert.fails(set_bad, 'can\'t assign "list" to "int64" field')

# Setting to a message fails.
def set_msg():
  m2.i64 = testprotos.SimpleFields()
assert.fails(set_msg, 'can\'t assign "testprotos.SimpleFields" to "int64" field')

# We don't support implicit conversions from float to int. Callers should use
# int(...) cast explicitly.
def set_float():
  m2.i64 = 123.4
assert.fails(set_float, 'can\'t assign "float" to "int64" field')

# Serialization to text proto works.
text = proto.to_textpb(testprotos.SimpleFields(i64=987))
assert.eq(text, "i64: 987\n")
