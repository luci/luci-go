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

l = proto.new_loader(proto.new_descriptor_set(blob=read('./testprotos/all.pb')))
testprotos = l.module('go.chromium.org/luci/starlark/starlarkproto/testprotos/test.proto')

# Can round-trip through binary encoding.
msg = testprotos.Complex(
    enum_val = 1,
    i64 = 123,
    i64_rep = [1, 2, 3],
    mp = {'1': {}, '2': {}},
    msg_val = {'i': 123},
    msg_val_rep = [{}, {}, {'i': 123}, {}],
)
blob = proto.to_wirepb(msg)
assert.eq(len(blob), 37)
assert.eq(msg, proto.from_wirepb(testprotos.Complex, blob))

# Maps are serialized deterministically.
mapMsg = testprotos.MapWithPrimitiveType(m3={i: i for i in range(100)})
b1 = proto.to_wirepb(mapMsg)
b2 = proto.to_wirepb(mapMsg)
assert.eq(b1, b2)

# The returned message is frozen.
def from_wirepb_frozen():
  proto.from_wirepb(testprotos.Complex, blob).i64 = 456
assert.fails(from_wirepb_frozen, 'cannot modify frozen')
