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

# Repeated 'bytes' field is represented by a list of lists.

m = testprotos.SimpleFields()

# Default value.
assert.eq(m.bs_rep, [])

# Setter and getter works.
m.bs_rep = [[], [1, 2, 3]]
assert.eq(m.bs_rep, [[], [1, 2, 3]])
assert.eq(proto.to_pbtext(m), 'bs_rep: ""\nbs_rep: "\\001\\002\\003"\n')

# Setting through constructor works.
m2 = testprotos.SimpleFields(bs_rep=[[0], [1]])
assert.eq(m2.bs_rep, [[0], [1]])

# Clearing works.
m2.bs_rep = None
assert.eq(m2.bs_rep, [])

# Setting wrong type fails.
def set_bad():
  m2.bs_rep = 1
assert.fails(set_bad, 'can\'t assign integer to a value of kind "slice"')

# None values are forbidden when serializing.
def set_none_item():
  m2.bs_rep = [None]
  proto.to_pbtext(m2)
assert.fails(set_none_item, 'can\'t assign nil to a value of kind "slice"')

# Trying to put a wrong type into the list fails when serializing.
def set_bad_item():
  m2.bs_rep = [1]
  proto.to_pbtext(m2)
assert.fails(set_bad_item, 'can\'t assign integer to a value of kind "slice"')
