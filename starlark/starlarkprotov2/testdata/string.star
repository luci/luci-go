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

m = testprotos.SimpleFields()

# Default value.
assert.eq(m.s, '')

# Setter and getter works.
m.s = 'blah'
assert.eq(m.s, 'blah')
assert.eq(proto.to_textpb(m), 's: "blah"\n')

# Setting through constructor works.
m2 = testprotos.SimpleFields(s='blah')
assert.eq(m2.s, 'blah')

# Clearing works.
m2.s = None
assert.eq(m2.s, '')

# Setting wrong type fails.
def set_bad():
  m2.s = 1
assert.fails(set_bad, 'got int, want string')

# Assiging string to a non-string field fails.
def set_string_to_int():
  m2.i64 = ''
assert.fails(set_string_to_int, 'got string, want int')
