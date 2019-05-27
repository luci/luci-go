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

m = testprotos.Simple()

# Works in general.
m.i = 123
assert.eq(proto.to_jsonpb(m), """{
	"i": "123"
}""")

# Prints default values when requested.
# Just do a "contain" rather than eq so that this test doesn't break every time
# testProtos.Simple gets a new field.
m.i = 0
assert.contains(proto.to_jsonpb(m, emit_defaults=True), '"i": "0",')

# Doesn't print default values when 'emit_defaults' is False.
m.i = 0
assert.eq(proto.to_jsonpb(m, emit_defaults=False), """{

}""")

# to_jsonpb expects an argument.
def to_jsonpb_no_args():
  proto.to_jsonpb()
assert.fails(to_jsonpb_no_args, 'missing argument for msg')

# Too many arguments.
def to_jsonpb_too_many_args():
  proto.to_jsonpb(m, m, m)
assert.fails(to_jsonpb_too_many_args, 'to_jsonpb: got 3 arguments, want at most 2')

# None argument.
def to_jsonpb_with_none():
  proto.to_jsonpb(None)
# TODO(tikuta): comment in after importing
# https://github.com/google/starlark-go/commit/daf30b69ce81217a37e152ea3cd2144d83dadacf
# assert.fails(to_jsonpb_with_none, 'to_jsonpb: for parameter msg: got NoneType, want proto.Message')

# Wrongly typed argument.
def to_jsonpb_with_int():
  proto.to_jsonpb(1)
# TODO(tikuta): comment in after importing
# https://github.com/google/starlark-go/commit/daf30b69ce81217a37e152ea3cd2144d83dadacf
# assert.fails(to_jsonpb_with_int, 'to_jsonpb: for parameter msg: got int, want proto.Message')
