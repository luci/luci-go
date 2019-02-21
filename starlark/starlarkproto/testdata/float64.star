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

m = testprotos.SimpleFields()

# Default value.
assert.eq(m.f64, 0.0)

# Setter and getter works.
m.f64 = 1.0
assert.eq(m.f64, 1.0)
assert.eq(proto.to_textpb(m), 'f64: 1\n')

# Setting through constructor works.
m2 = testprotos.SimpleFields(f64=1.0)
assert.eq(m2.f64, 1.0)

# Clearing works.
m2.f64 = None
assert.eq(m2.f64, 0.0)

# Setting wrong type fails.
def set_bad():
  m2.f64 = [1, 2, 3]
assert.fails(set_bad, 'can\'t assign list to a value of kind "float64"')

# Implicit conversion from int to float is supported.
m2.f64 = 123
assert.eq(m2.f64, 123.0)
assert.eq(proto.to_textpb(m2), 'f64: 123\n')
