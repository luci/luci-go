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

outer = testprotos.Complex()

# Constructors for nested messages live under the outer message namespace.
inner = testprotos.Complex.InnerMessage(i=123)

# Assignment works.
outer.msg_val = inner
assert.eq(proto.to_textpb(outer), 'msg_val: <\n  i: 123\n>\n')

# Clearing works.
outer.msg_val = None
assert.eq(proto.to_textpb(outer), '')

# Auto-instantiation of default value works.
outer.msg_val.i = 456
assert.eq(proto.to_textpb(outer), 'msg_val: <\n  i: 456\n>\n')

# Repeated fields also work.
outer2 = testprotos.Complex()
outer2.msg_val_rep = [
  testprotos.Complex.InnerMessage(i=123),
  testprotos.Complex.InnerMessage(i=456)
]
assert.eq(proto.to_textpb(outer2), """msg_val_rep: <
  i: 123
>
msg_val_rep: <
  i: 456
>
""")
