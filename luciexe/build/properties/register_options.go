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

package properties

type registerOptions struct {
	skipFrames        int
	unknownFields     unknownFieldSetting
	jsonUseNumber     bool
	protoUseJSONNames bool
}

// unknownFieldSetting allows you to select the behavior of registered
// namespaces when they encounter unknown fields while parsing the initial
// state.
type unknownFieldSetting int

const (
	// rejectUnknownFields will cause the initial state to error out if it's asked
	// to parse any fields which aren't known by the Go struct or proto Message.
	//
	// By default (i.e. 0 value UnknownFieldSetting), we will reject unknown fields.
	//
	// This prevents accidental typos, etc. from being undetected.
	//
	// However, this means that migrations need to be handled with care:
	//   * When adding a field, add it to the binary, and deploy the binary,
	//   before adding the property to any configuration.
	//   * When removing a field, make sure to mark the name as reserved in proto,
	//   or keep the old field name around in the Go struct to mark it as
	//   deprecated. This will allow new binaries to continue to process
	//   properties aimed at older binaries.
	rejectUnknownFields unknownFieldSetting = iota

	// ignoreUnknownFields will cause the initial state to ignore any fields which
	// aren't known by the Go struct or proto Message.
	//
	// This makes migrations easier, but can also lead to typos being silently
	// undetected.
	ignoreUnknownFields
)

// A RegisterOption is the way to specify extra behavior when calling any of the
// (Must)?Register(In)?(Out)? functions.
type RegisterOption func(*registerOptions)

// OptSkipFrames returns a RegisterOption which allows you to skip additional
// frames when Register{Proto,Struct} walk the stack looking for the
// registration location.
//
// If supplied multiple times, the number of frames skipped accumulates (i.e.
// `OptSkipFrames(1), OptSkipFrames(1)` is the same as `OptSkipFrames(2)`
func OptSkipFrames(frames int) RegisterOption {
	return func(ro *registerOptions) {
		ro.skipFrames += frames
	}
}

// OptIgnoreUnknownFields is a RegisterOption which allows your registered proto
// or struct to ignore unknown fields when parsing the input value.
//
// By default this library will reject unknown fields when parsing the input,
// which prevents accidental typos, etc. from being undetected.
//
// However, this means that migrations need to be handled with care:
//   - When adding a field, add it to the binary, and deploy the binary,
//     before adding the property to any configuration.
//   - When removing a field, make sure to mark the name as reserved in proto,
//     or keep the old field name around in the Go struct to mark it as
//     deprecated. This will allow new binaries to continue to process
//     properties aimed at older binaries.
//
// Example:
//
//	message Msg {
//	  string some_field = 1;
//	}
//
//	// by default
//	{ "somefield": "hello" } => error
//
//	// with OptIgnoreUnknownFields()
//	{ "somefield": "hello" } => parses, but Msg.some_field will be ""
func OptIgnoreUnknownFields() RegisterOption {
	return func(ro *registerOptions) {
		ro.unknownFields = ignoreUnknownFields
	}
}

// OptProtoUseJSONNames is a RegisterOption, which will make Register use the
// protobuf default names when serializing proto.Message types. These names are:
//   - some_thing -> someThing
//   - some_thing [json_name = "wow"] -> wow
//
// By default, this library will use the 'proto' name, so fields in the JSON
// output appear exactly how the fields exist in in the .proto file. This
// generally causes much less confusion when working with JSONPB because what
// you see in the .proto file is what you get in the data which makes gerpping
// and debugging much easier.
//
// Regardless of this option, when deserializing from Struct, all possible field
// names are allowed (so, lowerCamelCase, lower_camel_case and json_name
// annotation).
//
// Ignored for non-proto messages.
func OptProtoUseJSONNames() RegisterOption {
	return func(ro *registerOptions) {
		ro.protoUseJSONNames = true
	}
}

// OptJSONUseNumber is a RegisterOption, which will make Register use the
// json.Number when decoding a number to an `any` as part of a Go struct or map.
//
// Ignored for non-json messages.
func OptJSONUseNumber() RegisterOption {
	return func(ro *registerOptions) {
		ro.jsonUseNumber = true
	}
}

func loadRegOpts(opts []RegisterOption) registerOptions {
	var ret registerOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&ret)
		}
	}
	return ret
}
