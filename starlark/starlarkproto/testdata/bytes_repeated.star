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

# Repeated 'bytes' field is represented by a list of strings.

m = testprotos.SimpleFields()

# Default value.
assert.eq(len(m.bs_rep), 0)

# Setter and getter works.
m.bs_rep = ['', '\x01\x02\x03']
assert.eq(list(m.bs_rep), ['', '\x01\x02\x03'])
assert.eq(proto.to_textpb(m), 'bs_rep: ""\nbs_rep: "\\x01\\x02\\x03"\n')

# Setting through constructor works.
m2 = testprotos.SimpleFields(bs_rep=['a', 'b'])
assert.eq(list(m2.bs_rep), ['a', 'b'])

# Clearing works.
m2.bs_rep = None
assert.eq(len(m2.bs_rep), 0)

# Setting wrong type fails.
def set_bad():
  m2.bs_rep = 1
assert.fails(set_bad, 'got int, want an iterable')

# Trying to put a wrong type into the list fails.
def set_bad_item1():
  m2.bs_rep = [1]
assert.fails(set_bad_item1, 'item #0: got int, want string')


# Trying to put a wrong type into the list fails.
def set_bad_item2():
  m2.bs_rep = [None]
assert.fails(set_bad_item2, 'item #0: got NoneType, want string')
