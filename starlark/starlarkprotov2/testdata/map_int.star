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
assert.fails(set_to_int, 'can\'t assign "int" to a map field')

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

check_fail({0: 1}, 'key 0: can\'t assign "int" to "string" field')
check_fail({'': 'zzz'}, 'value of key "": can\'t assign "string" to "int64" field')
check_fail({None: 1}, 'key None: can\'t assign "NoneType" to "string" field')
check_fail({'': None}, 'value of key "": can\'t assign "NoneType" to "int64" field')
check_fail({'': msg}, 'value of key "": can\'t assign "testprotos.MapWithPrimitiveType" to "int64" field')
