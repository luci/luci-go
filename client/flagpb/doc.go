// Copyright 2016 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package flagpb defines a flag format for protobuf messages,
// implements a parser and a formatter.
//
// Currently flagpb supports only untyped messages, not proto.Message.
// Support for the latter could be added too.
//
// Syntax
//
// Flag syntax by example. First line is flagpb, second is jsonpb.
//  -x=42 -b -s hello
//  {"x": 42, "b": true, "s": "hello"}
//
//  -m.x 3 -m.s world
//  {"m": {"x": 3, "s": "world"}}
//
//  -rx 1 -rx 2
//  {"rx": [1, 2]}
//
//  -rm.x 1 -rm -rm.x 2
//  {"rm": [{"x": 1}, {"x": 2}]}
//
// where x fields are int32, m are message fields, b are boolean and
// s are strings. Fields with "r" prefix are repeated.
//
// Bytes field values are decoded from hex, e.g. "FF02AB".
//
// Enum field values can be specified by enum member name or number.
package flagpb
