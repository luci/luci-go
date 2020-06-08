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

m = testprotos.Simple()

# type() works.
assert.eq(type(m), 'proto.Message<testprotos.Simple>')

# proto.message_type() works.
assert.eq(proto.message_type(m), testprotos.Simple)

# dir() works.
assert.eq(dir(m), ['i', 'many_i'])

# All proto messages are truthy.
assert.true(bool(m))

# Stringification works.
assert.eq(str(testprotos.Simple(i=123)), 'i: 123')
assert.eq(str(testprotos.Complex(i64=1, msg_val={'i': 1})), 'i64: 1\nmsg_val: { i: 1 }')

# Stringifying the type returns its proto type name.
assert.eq(str(testprotos.Simple), 'testprotos.Simple')

# Assigning totally unsupported types to fields fails.
def set_unrelated():
  m.i = set([])
assert.fails(set_unrelated, 'got set, want int')

# Grabbing unknown field fails.
def get_unknown():
  print(m.zzz)
assert.fails(get_unknown, 'proto.Message<testprotos.Simple> has no field "zzz"')

# Setting unknown field fails.
def set_unknown():
  m.zzz = 123
assert.fails(set_unknown, 'proto.Message<testprotos.Simple> has no field "zzz"')

# Proto messages are non-hashable currently.
def use_as_key():
  d = {m: 123}
assert.fails(use_as_key, 'not hashable')

# dir(...) on a message namespace works.
assert.eq(dir(testprotos.Complex), ['ENUM_VAL_1', 'InnerMessage', 'UNKNOWN'])

# Attempting to access undefined inner message type fails.
def unknown_inner():
  testprotos.Complex.Blah()
assert.fails(unknown_inner, 'proto.MessageType has no .Blah field or method')

# Attempting to replace inner message type fails.
def replace_inner():
  testprotos.Complex.InnerMessage = None
assert.fails(replace_inner, 'can\'t assign to .InnerMessage field of proto.MessageType')

# Using message_type or non-proto fails.
assert.fails(lambda: proto.message_type(123), 'got int, want proto.Message')
