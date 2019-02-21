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

# 'bytes' fields are represented as lists of integers on the Starlark side and
# generally indistinguishable from repeated integer fields (except 0-255 range
# validation when serializing).

m = testprotos.SimpleFields()

# Default value.
assert.eq(m.bs, [])

# Setter and getter works.
m.bs = [0, 1, 2, 3]
assert.eq(m.bs, [0, 1, 2, 3])
assert.eq(proto.to_textpb(m), 'bs: "\\000\\001\\002\\003"\n')

# Setting through constructor works.
m2 = testprotos.SimpleFields(bs=[0, 1])
assert.eq(m2.bs, [0, 1])

# Clearing works.
m2.bs = None
assert.eq(m2.bs, [])

# Setting wrong type fails.
def set_bad():
  m2.bs = 1
assert.fails(set_bad, 'can\'t assign integer to a value of kind "slice"')


# Trying to serialize out-of-range byte fails.
def serialize_min():
  m2.bs = [-1]
  proto.to_textpb(m2)
assert.fails(serialize_min, 'integer -1 doesn\'t fit into uint8')

def serialize_max():
  m2.bs = [256]
  proto.to_textpb(m2)
assert.fails(serialize_max, 'integer 256 doesn\'t fit into uint8')
