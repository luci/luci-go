# Copyright 2020 The LUCI Authors.
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

m = testprotos.Complex()

# Completely unknown fields are unset.
assert.true(not proto.has(m, 'doesnt_exist'))


# Scalar fields with default values are assumed to be always set. With proto3
# there's no way to distinguish a field initialized with its default value from
# an unset field.
assert.true(proto.has(m, 'i64'))
assert.true(proto.has(m, 'enum_val'))


# Repeated fields are assumed to be always set as well (they just may be empty).
assert.true(proto.has(m, 'i64_rep'))
assert.true(proto.has(m, 'msg_val_rep'))


# Singular message-typed fields can be uninitialized.
assert.true(not proto.has(m, 'msg_val'))
m.msg_val = {}
assert.true(proto.has(m, 'msg_val'))
m.msg_val = None
assert.true(not proto.has(m, 'msg_val'))


# One-of alternatives behave as message-typed fields, even if they have
# a primitive type.
assert.true(not proto.has(m, 'simple'))
assert.true(not proto.has(m, 'i64_oneof'))

m.simple = {}
assert.true(proto.has(m, 'simple'))
assert.true(not proto.has(m, 'i64_oneof'))

m.i64_oneof = 0
assert.true(not proto.has(m, 'simple'))
assert.true(proto.has(m, 'i64_oneof'))

m.i64_oneof = None
assert.true(not proto.has(m, 'simple'))
assert.true(not proto.has(m, 'i64_oneof'))
