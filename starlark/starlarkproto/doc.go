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

// Package starlarkproto exposes protobuf messages as Starlark types.
//
// Internally a message is stored as a tree of Starlark values, with some type
// checking done when manipulating fields, based on proto message descriptors
// loaded dynamically from a serialized FileDescriptorSet.
//
// For example, reading or assigning to a field not defined in a message will
// cause a runtime error. Similarly, trying to assign a value of a wrong type to
// a non-repeated field will fail.
//
// Instantiating messages and default field values
//
// Each proto message in a loaded package is exposed via a constructor function
// that takes optional keyword arguments and produces a new object of *Message
// type.
//
// All unassigned fields are implicitly set to their default zero values on
// first access, including message-typed fields, lists and maps. It means, for
// example, if a message 'a' has a singular field 'b', that has a field 'c', it
// is always fine to write 'a.b.c' to read or set 'c' value, without explicitly
// checking that 'b' is set.
//
// To clear a field, assign None to it (regardless of its type).
//
// References and aliasing
//
// Messages are passed around everywhere by reference. In particular it is
// possible to have multiple fields pointing to the exact same message, e.g.
//
//    m1 = M()
//    m2 = M()
//    a = A()
//    m1.f = a
//    m2.f = a
//    a.i = 123  # changes both m1.f.i and m2.f.i
//
// Note that 'm1.f = a' assignment checks the type of 'a', it should either
// match type of 'f' identically (no duck typing), or be a dict or None (which
// will be converted to messages, see below).
//
// Working with repeated fields and maps
//
// Values of repeated fields are represented by special sequence types that
// behave as strongly-typed lists. Think of them as list[T] types or as
// hypothetical 'message List<T>' messages.
//
// When assigning a non-None value R to a repeated field F of type list[T],
// the following rules apply (sequentially):
//   1. If R is not a sequence => error.
//   2. If R has type list[T'], then
//      a. If T == T', then F becomes an alias of R.
//      b. If T != T' => error.
//   3. A new list[T] is instantiated from R and assigned to F.
//
// Notice that rule 2 is exactly like the aliasing rule for messages. This is
// where "think of them as 'message List<T>'" point of view comes into play.
//
// As a concrete example, consider this:
//
//    m1 = M()
//    m1.int64s = [1, 2]  # a list is implicitly converted (copied) to list[T]
//
//    m2 = M()
//    m2.int64s = m1.int64s  # points to the exact same list[T] object now
//    m2.int64s.append(3)    # updates both m1 and m2
//
//    m1.int32s = m1.int64s        # error! list[int64] is not list[int32]
//    m1.int32s = list(m1.int64s)  # works now (by making a copy of a list copy)
//
// Maps behave in the similar way. Think of them as strongly-typed map<K,V>
// values or as 'message List<Pair<K,V>>' messages. They can be implicitly
// instantiated from iterable mappings (e.g. dicts).
//
// Auto-conversion of dicts and None's into Messages
//
// When assigning to a message-typed value (be it a field, an element of a
// list[T] or a value of map<K,V>) dicts are implicitly converted into messages
// as if via 'T(**d)' call. Similarly, None's are converted into empty messages,
// as if via 'T()' call.
//
// Differences from starlarkproto (beside using different guts):
//    * Message types are instantiated through proto.new_loader().
//    * Text marshaller appends\removes trailing '\n' somewhat differently.
//    * Text marshaller marshals empty messages as '<>' (without line breaks).
//    * Bytes fields are represented as str, not as []uint8{...}.
//    * proto.to_jsonpb doesn't have 'emit_defaults' kwarg (it is always False).
//    * Better support for proto2 messages.
package starlarkproto

// TODO: support more known types (any, struct).
// TODO: delete struct_to_textpb, use dynamic protos instead
