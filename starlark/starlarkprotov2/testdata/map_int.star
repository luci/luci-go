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

# Default value is empty typed dict.
assert.eq(type(msg.m), 'dict<string,int64>')
assert.eq(len(msg.m), 0)

# Setting individual keys is OK.
msg.m['k'] = 1
assert.eq(dict(msg.m), {'k': 1})

# Performs type checks for both keys and values.
def wrong_key_type():
  msg.m[123] = 123
assert.fails(wrong_key_type, 'bad key: got int, want string')
def wrong_val_type():
  msg.m['zzz'] = 'zzz'
assert.fails(wrong_val_type, 'bad value: got string, want int')

# Overriding the value entirely is OK.
msg.m = {'v': 1}
assert.eq(dict(msg.m), {'v': 1})

# Checking that it is a dict.
def set_to_int():
  msg.m = 1
assert.fails(set_to_int, 'got int, want an iterable mapping')

# Clearing resets the field to its default value (empty dict).
msg.m = None
assert.eq(len(msg.m), 0)

# Setting through constructor works
msg2 = testprotos.MapWithPrimitiveType(m={'k': 2})
assert.eq(dict(msg2.m), {'k': 2})

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
