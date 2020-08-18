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

package exeutil

import (
	"bytes"
	"encoding/json"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	structpb "github.com/golang/protobuf/ptypes/struct"

	"go.chromium.org/luci/common/errors"
)

// ParseSingleProperty interprets a protobuf 'struct' value as structured Go
// data.
//
// `prop` must be a *structpb.Struct or *structpb.Value.
//
// `val` is an output structure which may be one of two things:
//
//  * a non-nil proto.Message. The data in this field will be interpreted as
//    JSONPB and Unmarshaled into the proto.Message.
//  * a valid "encoding/json" unmarshal target. The data in this field will be
//    unmarshaled into with the stdlib "encoding/json" package.
//
// This function will marshal the Struct (or Value) to JSON, then decode it
// back into `val`.
//
// Example:
//
//   myProto := &myprotos.Message{}
//   myStruct := &MyStruct{}
//   ParseSingleProperty(build.Input.Properties["myProto"], myProto)
//   ParseSingleProperty(build.Input.Properties["myStruct"], myStruct)
//   // handle errs :)
//   fmt.Println("Got:", myProto.Field)
//   fmt.Println("Got:", myStruct.Field)
//
// See also ParseProperties which can parse multiple properties from a Struct.
func ParseSingleProperty(structOrValue proto.Message, val interface{}) error {
	switch structOrValue.(type) {
	case *structpb.Struct, *structpb.Value:
	default:
		return errors.Reason(
			"unexpected type %T: wanted *Struct or *Value", structOrValue).Err()
	}

	var jsonBuf bytes.Buffer
	if err := (&jsonpb.Marshaler{}).Marshal(&jsonBuf, structOrValue); err != nil {
		return err
	}

	var err error
	switch x := val.(type) {
	case proto.Message:
		err = jsonpb.Unmarshal(&jsonBuf, x)
	default:
		err = json.NewDecoder(&jsonBuf).Decode(x)
	}

	return err
}

// ParseProperties interprets a protobuf 'struct' as structured Go data.
//
// `outputs` is a mapping of a property name to an output structure. An output
// structure may be anything defined as such by ParseSingleProperty.
//
// If `outputs` defines fields which do not exist in `props`, they are ignored.
// If `props` defines fields which do not exist in `outputs`, they are ignored.
func ParseProperties(props *structpb.Struct, outputs map[string]interface{}) error {
	ret := errors.NewLazyMultiError(len(outputs))

	idx := -1
	for field, output := range outputs {
		idx++

		val := props.Fields[field]
		if val == nil {
			continue
		}

		if err := ParseSingleProperty(val, output); err != nil {
			ret.Assign(idx, errors.Annotate(err, "parsing %q", field).Err())
		}
	}

	return ret.Get()
}

type nullType struct{}

// Null is a sentinel value to assign JSON `null` to a property with
// WriteProperties.
var Null = nullType{}

// SerializeProperty serializes `value` to a structpb Value.
//
// `value` may be one of:
//
//  * a non-nil proto.Message. The returned Value will contain the JSONPB
//    encoding of this message.
//  * a valid "encoding/json" marshal source. The returned Value will contain
//    the equivalent JSON encoding of this object.
//  * The `Null` value in this package. The returned Value will contain a JSON
//    `null`.
//  * `nil`. The returned Value will be `nil`.
//
// Example:
//
//   myProto := &myprotos.Message{Field: "something"}
//   myProtoAsValue, _ := SerializeProperty(myProto)
//
//   myStruct := &MyStruct{Field: 100}
//   myStructAsValue, _ := SerializeProperty(myStruct)
//
//   // handle errs :)
func SerializeProperty(value interface{}) (*structpb.Value, error) {
	switch value.(type) {
	case nullType:
		return &structpb.Value{Kind: &structpb.Value_NullValue{}}, nil

	case nil:
		return nil, nil
	}

	var buf bytes.Buffer
	var err error
	ret := &structpb.Value{}

	switch x := value.(type) {
	case proto.Message:
		err = (&jsonpb.Marshaler{OrigName: true}).Marshal(&buf, x)

	default:
		var data []byte
		data, err = json.Marshal(x)
		buf.Write(data)
	}

	if err != nil {
		return nil, err
	}

	return ret, jsonpb.Unmarshal(&buf, ret)
}

// WriteSingleProperty is a helper function to rewrite a Struct with the
// serialization of a single 'struct-like' input (either a Go struct or
// a proto.Message).
//
// Returns an error if `input` is not struct-like.
func WriteSingleProperty(singleProp *structpb.Struct, input interface{}) error {
	val, err := SerializeProperty(input)
	if err != nil {
		return err
	}
	structVal := val.GetStructValue()
	if structVal == nil {
		return errors.Reason("object %T is not struct-like", input).Err()
	}
	singleProp.Fields = structVal.Fields
	return nil
}

// WriteProperties updates a protobuf 'struct' with structured Go data.
//
// `inputs` is a mapping of a property name to an input structure. An input
// structure may be any of the types that SerializeProperty defines. An input
// structure of the literal value `nil` will remove that entry from `props`, if
// present.
//
// If `inputs` defines fields which do not exist in `props`, they are ignored.
// If `props` defines fields which do not exist in `inputs`, they are ignored.
func WriteProperties(props *structpb.Struct, inputs map[string]interface{}) error {
	if props.Fields == nil {
		props.Fields = make(map[string]*structpb.Value, len(inputs))
	}

	ret := errors.NewLazyMultiError(len(inputs))

	idx := -1
	for field, input := range inputs {
		idx++

		newField, err := SerializeProperty(input)
		if err != nil {
			ret.Assign(idx, errors.Annotate(err, "serializing %q", field).Err())
			continue
		}

		if newField == nil {
			delete(props.Fields, field)
		} else {
			props.Fields[field] = newField
		}
	}

	return ret.Get()
}
