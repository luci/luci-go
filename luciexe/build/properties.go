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

package build

import (
	"go.chromium.org/luci/luciexe/build/properties"
)

// Properties is the canonical registry for all `build` input and output
// properties.
var Properties = &properties.Registry{}

// RegisterProperty registers a proto message, a *struct or a map as the input
// and output schema for `namespace` in the Properties registry (in this package).
// The returned RegisteredProperty will allow you to read the decoded input or
// manipulate the output properties any time during the execution of the Build.
//
// See go.chromium.org/luci/luciexe/build/properties for more details, but as
// a quick reference:
//   - The namespace "" indicates that this will be the schema for the
//     'top level' property message.
//   - Multiple registrations for the same namespace will panic, even if the
//     schema types align. If you want multiple Go packages to have access to the
//     same region of the Build properties, consider registering the schema in
//     a single shared package, or exporting functions from that package to
//     interact with the properties at a higher level than just exposing raw
//     property access (which can potentially act as a global variable in your
//     program).
//
// `T` may be any of the following:
//   - A proto.Message type (e.g. *MyMessage), in which case it will be
//     serialized as JSONPB. *structpb.Struct is special-cased to have
//     pass-through encoding, in case your program needs to do something
//     particularly special with the raw *Struct message.
//   - A non proto.Message pointer-to-struct (e.g. *MyStruct), in which case it
//     will be serialized with encoding/json.
//   - A map[string]<something> (e.g. map[string]any), in which case it will be
//     serialized with encoding/json.
//
// Example:
//
//		package mypkg
//
//		// message MyProtoMsg {
//		//   string setting = 1;
//		//   repeated string output = 2;
//		// }
//		var ioProp = build.RegisterProperty[*MyProtoMsg]("$mypkg")
//
//		// ReadInitialSetting returns the value of Setting for mypkg that the build
//		// started with (i.e. was an input property).
//		func ReadInitialSetting(ctx context.Context) string {
//		  // returns exactly `*MyProtoMsg`. If there is no Build in `ctx`, this
//		  // returns nil.
//		  return ioProp.GetInput(ctx).GetSetting()
//		}
//
//		// AppendToOutput adds a string to mypkg's output.
//		func AppendToOutput(ctx context.Context, newVal string) {
//	    ioProp.MutateOutput(ctx, func(state *MyProtoMsg) (mutated bool) {
//	      state.Output = append(state.Output, newVal)
//	      return true  // will cause the build properties to be sent
//	    }
//		}
//
// This is just a shorthand (and summarized documentation) for:
//
//	properties.MustRegister[T](build.Properties, namespace, opts...)
func RegisterProperty[T any](namespace string, opts ...properties.RegisterOption) properties.RegisteredProperty[T, T] {
	return properties.MustRegister[T](Properties, namespace, append([]properties.RegisterOption{properties.OptSkipFrames(1)}, opts...)...)
}

// RegisterInputProperty registers a schema for this namespace, but ONLY for
// input.
func RegisterInputProperty[T any](namespace string, opts ...properties.RegisterOption) properties.RegisteredPropertyIn[T] {
	return properties.MustRegisterIn[T](Properties, namespace, append([]properties.RegisterOption{properties.OptSkipFrames(1)}, opts...)...)
}

// RegisterOutputProperty registers a schema for this namespace, but ONLY for
// output.
func RegisterOutputProperty[T any](namespace string, opts ...properties.RegisterOption) properties.RegisteredPropertyOut[T] {
	return properties.MustRegisterOut[T](Properties, namespace, append([]properties.RegisterOption{properties.OptSkipFrames(1)}, opts...)...)
}

// RegisterSplitProperty registers differing input and output schemas for this
// namespace.
func RegisterSplitProperty[InT, OutT any](namespace string, opts ...properties.RegisterOption) properties.RegisteredProperty[InT, OutT] {
	return properties.MustRegisterInOut[InT, OutT](Properties, namespace, append([]properties.RegisterOption{properties.OptSkipFrames(1)}, opts...)...)
}
