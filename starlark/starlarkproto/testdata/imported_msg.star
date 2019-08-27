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

m = testprotos.RefsOtherProtos()

# Indirectly using messages imported from elsewhere works.
m.another_msg.i = 123
m.ts.seconds = 456
assert.eq(proto.to_textpb(m), """another_msg: <
  i: 123
>
ts: <
  seconds: 456
>
""")

# To instantiate them directly, need to load the corresponding proto module
# first. To avoid clashing with already imported 'testprotos' symbol, import it
# under a different name.
another_pb = l.module('go.chromium.org/luci/starlark/starlarkproto/testprotos/another.proto')
assert.eq(proto.to_textpb(another_pb.AnotherMessage(i=123)), "i: 123\n")

# Can import modules with dotted names too.
google_pb = l.module('google/protobuf/timestamp.proto')
assert.eq(proto.to_textpb(google_pb.Timestamp(seconds=456)), "seconds: 456\n")
