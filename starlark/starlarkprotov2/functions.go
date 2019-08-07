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

package starlarkprotov2

import (
	"bytes"
	"errors"
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/dynamicpb"
)

// ProtoLib returns a dict with single struct named "proto" that holds public
// Starlark API for working with proto messages.
//
// Exported symbols:
//
//    def new_loader(*raw_descriptor_sets):
//      """Returns a new proto loader."""
//
//    def to_textpb(msg):
//      """Serializes a protobuf message to text proto.
//
//      Args:
//        msg: a *Message to serialize.
//
//      Returns:
//        A str representing msg in text format.
//      """
//
//    def to_jsonpb(msg):
//      """Serializes a protobuf message to JSONPB string.
//
//      Args:
//        msg: a *Message to serialize.
//
//      Returns:
//        A str representing msg in JSONPB format.
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
//
//    def struct_to_textpb(s):
//      """Converts a struct to a text proto string.
//
//      Args:
//        s: a struct object. May not contain dicts.
//
//      Returns:
//        A str containing a text format protocol buffer message.
//      """
func ProtoLib() starlark.StringDict {
	return starlark.StringDict{
		"proto": starlarkstruct.FromStringDict(starlark.String("proto"), starlark.StringDict{
			"new_loader":       starlark.NewBuiltin("new_loader", newLoader),
			"to_textpb":        starlark.NewBuiltin("to_textpb", toTextPb),
			"to_jsonpb":        starlark.NewBuiltin("to_jsonpb", toJSONPb),
			"from_textpb":      starlark.NewBuiltin("from_textpb", fromTextPb),
			"from_jsonpb":      starlark.NewBuiltin("from_jsonpb", fromJSONPb),
			"struct_to_textpb": starlark.NewBuiltin("struct_to_textpb", structToTextPb),
		}),
	}
}

// newLoader constructs new Loader, seeding it with the given descriptor sets.
func newLoader(_ *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(kwargs) > 0 {
		return nil, errors.New("loader: unexpected keyword arguments")
	}
	raw := make([]string, len(args))
	for i, v := range args {
		s, ok := v.(starlark.String)
		if !ok {
			return nil, fmt.Errorf("loader: for parameter %d: got %s, want string", i+1, v.Type())
		}
		raw[i] = s.GoString()
	}
	l := NewLoader()
	for i, r := range raw {
		if err := l.AddDescriptorSet([]byte(r)); err != nil {
			return nil, fmt.Errorf("loader: for parameter %d: %s", i+1, err)
		}
	}
	return l, nil
}

// toTextPb takes a protobuf message and serializes it using text encoding.
func toTextPb(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg *Message
	if err := starlark.UnpackArgs("to_textpb", args, kwargs, "msg", &msg); err != nil {
		return nil, err
	}
	pb, err := msg.ToProto()
	if err != nil {
		return nil, err
	}
	opts := prototext.MarshalOptions{
		AllowPartial: true,
		Indent:       "  ",
		Resolver:     msg.typ.loader.types, // used for google.protobuf.Any fields
	}
	blob, err := opts.Marshal(pb)
	if err != nil {
		return nil, fmt.Errorf("to_textpb: %s", err)
	}
	return starlark.String(blob), nil
}

// toJSONPb takes a protobuf message and serializes it using JSON encoding.
func toJSONPb(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg *Message
	if err := starlark.UnpackArgs("to_jsonpb", args, kwargs, "msg", &msg); err != nil {
		return nil, err
	}
	pb, err := msg.ToProto()
	if err != nil {
		return nil, err
	}
	opts := protojson.MarshalOptions{
		AllowPartial: true,
		Indent:       "\t",
		Resolver:     msg.typ.loader.types, // used for google.protobuf.Any fields
	}
	blob, err := opts.Marshal(pb)
	if err != nil {
		return nil, fmt.Errorf("to_jsonpb: %s", err)
	}
	return starlark.String(blob), nil
}

