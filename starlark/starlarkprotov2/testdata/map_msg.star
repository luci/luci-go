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

msg = testprotos.MapWithMessageType()

# Default value is empty dict.
assert.eq(len(msg.m), 0)

# Can set and get values, they are stored by reference.
simple = testprotos.Simple(i=123)
msg.m['k'] = simple
assert.eq(msg.m['k'].i, 123)
simple.i = 456
assert.eq(msg.m['k'].i, 456)

# None values are converted to empty messages. Use 'pop' to throw values away.
msg.m['k'] = None
assert.eq(type(msg.m['k']), 'testprotos.Simple')
assert.eq(str(msg.m['k']), '')
msg.m.pop('k')
assert.eq(msg.m.get('k'), None)

# Dicts are converted to corresponding messages.
msg.m['k'] = {'i': 999}
assert.eq(msg.m['k'].i, 999)

# Dicts with wrong schema are rejected.
def bad_schema():
  msg.m['k'] = {'unknown': 123}
assert.fails(bad_schema, 'proto message testprotos.Simple has no field "unknown"')

# Serialization to text proto works.
text = proto.to_textpb(testprotos.MapWithMessageType(m={
  'k1': testprotos.Simple(i=1),
  'k2': testprotos.Simple(i=2),
}))
assert.eq(text, """m: <
  key: "k1"
  value: <
    i: 1
  >
>
m: <
  key: "k2"
  value: <
    i: 2
  >
>
""")
