# Copyright 2019 The LUCI Authors.
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

# Works in general.
m = proto.from_textpb(testprotos.Simple, 'i: 123')
assert.eq(type(m), 'testprotos.Simple')
assert.eq(m.i, 123)

# Bad text proto. Detailed tests for type conversions are in from_proto.star.
def from_textpb_bad_proto():
  proto.from_textpb(testprotos.Simple, 'huh?')
assert.fails(from_textpb_bad_proto, 'from_textpb: line 1.0: unknown field name "huh" in testprotos.Simple')

# Too many arguments.
def from_textpb_too_many_args():
  proto.from_textpb(testprotos.Simple, 'i: 123', 'i: 123')
assert.fails(from_textpb_too_many_args, 'from_textpb: got 3 arguments, want at most 2')

# Wrong type for constructor.
def from_textpb_with_wrong_ctor():
  proto.from_textpb(lambda: None, 'i: 123')
assert.fails(from_textpb_with_wrong_ctor, 'from_textpb: got function, expecting a proto message constructor')
