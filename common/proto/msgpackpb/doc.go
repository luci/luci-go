// Copyright 2022 The LUCI Authors.
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

// Package msgpackpb implements generic protobuf message serialization to
// msgpack.
//
// This library exists primarially to allow safe interchange between lua scripts
// running inside of a Redis instance, and external programs.
//
// It is intended to be fast and compact for lua to decode via cmsgpack
// (specifically, the version of cmsgpack which ships with Redis 5.1+).
//
// To avoid implementing a brand new versioning or encoding schemes while not
// totally sacrificing performance and storage cababilities (e.g. by using
// JSONPB), we lean on the versioning and compatibility features of protobufs,
// and so choose a scheme which can be derived entirely from the proto schema
// definition.
//
// The scheme works by encoding a message as a map of `field tag` to `value`.
//
// The value can be a message, a scalar (bool, int, uint, float, string), a list
// of messages or scalars, a map of (bool, int, uint, string) to messages or
// scalars.
//
//	message Foo {
//	  string field = 2;
//	  Foo recurse = 7;
//	}
//
// Would encode the instance `field: "hello" recurse {field: "hi"}}` as msgpack:
//
//	{
//	  2: "hello",
//	  7: {
//	    2: "hi"
//	  }
//	}
//
//	i.e. `94 02 a5 68 65 6c 6c 6f f9 92 02 a2 68 69`
//
// This would be 14 bytes vs 12 bytes for binary proto or 46 bytes for JSONPB.
//
// This encoding is simple enough that we can make a simple table based
// encoder/decoder in Lua, but robust enough that as long as everyone follows
// the backwards compatibility rules for proto, we should be OK when having Go
// and Lua interact with the same-encoded messages.
//
// # Unknown fields
//
// Unknown fields are saved in decoded protobufs as an unknown field tagged with
// INT32_MAX. The value of this field is essentially a filtered version of the
// Message; a map of `field tag` and `value`, but just for the unknown field
// tags.
//
// # Deterministic serialization
//
// This library optionally provides deterministic serialization of messages,
// which will:
//   - Sort all maps
//   - Order all messages by tag
//   - Emit all numeric types using the most compact representation in
//     msgpack.
//   - Walk all unknown fields, ordering messages there by tag, and interleaving
//     their field numbers with the known fields in sorted order.
//   - If any of the above would yield a map with integer keys from 1..N,
//     instead emit this as a list of just the values.
//
// It is intended that this encoding be stable across binary versions, and
// should be suitable for hashing (see MarshalStream; You can use io.MultiWriter
// to marshal to e.g. a strings.Builder at the same time that you write to
// a hash).
//
// # Notes
//
// NOTE: It could probably be a better idea to implement native protobuf
// encoding for Lua (and, in fact, there are Redis extensions for this), but we
// cannot currently use them with Cloud Memorystore, which is what manages our
// Redis instance. Writing a 'pure lua' protobuf codec seemed like it would be
// a time sink, but it could still be an option if this msgpack encoding proves
// difficult to work with.
//
// NOTE: An alternative to this package would be to create a bespoke encoding
// for your data objects, and include, e.g. a version identifier, to allow for
// schema updates. This is actually the initial approach that we took before
// writing this library, but we deemed this to be unwieldy and were concerned
// about having to worry both about proto change semantics and mapping them to
// the bespoke versioning semantics.
//
// NOTE: It would be possible to build a more efficient code generation
// marshalling implementation, but it presents a problem; Doing so would require
// generating serialization code for ALL proto messages which could be encoded,
// which includes references to externally declared proto messages (for example,
// google.protobuf.Duration). As such, a code generation approach would need to
// diverge pretty significantly from the 'usual' Go codegen model (i.e. where
// the generated code lives next to the protos that generate the code).
// Specifically, the generated code for external protos would need to be
// generated into a common library or something like that which lives separately
// from the protos.
//
// The other alternative would be to partially generate the marshalling code for
// protos which we own, but then fall back to a reflection based approach for
// messages which don't have the codegen encoding scheme. However this means
// that we would need a fully working codegen scheme AND ALSO a fully working
// reflection based scheme.
//
// For 'simplicity', we took the reflection-based approach as the initial and
// sole approach for now.
//
// NOTE: Unlike binary protobuf, msgpackpb messages cannot be concatenated to
// produce a single encodeable message. However, concatenated messages can
// accurately be parsed serially and applied to the same Message using
// UnmarshalStream.
//
// NOTE: Using this to interact with lua, keep in mind that lua (until 5.3)
// stores all numbers as double precision floating point (note that Redis looks
// like it's effectively stuck on lua 5.1 indefinitely, as of late 2022).
// The upshot of this is that integer types will be integers until they hit 2^52
// or so (assuming that your redis is using lua with 64bit numbers! It is
// possible to configure lua to use 32bit numbers...). If your lua program
// serializes a number past this threshold, Go will refuse to decode it into
// a field with an integer type, so at least this won't cause silent corruption.
//
// NOTE: Lua only has a single type to represent maps and lists; the 'table'.
// Additionally, the lua cmsgpack library will encode a table as a list if it
// 'looks like' a list. A table 'looks like' a list if it contains N entries and
// all the entries are keyed with the numbers 1 through N. This affects the
// encoding of both messages (which are map of field tag to value) as well as
// proto fields which are maps.
//
// NOTE: Should you need to switch to another encoding, note that because this
// encoding ALWAYS encodes a message, the msgpack 'type' of the first item in
// a stream will always be either a map or a list (due to lua table
// shenanigans). This means that you could insert a number into the stream as
// the first item, instead, and use this to disambiguate future versions of this
// encoding.
package msgpackpb
