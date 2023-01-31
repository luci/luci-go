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

package msgpackpb

type options struct {
	deterministic        bool
	unknownFieldBehavior unknownFieldBehavior

	internUnmarshalTable []string
	internMarshalTable   map[string]uint
}

// Option allows modifying the behavior of Marshal and Unmarshal.
type Option func(*options)

// IgnoreUnknownFields is an Option which affects Marshal + Unmarshal.
//
// Marshal: Unknown msgpack fields on the proto message to be dropped
// (non-msgpack unknown fields will result in an error).
//
// Unmarshal: Skips unknown fields, and do not store them on the decoded proto message.
func IgnoreUnknownFields(o *options) {
	o.unknownFieldBehavior = ignoreUnknownFields
}

// DisallowUnknownFields is an Option which affects Marshal + Unmarshal.
//
// Marshal: Return an error when encoding proto messages containing any unknown
// fields.
//
// Unmarshal: Return an error when decoding messages containing unknown fields.
func DisallowUnknownFields(o *options) {
	o.unknownFieldBehavior = disallowUnknownFields
}

// Deterministic is an Option which affects Marshal.
//
// (Providing this to Unmarshal is an error).
//
// If set, the proto will be encoded with the following additional rules:
//   - All fields will be output ordered by their field tag number.
//   - Maps will sort keys (lexically or numerically)
//   - Any unknown msgpack fields will (if PreserveUnknownFields was given when
//     Unmarshalling) be interleaved with the known fields in order, sorting any
//     messages or maps.
func Deterministic(o *options) {
	o.deterministic = true
}

// WithStringInternTable lets you set an optional string internment table; Any
// strings encoded which are contained in this table will be replaced with an
// integer denoting the index in this table where the string was found.
//
// This includes repeated strings, map keys, and string value fields.
func WithStringInternTable(table []string) Option {
	return func(o *options) {
		o.internUnmarshalTable = table
		o.internMarshalTable = make(map[string]uint, len(table))
		for i, val := range table {
			o.internMarshalTable[val] = uint(i)
		}
	}
}

type unknownFieldBehavior byte

const (
	preserveUnknownFields unknownFieldBehavior = iota
	ignoreUnknownFields
	disallowUnknownFields
)
