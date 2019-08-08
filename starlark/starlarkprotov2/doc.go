// Copyright 2019 The LUCI Authors.
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

// Package starlarkprotov2 exposes protobuf messages as Starlark types.
//
// Uses a slightly modified vendored copy of "google.golang.org/protobuf"
// internally. It's expected that once "google.golang.org/protobuf" is
// officially released, starlarkprotov2 will switch to using the released
// version.
//
// Internally a message is stored as a tree of Starlark native values, with
// some type checking done when manipulating fields, based on proto message
// descriptors loaded dynamically from a serialized FileDescriptorSet.
//
// For example, reading or assigning to a field not defined in a message will
// cause a runtime error. Similarly, trying to assign a value of a wrong type to
// a non-repeated field will fail.
//
// Repeated fields have more lax type checks: they just have to be lists or
// tuples. It is possible to put wrong values inside them, which will cause
// runtime error at a later stage, when trying to serialize the proto message
// (or materialize it as proto.Message on the Go side).
//
// Instantiating messages and default field values
//
// Each proto message in a loaded package is exposed via constructor function
// that takes optional keyword arguments and produces a new object of *Message
// type.
//
// All unassigned fields are implicitly set to their default zero values on
// first access, including message-typed fields. It means, for example, if a
// message 'a' has a singular field 'b', that has a field 'c', it is always fine
// to write 'a.b.c' to read or set 'c' value, without explicitly checking that
// 'b' is set.
//
// To clear a field, assign None to it (regardless of its type).
//
// Differences from starlarkproto (beside using different guts):
//    * Message types are instantiated through proto.new_loader().
//    * Text marshaller appends\removes trailing '\n' somewhat differently.
//    * Text marshaller marshals empty messages as '<>' (without line breaks).
//    * Bytes fields are represented as str, not as []uint8{...}.
//    * proto.to_jsonpb doesn't have 'emit_defaults' kwarg (it is always False).
//    * Better support for proto2 messages.
package starlarkprotov2

// TODO: support more known types (any, struct).
// TODO: support '==' ?
// TODO: delete struct_to_textpb, use dynamic protos instead
