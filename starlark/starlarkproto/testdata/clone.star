# Copyright 2020 The LUCI Authors.
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


# Works in general.
src = testprotos.SimpleFields(i64=123, b=True)
assert.eq(proto.clone(src), src)

# Should return a deep copy.
map_src = testprotos.MapWithMessageType(m={
    "msg1": testprotos.Simple(many_i=[1, 2, 3]),
})

copy = proto.clone(map_src)
copy.m['msg1'].i = -1
copy.m['msg1'].many_i[0] += 100
copy.m['msg1'].many_i.append(1024)
copy.m['msg2'] = testprotos.Simple()

assert.eq(map_src, testprotos.MapWithMessageType(m={
    "msg1": testprotos.Simple(many_i=[1, 2, 3]),
}))
assert.eq(copy, testprotos.MapWithMessageType(m={
    "msg1": testprotos.Simple(i=-1, many_i=[101, 2, 3, 1024]),
    "msg2": testprotos.Simple(),
}))

# clone expects an argument.
def clone_no_args():
  proto.clone()
assert.fails(clone_no_args, 'missing argument for msg')

# Too many arguments.
def clone_too_many_args():
  proto.clone(src, src)
assert.fails(clone_too_many_args, 'clone: got 2 arguments, want at most 1')

# None argument.
def clone_with_none():
  proto.clone(None)
assert.fails(clone_with_none, 'clone: for parameter msg: got NoneType, want proto.Message')

# Wrongly typed argument.
def clone_with_int():
  proto.clone(1)
assert.fails(clone_with_int, 'clone: for parameter msg: got int, want proto.Message')
