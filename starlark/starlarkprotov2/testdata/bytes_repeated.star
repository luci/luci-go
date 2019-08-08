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

# Repeated 'bytes' field is represented by a list of strings.

m = testprotos.SimpleFields()

# Default value.
assert.eq(m.bs_rep, [])

# Setter and getter works.
m.bs_rep = ['', '\x01\x02\x03']
assert.eq(m.bs_rep, ['', '\x01\x02\x03'])
assert.eq(proto.to_textpb(m), 'bs_rep: ""\nbs_rep: "\\x01\\x02\\x03"\n')

# Setting through constructor works.
m2 = testprotos.SimpleFields(bs_rep=['a', 'b'])
assert.eq(m2.bs_rep, ['a', 'b'])

# Clearing works.
m2.bs_rep = None
assert.eq(m2.bs_rep, [])

# Setting wrong type fails.
def set_bad():
  m2.bs_rep = 1
assert.fails(set_bad, 'can\'t assign "int" to a repeated field')

# Trying to put a wrong type into the list fails when serializing.
def set_bad_item():
  m2.bs_rep = [1]
  proto.to_textpb(m2)
assert.fails(set_bad_item, 'can\'t assign "int" to "bytes" field')
