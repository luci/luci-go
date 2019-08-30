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

ds = proto.new_descriptor_set(name='zzz', blob=read('./testprotos/all.pb'))
assert.true(ds)
assert.eq(str(ds), 'proto.DescriptorSet("zzz")')
assert.eq(type(ds), 'proto.DescriptorSet')
assert.eq(dir(ds), ['register'])

# Can be used as a map key.
m = {ds: 123}

# Registration works.
l1 = proto.new_loader()
ds.register(l1)
assert.true(l1.module('go.chromium.org/luci/starlark/starlarkproto/testprotos/test.proto'))
ds.register(l1)  # doing it twice is OK
l1.add_descriptor_set(ds)  # this also works

# Registration through deps works.
l2 = proto.new_loader()
ds2 = proto.new_descriptor_set(deps=[ds])
ds2.register(l2)
assert.true(l2.module('go.chromium.org/luci/starlark/starlarkproto/testprotos/test.proto'))
