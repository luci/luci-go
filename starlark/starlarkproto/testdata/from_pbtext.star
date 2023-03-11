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

l = proto.new_loader(proto.new_descriptor_set(blob=read('./testprotos/all.pb')))
testprotos = l.module('go.chromium.org/luci/starlark/starlarkproto/testprotos/test.proto')

# Works in general.
m = proto.from_textpb(testprotos.Simple, 'i: 123')
assert.eq(type(m), 'proto.Message<testprotos.Simple>')
assert.eq(m.i, 123)

# Unrecognized fields are forbidden.
def unrecognized_field():
  proto.from_textpb(testprotos.Simple, 'unknown: 123')
assert.fails(unrecognized_field, 'unknown field')

# Unrecognized fields are allowed if discard_unknown is true.
proto.from_textpb(testprotos.Simple, 'unknown: 123', discard_unknown = True)

# The returned message is frozen.
def from_textpb_frozen():
  proto.from_textpb(testprotos.Simple, 'i: 123').i = 456
assert.fails(from_textpb_frozen, 'cannot modify frozen')

# Bad text proto. Detailed tests for type conversions are in from_proto.star.
def from_textpb_bad_proto():
  proto.from_textpb(testprotos.Simple, '???')
assert.fails(from_textpb_bad_proto, 'syntax error')

# Too many arguments.
def from_textpb_too_many_args():
  proto.from_textpb(testprotos.Simple, 'i: 123', False, False)
assert.fails(from_textpb_too_many_args, 'from_textpb: got 4 arguments, want at most 3')

# Wrong type for constructor.
def from_textpb_with_wrong_ctor():
  proto.from_textpb(lambda: None, 'i: 123')
assert.fails(from_textpb_with_wrong_ctor, 'from_textpb: got function, expecting a proto message constructor')
