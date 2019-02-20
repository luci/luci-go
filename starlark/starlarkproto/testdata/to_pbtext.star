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

m = testprotos.Simple()

# Works in general.
m.i = 123
assert.eq(proto.to_textpb(m), 'i: 123\n')

# to_textpb expects an argument.
def to_textpb_no_args():
  proto.to_textpb()
assert.fails(to_textpb_no_args, 'missing argument for msg')

# Too many arguments.
def to_textpb_too_many_args():
  proto.to_textpb(m, m)
assert.fails(to_textpb_too_many_args, 'to_textpb: got 2 arguments, want at most 1')

# None argument.
def to_textpb_with_none():
  proto.to_textpb(None)
assert.fails(to_textpb_with_none, 'for parameter 1: got NoneType, want proto.Message')

# Wrongly typed argument.
def to_textpb_with_int():
  proto.to_textpb(1)
assert.fails(to_textpb_with_int, 'for parameter 1: got int, want proto.Message')
