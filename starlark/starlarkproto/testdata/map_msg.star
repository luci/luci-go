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

msg = testprotos.MapWithMessageType()

# Default value is empty dict.
assert.eq(msg.m, {})

# Can set and get values.
msg.m['k'] = testprotos.Simple(i=123)
assert.eq(msg.m['k'].i, 123)

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

# Conversion to proto does full type checking.
def check_fail(m, msg):
  assert.fails(lambda: proto.to_textpb(testprotos.MapWithMessageType(m=m)), msg)

check_fail({'': None}, 'can\'t assign nil to a testprotos.Simple message')
check_fail({'': 1}, 'can\'t assign integer to a testprotos.Simple message')
check_fail({'': msg}, 'can\'t assign a testprotos.MapWithMessageType message to a testprotos.Simple message')
