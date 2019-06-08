// Copyright 2018 The LUCI Authors.
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

package starlarkproto

import (
	"fmt"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// ProtoLib returns a dict with single struct named "proto" that holds helper
// functions to manipulate protobuf messages (in particular serialize them).
//
// Exported functions:
//
//    def to_textpb(msg):
//      """Serializes a protobuf message to text proto.
//
//      Args:
//        msg: a *Message to serialize.
//
//      Returns:
//        An str representing msg in text format.
//      """
//
//    def to_jsonpb(msg, emit_defaults=False):
//      """Serializes a protobuf message to JSONPB string.
//
//      Args:
//        msg: a *Message to serialize.
//        emit_defaults: if True, do not omit fields with default values.
//
//      Returns:
//        An str representing msg in JSONPB format.
//      """
//
//    def from_textpb(ctor, text):
//      """Deserializes a protobuf message given in text proto form.
//
//      Unknown fields are not allowed.
//
//      Args:
//        ctor: a message constructor function.
//        text: a string with serialized message.
//
//      Returns:
//        Deserialized message constructed via `ctor`.
//      """
//
//    def from_jsonpb(ctor, text):
//      """Deserializes a protobuf message given as JBONPB string.
//
//      Unknown fields are silently skipped.
//
//      Args:
//        ctor: a message constructor function.
//        text: a string with serialized message.
//
//      Returns:
//        Deserialized message constructed via `ctor`.
//      """
func ProtoLib() starlark.StringDict {
	return starlark.StringDict{
		"proto": starlarkstruct.FromStringDict(starlark.String("proto"), starlark.StringDict{
			"to_textpb":   starlark.NewBuiltin("to_textpb", toTextPb),
			"to_jsonpb":   starlark.NewBuiltin("to_jsonpb", toJSONPb),
			"from_textpb": starlark.NewBuiltin("from_textpb", fromTextPb),
			"from_jsonpb": starlark.NewBuiltin("from_jsonpb", fromJSONPb),
		}),
	}
}

// toTextPb takes a single protobuf message and serializes it using text
// protobuf serialization.
func toTextPb(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg *Message
	if err := starlark.UnpackArgs("to_textpb", args, kwargs, "msg", &msg); err != nil {
		return nil, err
	}
	pb, err := msg.ToProto()
	if err != nil {
		return nil, err
	}
	return starlark.String(proto.MarshalTextString(pb)), nil
}

// toJSONPb takes a single protobuf message and serializes it using JSONPB
// serialization.
func toJSONPb(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg *Message
	var emitDefaults starlark.Bool
	if err := starlark.UnpackArgs("to_jsonpb", args, kwargs, "msg", &msg, "emit_defaults?", &emitDefaults); err != nil {
		return nil, err
	}
	pb, err := msg.ToProto()
	if err != nil {
		return nil, err
	}
	// More jsonpb Marshaler options may be added here as needed.
	var jsonMarshaler = &jsonpb.Marshaler{Indent: "\t", EmitDefaults: bool(emitDefaults)}
	str, err := jsonMarshaler.MarshalToString(pb)
	if err != nil {
		return nil, err
	}
	return starlark.String(str), nil
}

// fromTextPb takes a text-serialized proto messages and returns *Message.
func fromTextPb(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var ctor starlark.Value
	var text starlark.String
	if err := starlark.UnpackArgs("from_textpb", args, kwargs, "ctor", &ctor, "text", &text); err != nil {
		return nil, err
	}

	c, ok := ctor.(*messageCtor)
	if !ok {
		return nil, fmt.Errorf("from_textpb: got %s, expecting a proto message constructor", ctor.Type())
	}
	typ := c.typ

	protoMsg := typ.NewProtoMessage().Interface().(proto.Message)
	if err := proto.UnmarshalText(text.GoString(), protoMsg); err != nil {
		return nil, fmt.Errorf("from_textpb: %s", err)
	}

	msg := NewMessage(typ)
	if err := msg.FromProto(protoMsg); err != nil {
		return nil, fmt.Errorf("from_textpb: %s", err)
	}
	return msg, nil
}

// fromJSONPb takes a JSONPB-serialized proto messages and returns *Message.
func fromJSONPb(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var ctor starlark.Value
	var text starlark.String
	if err := starlark.UnpackArgs("from_jsonpb", args, kwargs, "ctor", &ctor, "text", &text); err != nil {
		return nil, err
	}

	c, ok := ctor.(*messageCtor)
	if !ok {
		return nil, fmt.Errorf("from_jsonpb: got %s, expecting a proto message constructor", ctor.Type())
	}
	typ := c.typ

	protoMsg := typ.NewProtoMessage().Interface().(proto.Message)
	u := jsonpb.Unmarshaler{AllowUnknownFields: true}
	if err := u.Unmarshal(strings.NewReader(text.GoString()), protoMsg); err != nil {
		return nil, fmt.Errorf("from_jsonpb: %s", err)
	}

	msg := NewMessage(typ)
	if err := msg.FromProto(protoMsg); err != nil {
		return nil, fmt.Errorf("from_jsonpb: %s", err)
	}
	return msg, nil
}
