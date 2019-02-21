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

msg = testprotos.MapWithPrimitiveType()

# Default value is empty dict.
assert.eq(msg.m, {})

# Setting individual keys is OK.
msg.m['k'] = 1
assert.eq(msg.m, {'k': 1})

# No type checks at this stage.
msg.m[123] = 'zzz'
assert.eq(msg.m, {'k': 1, 123: 'zzz'})

# Overriding the value entirely is OK.
msg.m = {'v': 1}
assert.eq(msg.m, {'v': 1})

# Checking that it is a dict, but nothing else at this stage.
msg.m = {123: 'zzz'}
def set_to_int():
  msg.m = 1
assert.fails(set_to_int, 'can\'t assign integer to a value of kind "map"')

# Clearing resets the field to its default value (empty dict).
msg.m = None
assert.eq(msg.m, {})

# Setting through constructor works
msg2 = testprotos.MapWithPrimitiveType(m={'k': 2})
assert.eq(msg2.m, {'k': 2})

# Serialization to text proto works.
text = proto.to_textpb(testprotos.MapWithPrimitiveType(m={
  'k1': 1,
  'k2': 2,
}))
assert.eq(text, """m: <
  key: "k1"
  value: 1
>
m: <
  key: "k2"
  value: 2
>
""")

# Conversion to proto does full type checking.
def check_fail(m, msg):
  assert.fails(lambda: proto.to_textpb(testprotos.MapWithPrimitiveType(m=m)), msg)

check_fail({0: 1}, 'bad key 0 - can\'t assign integer to a value of kind "string"')
check_fail({'': 'zzz'}, 'bad value at key "" - can\'t assign string to a value of kind "int64"')
check_fail({None: 1}, 'bad key None - can\'t assign nil to a value of kind "string"')
check_fail({'': None}, 'bad value at key "" - can\'t assign nil to a value of kind "int64"')
check_fail({'': msg}, 'bad value at key "" - can\'t assign proto struct to a value of kind "int64"')
