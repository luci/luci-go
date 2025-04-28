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

package exe

import (
	"bytes"
	"encoding/json"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/errors"
)

// ParseProperties interprets a protobuf 'struct' as structured Go data.
//
// `outputs` is a mapping of a property name to an output structure. An output
// structure may be one of two things:
//
//   - a non-nil proto.Message. The data in this field will be interpreted as
//     JSONPB and Unmarshaled into the proto.Message.
//   - a valid "encoding/json" unmarshal target. The data in this field will be
//     unmarshaled into with the stdlib "encoding/json" package.
//
// This function will scan the props (usually `build.Input.Properties`) and
// unmarshal them as appropriate into the outputs.
//
// Example:
//
//	myProto := &myprotos.Message{}
//	myStruct := &MyStruct{}
//	err := ParseProperties(build.Input.Properties, map[string]any{
//	  "proto": myProto, "$namespaced/struct": myStruct})
//	// handle err :)
//	fmt.Println("Got:", myProto.Field)
//	fmt.Println("Got:", myStruct.Field)
func ParseProperties(props *structpb.Struct, outputs map[string]any) error {
	ret := errors.NewLazyMultiError(len(outputs))

	idx := -1
	for field, output := range outputs {
		idx++

		val := props.Fields[field]
		if val == nil {
			continue
		}

		var jsonBuf bytes.Buffer
		if err := (&jsonpb.Marshaler{}).Marshal(&jsonBuf, val); err != nil {
			ret.Assign(idx, errors.Annotate(err, "marshaling %q", field).Err())
			continue
		}

		var err error
		switch x := output.(type) {
		case proto.Message:
			err = jsonpb.Unmarshal(&jsonBuf, x)
		default:
			err = json.NewDecoder(&jsonBuf).Decode(x)
		}

		if err != nil {
			ret.Assign(idx, errors.Annotate(err, "unmarshalling %q", field).Err())
		}
	}

	return ret.Get()
}

type nullType struct{}

// Null is a sentinel value to assign JSON `null` to a property with
// WriteProperties.
var Null = nullType{}

// WriteProperties updates a protobuf 'struct' with structured Go data.
//
// `inputs` is a mapping of a property name to an input structure. An input
// structure may be one of two things:
//
//   - a non-nil proto.Message. The data in this field will be interpreted as
//     JSONPB and Unmarshaled into the proto.Message.
//   - a valid "encoding/json" marshal source. The data in this field will be
//     interpreted as json and marshaled with the stdlib "encoding/json" package.
//   - The `Null` value in this package. The top-level property will be set to
//     JSON `null`.
//   - nil. The top-level property will be removed.
//
// This function will scan the inputs and marshal them as appropriate into
// `props` (usually `build.Output.Properties`).
//
// Example:
//
//	myProto := &myprotos.Message{Field: "something"}
//	myStruct := &MyStruct{Field: 100}
//	err := WriteProperties(build.Output.Properties, map[string]any{
//	  "proto": myProto, "$namespaced/struct": myStruct})
//	// handle err :)
func WriteProperties(props *structpb.Struct, inputs map[string]any) error {
	if props.Fields == nil {
		props.Fields = make(map[string]*structpb.Value, len(inputs))
	}

	ret := errors.NewLazyMultiError(len(inputs))

	idx := -1
	for field, input := range inputs {
		idx++

		if input == nil {
			delete(props.Fields, field)
			continue
		}

		var buf bytes.Buffer
		var err error

		fieldVal := props.Fields[field]
		if fieldVal == nil {
			fieldVal = &structpb.Value{}
			props.Fields[field] = fieldVal
		}

		switch x := input.(type) {
		case nullType:
			fieldVal.Kind = &structpb.Value_NullValue{}
			continue

		case proto.Message:
			err = (&jsonpb.Marshaler{OrigName: true}).Marshal(&buf, x)

		default:
			var data []byte
			data, err = json.Marshal(x)
			buf.Write(data)
		}

		if err != nil {
			ret.Assign(idx, errors.Annotate(err, "marshaling %q", field).Err())
			continue
		}

		ret.Assign(idx, errors.Annotate(
			jsonpb.Unmarshal(&buf, fieldVal), "unmarshalling %q", field,
		).Err())
	}

	return ret.Get()
}
