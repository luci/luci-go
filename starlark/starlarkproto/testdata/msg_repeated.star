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

load("go.chromium.org/luci/starlark/starlarkproto/testprotos/test.proto", "testprotos")

# Behavior and implementation if message-valued repeated fields isn't much
# different from other repeated fields, so we test only special cases.

m = testprotos.MessageFields()

# Default value.
assert.eq(m.rep, [])

# Can append to it, it is just a list.
m.rep.append(testprotos.Simple(i=123))
assert.eq(m.rep[0].i, 123)

# Can replace it.
m.rep = [testprotos.Simple(i=456)]
assert.eq(m.rep[0].i, 456)

# Sneakily adding wrong-typed element to the list is NOT an immediate error
# currently. This is discovered later when trying to serialize the object.
m.rep = [testprotos.Simple(i=456), testprotos.MessageFields()]
def serialize():
  proto.to_pbtext(m)
assert.fails(serialize, 'list item #1 - can\'t assign a testprotos.MessageFields message to a testprotos.Simple message')

# Serialization to text proto works.
text = proto.to_pbtext(testprotos.MessageFields(
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
