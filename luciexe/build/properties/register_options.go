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

import (
	"reflect"

	"go.chromium.org/luci/common/errors"
)

type registerOptions struct {
	skipFrames        int
	unknownFields     unknownFieldSetting
	jsonUseNumber     bool
	protoUseJSONNames bool
	strictTopLevel    bool
}

// unknownFieldSetting allows you to select the behavior of registered
// namespaces when they encounter unknown fields while parsing the initial
// state.
type unknownFieldSetting int

const (
	// logUnknownFields will cause the initial state parse to log WARNINGS for any
	// fields that are unrecognized which aren't known by the Go struct or proto
	// Message.
	logUnknownFields unknownFieldSetting = iota

	// rejectUnknownFields will cause the initial state to error out if it's asked
	// to parse any fields which aren't known by the Go struct or proto Message.
	//
	// Using rejectUnknownFields prevents accidental typos, etc. from being
	// undetected.
	//
	// However, this means that migrations need to be handled with care:
	//   * When adding a field, add it to the binary, and deploy the binary,
	//   before adding the property to any configuration.
	//   * When removing a field, make sure to mark the name as reserved in proto,
	//   or keep the old field name around in the Go struct to mark it as
	//   deprecated. This will allow new binaries to continue to process
	//   properties aimed at older binaries.
	rejectUnknownFields
)

// A RegisterOption is the way to specify extra behavior when calling any of the
// (Must)?Register(In)?(Out)? functions.
type RegisterOption func(opts *registerOptions, namespace string, inT, outT reflect.Type) error

// OptSkipFrames returns a RegisterOption which allows you to skip additional
// frames when Register{Proto,Struct} walk the stack looking for the
// registration location.
//
// If supplied multiple times, the number of frames skipped accumulates (i.e.
// `OptSkipFrames(1), OptSkipFrames(1)` is the same as `OptSkipFrames(2)`
func OptSkipFrames(frames int) RegisterOption {
	return func(opts *registerOptions, namespace string, inT, outT reflect.Type) error {
		opts.skipFrames += frames
		return nil
	}
}

// OptRejectUnknownFields returns a RegisterOption which allows your registered
// proto or struct to re'ect unknown fields when parsing the input value, rather
// than just logging them as WARNINGS.
//
// By default this library will log warnings for all unknown fields when parsing
// the input, which prevents accidental typos, etc. from being completely
// undetected, but sometimes you need a guard which is even stricter than just
// logging warnings.
//
// However, setting this option means that migrations need to be handled with care:
//   - When adding a field, you MUST add it to the binary, and deploy ALL
//     AFFECTED binaries, before setting the field.
//   - When removing a field, make sure to mark the name as reserved in proto,
//     or keep the old field name around in the Go struct to mark it as
//     deprecated. This will allow new binaries to continue to process
//     properties set for older binaries.
//
// Example:
//
//	message Msg {
//	  string some_field = 1;
//	}
//
//	// by default
//	{ "somefield": "hello" } => logs WARNING, Msg.some_field will be ""
//
//	// with OptRejectUnknownFields()
//	{ "somefield": "hello" } => logs ERROR and quits
//
// Error for output-only properties.
func OptRejectUnknownFields() RegisterOption {
	return func(opts *registerOptions, namespace string, inT, outT reflect.Type) error {
		if inT == nil {
			return errors.New(`OptRejectUnknownFields is not compatible with output-only properties`)
		}
		opts.unknownFields = rejectUnknownFields
		return nil
	}
}

// OptStrictTopLevelFields returns a RegisterOption which changes the way that
// top level fields are handled.
//
// By default, any top-level fields not parsed by the top-level namespace are
// ignored if they start with "$". All (Must)?Register(In)?(Out)? functions
// enforce that registered namespaces are either "", or begin with a "$". This
// default is a compromise between completely ignoring typo inputs and also
// causing builds to fail when passed configuration for a Go module that they
// happen to not use.
//
// By specifying OptStrictTopLevelFields, ALL extra top-level fields not parsed
// by the top-level namespace will be errors. Implies OptRejectUnknownFields().
//
// Error if used on non-top-level registrations.
// Error if top-level input type is a map type.
// Error for output-only properties.
func OptStrictTopLevelFields() RegisterOption {
	return func(opts *registerOptions, namespace string, inT, outT reflect.Type) error {
		if namespace != "" {
			return errors.Fmt(`OptStrictTopLevelFields is not compatible with namespace %q`, namespace)
		}
		if inT == nil {
			return errors.New(`OptStrictTopLevelFields is not compatible with output-only properties`)
		}
		if inT.Kind() == reflect.Map {
			return errors.Fmt(`OptStrictTopLevelFields is not compatible with type %s`, inT)
		}
		opts.strictTopLevel = true
		return OptRejectUnknownFields()(opts, namespace, inT, outT)
	}
}

// OptProtoUseJSONNames returns a RegisterOption, which will make Register use the
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
// Error for input-only properties.
// Error for non-proto messages.
func OptProtoUseJSONNames() RegisterOption {
	return func(opts *registerOptions, namespace string, inT, outT reflect.Type) error {
		if outT == nil {
			return errors.New("OptProtoUseJSONNames is not compatible with input-only properties")
		}
		if !outT.Implements(protoMessageType) {
			return errors.Fmt("OptProtoUseJSONNames is not compatible with non-proto type %s", outT)
		}
		opts.protoUseJSONNames = true
		return nil
	}
}

// OptJSONUseNumber is a RegisterOption, which will make Register use the
// json.Number when decoding a number to an `any` as part of a Go struct or map.
//
// Error for non-JSON messages.
// Error for output-only properties.
func OptJSONUseNumber() RegisterOption {
	return func(opts *registerOptions, namespace string, inT, outT reflect.Type) error {
		if inT == nil {
			return errors.New("OptJSONUseNumber is not compatible with output-only properties")
		}
		if inT.Implements(protoMessageType) {
			return errors.Fmt("OptJSONUseNumber is not compatible with proto %s", inT)
		}
		opts.jsonUseNumber = true
		return nil
	}
}

func loadRegOpts(namespace string, opts []RegisterOption, inT, outT reflect.Type) (registerOptions, error) {
	var ret registerOptions
	for _, opt := range opts {
		if opt != nil {
			if err := opt(&ret, namespace, inT, outT); err != nil {
				return ret, err
			}
		}
	}
	return ret, nil
}
