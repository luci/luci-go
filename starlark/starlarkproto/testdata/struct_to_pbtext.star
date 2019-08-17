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


# Empty struct.
assert.eq(proto.struct_to_textpb(struct()), '')

# All the supported field types.
assert.eq(
  proto.struct_to_textpb(struct(
    int=1, float=3.141, str="hello", bool=True, intlist=[1, 2, 3])),
  '''bool: true
float: 3.141
int: 1
intlist: 1
intlist: 2
intlist: 3
str: "hello"
''')

# Nested structs.
assert.eq(
  proto.struct_to_textpb(struct(
    x=struct(), y=[struct(a=1), struct(b=2, c=struct(p=1, q=2))])),
  '''x: <
>
y: <
  a: 1
>
y: <
  b: 2
  c: <
    p: 1
    q: 2
  >
>
''')

# Dict fields not supported
def struct_to_textpb_dict_field():
  proto.struct_to_textpb(struct(d={1: 'one'}))
assert.fails(struct_to_textpb_dict_field, 'struct_to_textpb: cannot convert dict to proto')

# struct_to_textpb expects an argument.
assert.fails(proto.struct_to_textpb, 'missing argument for struct')

# Too many arguments.
def struct_to_textpb_too_many_args():
  proto.struct_to_textpb(struct(), struct())
assert.fails(struct_to_textpb_too_many_args, 'struct_to_textpb: got 2 arguments, want at most 1')

# None argument.
def struct_to_textpb_with_none():
  proto.struct_to_textpb(None)
assert.fails(struct_to_textpb_with_none, 'struct_to_textpb: got NoneType, expecting a struct')

# Wrongly typed argument.
def struct_to_textpb_with_int():
  proto.struct_to_textpb(1)
assert.fails(struct_to_textpb_with_int, 'struct_to_textpb: got int, expecting a struct')
