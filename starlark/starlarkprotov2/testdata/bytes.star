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

# 'bytes' fields are represented as strings on the Starlark side.

m = testprotos.SimpleFields()

# Default value.
assert.eq(m.bs, '')

# Setter and getter works.
m.bs = '\x00\x01\x02\x03'
assert.eq(m.bs, '\x00\x01\x02\x03')
assert.eq(proto.to_textpb(m), 'bs: "\\x00\\x01\\x02\\x03"\n')

# Setting through constructor works.
m2 = testprotos.SimpleFields(bs='abc')
assert.eq(m2.bs, 'abc')

# Clearing works.
m2.bs = None
assert.eq(m2.bs, '')

# Setting wrong type fails.
def set_bad():
  m2.bs = 1
assert.fails(set_bad, 'got int, want string')
