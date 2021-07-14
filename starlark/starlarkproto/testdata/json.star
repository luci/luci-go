# Copyright 2021 The LUCI Authors.
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

# String fields with luci.text_pb_format option=JSON get formatted across
# multiple lines

m = testprotos.SimpleFields()
m.json = '{"foo": 0, "bar": "blah", "baz": ["x", "y", "z"]}'
assert.eq(proto.to_textpb(m), """\
json:
  '{'
  '  "foo": 0,'
  '  "bar": "blah",'
  '  "baz": ['
  '    "x",'
  '    "y",'
  '    "z"'
  '  ]'
  '}'
""")

def set_json_invalid():
    m.json = 'foo'
    proto.to_textpb(m)
assert.fails(set_json_invalid, "value for 'json' must be valid JSON, got value 'foo'")

m2 = testprotos.SimpleFields()
m2.json_rep = ['{"foo": 0}', '{"bar": 1}']
assert.eq(proto.to_textpb(m2), """\
json_rep:
  '{'
  '  "foo": 0'
  '}'
json_rep:
  '{'
  '  "bar": 1'
  '}'
""")

def set_json_rep_invalid():
    m2.json_rep = ['foo']
    proto.to_textpb(m2)
assert.fails(set_json_rep_invalid, "value for 'json_rep' must be valid JSON, got value 'foo'")

m3 = testprotos.NestedJson()
m3.nested = testprotos.Json()
m3.nested.json = '{"foo": 0, "bar": 1}'
assert.eq(proto.to_textpb(m3), """\
nested {
  json:
    '{'
    '  "foo": 0,'
    '  "bar": 1'
    '}'
}
""")

def set_nested_json_invalid():
    m3.nested.json = 'foo'
    proto.to_textpb(m3)
assert.fails(set_nested_json_invalid, "value for 'nested.json' must be valid JSON, got value 'foo'")
