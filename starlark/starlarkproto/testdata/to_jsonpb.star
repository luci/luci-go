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

m = testprotos.SimpleFields()

# Works in general.
m.i64 = 123
m.b = True
assert.eq(proto.to_jsonpb(m), """{
	"i64": "123",
	"b": true
}""")

# Omits defaults.
m.i64 = 0
m.b = False
assert.eq(proto.to_jsonpb(m), '{}')

# Obeys the use_proto_names argument.
m.str_rep = ["hello"]
assert.eq(proto.to_jsonpb(m), """{
	"strRep": [
		"hello"
	]
}""")
assert.eq(proto.to_jsonpb(m, use_proto_names = True), """{
	"str_rep": [
		"hello"
	]
}""")

# to_jsonpb expects an argument.
def to_jsonpb_no_args():
  proto.to_jsonpb()
assert.fails(to_jsonpb_no_args, 'missing argument for msg')

# Too many arguments.
def to_jsonpb_too_many_args():
  proto.to_jsonpb(m, False, False)
assert.fails(to_jsonpb_too_many_args, 'to_jsonpb: got 3 arguments, want at most 2')

# None argument.
def to_jsonpb_with_none():
  proto.to_jsonpb(None)
assert.fails(to_jsonpb_with_none, 'to_jsonpb: for parameter msg: got NoneType, want proto.Message')

# Wrongly typed argument.
def to_jsonpb_with_int():
  proto.to_jsonpb(1)
assert.fails(to_jsonpb_with_int, 'to_jsonpb: for parameter msg: got int, want proto.Message')
