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

# Behavior and implementation if message-valued repeated fields isn't much
# different from other repeated fields, so we test only special cases.

m = testprotos.MessageFields()

# Default value.
assert.eq(type(m.rep), 'list<proto.Message<testprotos.Simple>>')
assert.eq(len(m.rep), 0)

# Can append to it, it is just like a list.
m.rep.append(testprotos.Simple(i=123))
assert.eq(m.rep[0].i, 123)

# Can replace it.
m.rep = [testprotos.Simple(i=456)]
assert.eq(m.rep[0].i, 456)

# Assigning None initializes a new empty message.
m.rep[0] = None
assert.eq(m.rep[0].i, 0)

# Assigning a dict initializes a new message.
m.rep[0] = {'i': 999}
assert.eq(m.rep[0].i, 999)

# Does type checks when updating the list.
def append_bad_value():
  m.rep.append(testprotos.MessageFields())
assert.fails(append_bad_value,
    'append: got proto.Message<testprotos.MessageFields>, want proto.Message<testprotos.Simple>')
def set_bad_value():
  m.rep[0] = testprotos.MessageFields()
assert.fails(set_bad_value,
    'item #0: got proto.Message<testprotos.MessageFields>, want proto.Message<testprotos.Simple>')

# Checks types of elements when constructing from a list.
def copy_bad_list():
  m.rep = [testprotos.Simple(i=456), testprotos.MessageFields()]
assert.fails(copy_bad_list,
    'item #1: got proto.Message<testprotos.MessageFields>, want proto.Message<testprotos.Simple>')

# Serialization to text proto works.
text = proto.to_textpb(testprotos.MessageFields(
  rep=[
    testprotos.Simple(i=123),
    testprotos.Simple(i=456),
  ],
))
assert.eq(text, """rep: <
  i: 123
>
rep: <
  i: 456
>
""")
