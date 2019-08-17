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
testprotos = l.module('go.chromium.org/luci/starlark/starlarkproto/testprotos/test.proto')

msg = testprotos.MapWithPrimitiveType()

# Default value is empty typed dict.
assert.eq(type(msg.m1), 'dict<string,int64>')
assert.eq(len(msg.m1), 0)

# Setting individual keys is OK.
msg.m1['k'] = 1
assert.eq(dict(msg.m1), {'k': 1})

# Performs type checks for both keys and values.
def wrong_key_type():
  msg.m1[123] = 123
assert.fails(wrong_key_type, 'bad key: got int, want string')
def wrong_val_type():
  msg.m1['zzz'] = 'zzz'
assert.fails(wrong_val_type, 'bad value: got string, want int')

# Overriding the value entirely is OK.
msg.m1 = {'v': 1}
assert.eq(dict(msg.m1), {'v': 1})

# Checking that it is a dict.
def set_to_int():
  msg.m1 = 1
assert.fails(set_to_int, 'got int, want an iterable mapping')

# Checking that dict items have correct types.
def set_to_wrong_dict():
  msg.m1 = {'zzz': 'not an int'}
assert.fails(set_to_wrong_dict, 'when constructing dict<string,int64>: bad value: got string, want int')

# Clearing resets the field to its default value (empty dict).
msg.m1 = None
assert.eq(len(msg.m1), 0)

# Setting through constructor works
msg2 = testprotos.MapWithPrimitiveType(m1={'k': 2})
assert.eq(dict(msg2.m1), {'k': 2})

# Serialization to text proto works.
text = proto.to_textpb(testprotos.MapWithPrimitiveType(m1={
  'k1': 1,
  'k2': 2,
}))
assert.eq(text, """m1: <
  key: "k1"
  value: 1
>
m1: <
  key: "k2"
  value: 2
>
""")

# Fields of the exact same type can point to a single object.
msg.m1 = {'k1': 123}
msg2.m1 = msg.m1
assert.true(msg2.m1 == msg.m1)
msg2.m1['k2'] = 456
assert.eq(dict(msg.m1), {'k1': 123, 'k2': 456})

# Assigning a dict makes a copy.
dct = {'k': 555}
msg.m1 = dct
assert.true(msg.m1 != dct)
dct['k'] = 666
assert.eq(msg.m1['k'], 555)  # old

# map<string, int32>, i.e. different value type.
msg.m2 = {'k': 111}
def set_to_m2():
  msg.m1 = msg.m2
assert.fails(set_to_m2,
    'can\'t assign dict<string,int32> to field "m1" in ' +
    'proto.Message<testprotos.MapWithPrimitiveType>: want dict<string,int64> or just dict')
msg.m1 = dict(msg.m2)
assert.eq(dict(msg.m1), {'k': 111})

# map<int64, int64>, i.e. different key type.
msg.m3 = {111: 111}
def set_to_m3():
  msg.m1 = msg.m3
assert.fails(set_to_m3,
    'can\'t assign dict<int64,int64> to field "m1" in ' +
    'proto.Message<testprotos.MapWithPrimitiveType>: want dict<string,int64> or just dict')

# Ranges of ints within a map are checked.
TWO_POW_63 = 9223372036854775808
def set_big_key():
  msg.m3[TWO_POW_63] = 1
assert.fails(set_big_key, 'bad key: 9223372036854775808 doesn\'t fit into int64')
def set_big_val():
  msg.m3[1] = TWO_POW_63
assert.fails(set_big_val, 'bad value: 9223372036854775808 doesn\'t fit into int64')
