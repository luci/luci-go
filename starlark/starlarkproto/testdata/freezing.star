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
m1 = testprotos.Simple(i=456, many_i=[1, 2, 3])
freeze(m1)
assert.eq(m1.i, 456)
assert.eq(m1.many_i, [1, 2, 3])
def change_m1_i_1():
  m1.i = 456
assert.fails(change_m1_i_1, 'cannot modify frozen proto message "testprotos.Simple"')
def change_m1_i_2():
  m1.i = None
assert.fails(change_m1_i_2, 'cannot modify frozen proto message "testprotos.Simple"')

# Freezes recursively.
def change_m1_many_i():
  m1.many_i.append(4)
assert.fails(change_m1_many_i, 'cannot append to frozen list')

# Auto-initialization of fields still works, but default values are frozen.
m2 = testprotos.Simple()
freeze(m2)
assert.eq(m2.i, 0)
assert.eq(m2.many_i, [])
def change_m2_many_i():
  m2.many_i.append(4)
assert.fails(change_m2_many_i, 'cannot append to frozen list')
