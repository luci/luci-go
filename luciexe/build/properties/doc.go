// Copyright 2024 The LUCI Authors.
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

// Package properties encapsulates all logic and data structures for parsing the
// input properties and manipulating the output properties of a LUCI Build.
//
// LUCI Build input and output properties are proto Struct objects (effectively
// JSON objects). As such, they have very little type/schema information, and
// are cumbersome to directly manipulate/interact with.
//
// Additionally, both reads and writes to the property values must be
// synchronized within the process and ideally be type-safe (e.g. to prevent
// manipulating properties in ways which change their schema from one area of
// the program to another - say a key which is sometimes an number and sometimes
// a list).
//
// This package provides:
//   - transparent, type-safe methods for reading and writing property data
//     to/from proto Message classes as well as Go structs.
//   - Goroutine-safe manipulation and sending support.
//   - Ability to instantiate `properties` in Context apart from an entire LUCI
//     Build, meaning that code can interact with properties for input and output
//     without any other LUCI/luciexe mechanisms (highly useful for tests).
//
// # Data Model
//
// Logically input and output `properties` exist as single JSON objects. This
// package divides that object into multiple type-safe regions via a Registry.
// If you are using this with the [go.chromium.org/luci/luciexe/build] library,
// see [go.chromium.org/luci/luciexe/build.Properties].
//
// The singular Input or Output struct in a LUCI Build are allowed a schema to
// describe the 'top-level', minus any keys described by a registered namespace.
//
// For example:
//
//	{
//	  "some_key": 100,
//	  "other key": {
//	     "sub": "hello"
//	  },
//	  "another": { "lst": [1, 2, 3] }
//	}
//
// Could be broken into 3 schemas:
//
//	message TopLevel {  // registered to "", meaning top-level
//		int some_key = 1;
//	}
//
//	type OtherStruct struct {  // registered to "other key"
//	  Sub string `json:"sub"`
//	}
//
//	message Another {  // registered to "another"
//	  repeated int lst = 1;
//	}
//
// # Property Types
//
// All of the (Must)?Register(In)?(Out)? functions have additional restrictions
// on the types that they accept which cannot currently be expressed in terms of
// Go's generics.
//
// InT, OutT or T must be one of the following (in order of preference):
//   - A proto.Message (e.g. [Register][*MyProtoMessage]) - this will be
//     encoded with [google.golang.org/protobuf/encoding/protojson].
//   - A pointer to a struct type (e.g. [Register][*MyStruct]) - this will be
//     encoded with [encoding/json].
//   - A map (e.g. [Register][map[string]any]) with a string-like, int-like, or
//     [encoding.TextUnmarshaler] key to any type that [encoding/json] can decode
//     into.
//
// Using [*google.golang.org/protobuf/types/known/structpb.Struct] directly
// implements a pass-through encoding. This will allow you to register
// properties with ~zero overhead where you need to do something even more
// custom than using protojson or encoding/json. Note that working with Struct
// directly can be extremely annoying, so YMMV :).
//
// When parsing input properties, unknown fields can be ignored with
// OptIgnoreUnknownFields().
//
// If the top level type is a *structpb.Struct or a map type, it will simply
// collect all otherwise-unaccounted-for top-level keys. In this case, if
// state.Serialize() would case a top-level property to be overwritten by
// a namespace, it will return an error.
//
// # Namespaces
//
// All of the (Must)?Register(In)?(Out)? functions take a required 'namespace'
// argument. This is a key within the top-level set of input or output
// properties.
//
// If namespace is "", this will be the property namespace for the 'top level'
// property message, otherwise this is a namespace inside the top level
// properties.
//
// # Proto vs JSON tradeoffs
//
// Recommendation: Always prefer Protos where you can, except where you are 100%
// certain that you will never need to interop with other programs. JSON maps
// and *structpb.Struct can be used to ease migration from JSON -> Proto.
//
// Protos have the advantage that they can be generated for any language and are
// logically independent of the Go program source. This means that other
// programs can safely interact (write input properties, read output properties)
// with this Go program without needing to share source with this program.
//
// However, protos can be cumbersome to generate (though the `cproto` helper in
// LUCI makes this much easier to manage) vs 'just' Go structs, and sometimes
// you really do just need something quick.
//
// Think carefully about how your property messages will be used, how migrating
// from one format to another could be painful, etc. before picking one. If you
// are unsure, I would recommend to just use a Proto message.
package properties
