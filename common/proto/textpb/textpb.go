// Copyright 2021 The LUCI Authors.
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

// Package textpb can reformat text protos to be prettier.
//
// It is needed because "google.golang.org/protobuf/encoding/prototext"
// intentionally produces unstable output (inserting spaces in random places)
// and it very zealously escapes JSON-valued fields making them unreadable.
package textpb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/protocolbuffers/txtpbfmt/ast"
	"github.com/protocolbuffers/txtpbfmt/parser"
	"github.com/protocolbuffers/txtpbfmt/unquote"

	"go.chromium.org/luci/common/errors"
	luciproto "go.chromium.org/luci/common/proto"
)

// Format reformats a text proto of the given type to be prettier.
//
// Normalizes whitespaces and converts JSON-valued fields to be multi-line
// string literals instead of one giant string with "\n" inside. A string field
// can be annotated as containing JSON via field options:
//
//	import "go.chromium.org/luci/common/proto/options.proto";
//
//	message MyMessage {
//	  string my_field = 1 [(luci.text_pb_format) = JSON];
//	}
func Format(blob []byte, desc protoreflect.MessageDescriptor) ([]byte, error) {
	nodes, err := parser.ParseWithConfig(blob, parser.Config{
		SkipAllColons: true,
	})
	if err != nil {
		return nil, err
	}
	if err := transformTextPBAst(nodes, desc); err != nil {
		return nil, err
	}
	return []byte(parser.Pretty(nodes, 0)), nil
}

var marshalOpts = prototext.MarshalOptions{AllowPartial: true, Indent: " "}

// Marshal marshals the message into a pretty textproto.
//
// Uses the global protoregistry.GlobalTypes resolved. If you need to use
// a custom one, use "google.golang.org/protobuf/encoding/prototext" to marshal
// the message into a non-pretty form and the pass the result through Format.
func Marshal(m proto.Message) ([]byte, error) {
	blob, err := marshalOpts.Marshal(m)
	if err != nil {
		return nil, err
	}
	return Format(blob, m.ProtoReflect().Descriptor())
}

func transformTextPBAst(nodes []*ast.Node, desc protoreflect.MessageDescriptor) error {
	for _, node := range nodes {
		if err := transformTextPBNode(node, desc, ""); err != nil {
			return err
		}
	}
	return nil
}

func transformTextPBNode(node *ast.Node, desc protoreflect.MessageDescriptor, parentName string) error {
	fDesc := desc.Fields().ByName(protoreflect.Name(node.Name))
	if fDesc == nil {
		return errors.Reason("could not find field %q", node.Name).Err()
	}
	var format luciproto.TextPBFieldFormat
	if opts, ok := fDesc.Options().(*descriptorpb.FieldOptions); ok {
		format = proto.GetExtension(opts, luciproto.E_TextPbFormat).(luciproto.TextPBFieldFormat)
	}
	switch format {
	case luciproto.TextPBFieldFormat_JSON:
		if err := jsonTransformTextPBNode(node, parentName); err != nil {
			return err
		}
	}
	for _, child := range node.Children {
		if err := transformTextPBNode(child, fDesc.Message(), name(parentName, node)); err != nil {
			return err
		}
	}
	return nil
}

var quoteSwapper = strings.NewReplacer("'", "\"", "\"", "'")

func jsonTransformTextPBNode(node *ast.Node, parentName string) error {
	for _, value := range node.Values {
		if !isString(value.Value) {
			return nil
		}
	}
	s, _, err := unquote.Unquote(node)
	if err != nil {
		return errors.Annotate(err, "internal error: could not parse value for '%s' as string", name(parentName, node)).Err()
	}
	buf := &bytes.Buffer{}
	if err := json.Indent(buf, []byte(s), "", "  "); err != nil {
		return errors.Annotate(err, "value for '%s' must be valid JSON, got value '%s'", name(parentName, node), s).Err()
	}
	lines := strings.Split(buf.String(), "\n")
	values := make([]*ast.Value, 0, len(lines))
	for _, line := range lines {
		// Using single quotes for each string reduces the line noise by
		// preventing the double quotes (of which there are many) from
		// having to be escaped. There isn't a function to get a
		// single-quoted string, so swap the single and double quotes,
		// quote the string and then swap the quotes back
		line = quoteSwapper.Replace(strconv.Quote(quoteSwapper.Replace(line)))
		values = append(values, &ast.Value{Value: line})
	}
	node.Values = values
	return nil
}

func isString(s string) bool {
	return len(s) >= 2 &&
		(s[0] == '"' || s[0] == '\'') &&
		s[len(s)-1] == s[0]
}

func name(parentName string, node *ast.Node) string {
	if parentName == "" {
		return node.Name
	}
	return fmt.Sprintf("%s.%s", parentName, node.Name)
}
