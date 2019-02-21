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

m = testprotos.Complex()

# Oneof alternatives have no defaults.
assert.eq(m.simple, None)
assert.eq(m.another_simple, None)

# Setting one alternative resets the other (and doesn't touch other fields).
m.i64 = 123
m.simple = testprotos.Simple()
assert.true(m.simple != None)
assert.true(m.another_simple == None)
assert.eq(m.i64, 123)
m.another_simple = testprotos.AnotherSimple()
assert.true(m.simple == None)
assert.true(m.another_simple != None)
assert.eq(m.i64, 123)

# Setting a "picked" alternative to None resets it.
assert.true(m.another_simple != None)
m.another_simple = None
assert.true(m.another_simple == None)

# Setting some other alternative to None does nothing.
m.simple = testprotos.Simple()
m.another_simple = None
assert.true(m.simple != None)

# In constructors the last kwarg wins (starlark dicts preserve order).
m2 = testprotos.Complex(
    simple=testprotos.Simple(),
    another_simple=testprotos.AnotherSimple())
assert.true(m2.simple == None)
assert.true(m2.another_simple != None)
m3 = testprotos.Complex(
    another_simple=testprotos.AnotherSimple(),
    simple=testprotos.Simple())
assert.true(m3.simple != None)
assert.true(m3.another_simple == None)

# Serialization works.
assert.eq(
    proto.to_textpb(testprotos.Complex(simple=testprotos.Simple(i=1))),
    "simple: <\n  i: 1\n>\n")
