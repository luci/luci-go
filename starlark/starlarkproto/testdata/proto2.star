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
proto2 = l.module('go.chromium.org/luci/starlark/starlarkproto/testprotos/proto2.proto')

m = proto2.Proto2Message()

# Scalar fields.
assert.eq(m.i, 0)
m.i = 123
assert.eq(m.i, 123)

# Repeated fields.
assert.eq(len(m.rep_i), 0)
m.rep_i = [1, 2, 3]
assert.eq(list(m.rep_i), [1, 2, 3])

# Serialization works.
assert.eq(proto.to_textpb(m), "i: 123\nrep_i: 1\nrep_i: 2\nrep_i: 3\n")