// fromTextPb takes a text-serialized proto messages and returns *Message.
func fromTextPb(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var ctor starlark.Value
	var text starlark.String
	if err := starlark.UnpackArgs("from_textpb", args, kwargs, "ctor", &ctor, "text", &text); err != nil {
		return nil, err
	}

	typ, ok := ctor.(*MessageType)
	if !ok {
		return nil, fmt.Errorf("from_textpb: got %s, expecting a proto message constructor", ctor.Type())
	}

	// Unmarshal into a proto.Message.
	pb := dynamicpb.New(typ.desc)
	opts := prototext.UnmarshalOptions{
		AllowPartial: true,
		Resolver:     typ.loader.types, // used for google.protobuf.Any fields
	}
	if err := opts.Unmarshal([]byte(text.GoString()), pb); err != nil {
		return nil, fmt.Errorf("from_textpb: %s", err)
	}

	// Convert proto.Message into *Message.
	msg := typ.NewMessage()
	if err := msg.FromProto(pb); err != nil {
		return nil, fmt.Errorf("from_textpb: %s", err)
	}
	return msg, nil
}

// fromJSONPb takes a JSON-serialized proto messages and returns *Message.
func fromJSONPb(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var ctor starlark.Value
	var text starlark.String
	if err := starlark.UnpackArgs("from_jsonpb", args, kwargs, "ctor", &ctor, "text", &text); err != nil {
		return nil, err
	}

	typ, ok := ctor.(*MessageType)
	if !ok {
		return nil, fmt.Errorf("from_jsonpb: got %s, expecting a proto message constructor", ctor.Type())
	}

	// Unmarshal into a proto.Message.
	pb := dynamicpb.New(typ.desc)
	opts := protojson.UnmarshalOptions{
		AllowPartial:   true,
		DiscardUnknown: true,
		Resolver:       typ.loader.types, // used for google.protobuf.Any fields
	}
	if err := opts.Unmarshal([]byte(text.GoString()), pb); err != nil {
		return nil, fmt.Errorf("from_jsonpb: %s", err)
	}

	// Convert proto.Message into *Message.
	msg := typ.NewMessage()
	if err := msg.FromProto(pb); err != nil {
		return nil, fmt.Errorf("from_jsonpb: %s", err)
	}
	return msg, nil
}

// structToTextPb takes a struct and returns a string containing a text format protocol buffer.
func structToTextPb(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var val starlark.Value
	if err := starlark.UnpackArgs("struct_to_textpb", args, kwargs, "struct", &val); err != nil {
		return nil, err
	}
	s, ok := val.(*starlarkstruct.Struct)
	if !ok {
		return nil, fmt.Errorf("struct_to_textpb: got %s, expecting a struct", val.Type())
	}
	var buf bytes.Buffer
	err := writeProtoStruct(&buf, 0, s)
	if err != nil {
		return nil, err
	}
	return starlark.String(buf.String()), nil
}

// Based on
// https://github.com/google/starlark-go/blob/32ce6ec36500ded2e2340a430fae42bc43da8467/starlarkstruct/struct.go
func writeProtoStruct(out *bytes.Buffer, depth int, s *starlarkstruct.Struct) error {
	for _, name := range s.AttrNames() {
		val, err := s.Attr(name)
		if err != nil {
			return err
		}
		if err = writeProtoField(out, depth, name, val); err != nil {
			return err
		}
	}
	return nil
}

func writeProtoField(out *bytes.Buffer, depth int, field string, v starlark.Value) error {
	if depth > 16 {
		return fmt.Errorf("to_proto: depth limit exceeded")
	}

	switch v := v.(type) {
	case *starlarkstruct.Struct:
		fmt.Fprintf(out, "%*s%s: <\n", 2*depth, "", field)
		if err := writeProtoStruct(out, depth+1, v); err != nil {
			return err
		}
		fmt.Fprintf(out, "%*s>\n", 2*depth, "")
		return nil

	case *starlark.List, starlark.Tuple:
		iter := starlark.Iterate(v)
		defer iter.Done()
		var elem starlark.Value
		for iter.Next(&elem) {
			if err := writeProtoField(out, depth, field, elem); err != nil {
				return err
			}
		}
		return nil
	}

	// scalars
	fmt.Fprintf(out, "%*s%s: ", 2*depth, "", field)
	switch v := v.(type) {
	case starlark.Bool:
		fmt.Fprintf(out, "%t", v)

	case starlark.Int:
		out.WriteString(v.String())

	case starlark.Float:
		fmt.Fprintf(out, "%g", v)

	case starlark.String:
		fmt.Fprintf(out, "%q", string(v))

	default:
		return fmt.Errorf("struct_to_textpb: cannot convert %s to proto", v.Type())
	}
	out.WriteByte('\n')
	return nil
}
