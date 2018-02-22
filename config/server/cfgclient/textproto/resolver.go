// Copyright 2016 The LUCI Authors.
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

// Package textproto implements a textproto config service Resolver.
//
// It uses the "luci-go" text protobuf multi-line extension. For more
// information, see: go.chromium.org/luci/common/proto
//
// The textproto protobuf Resolver internally formats its content as a binary
// protobuf, rather than its raw text content. This has some advantages over raw
// text content caching:
//	- More space-efficient.
//	- Decodes faster.
//	- If the config service protobuf schema differs from the application's
//	  compiled schema, and the schema change is responsible (adding, renaming,
//	  repurposing) the binary cache value will continue to load where the text
//	  protobuf would fail.
package textproto

import (
	"bytes"
	"reflect"
	"strings"

	"go.chromium.org/luci/common/data/cmpbin"
	"go.chromium.org/luci/common/errors"
	luciProto "go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/backend"
	"go.chromium.org/luci/config/server/cfgclient/backend/format"

	"github.com/golang/protobuf/proto"
)

var typeOfProtoMessage = reflect.TypeOf((*proto.Message)(nil)).Elem()

// BinaryFormat is the resolver's binary protobuf format string.
const BinaryFormat = "go.chromium.org/luci/server/config/TextProto:binary"

// init registers this format in the registry of formats.
func init() {
	registerFormat()
}

func registerFormat() {
	format.Register(BinaryFormat, &Formatter{})
}

// Message is a cfgclient.Resolver that resolves config data into proto.Message
// instances by parsing the config data as a text protobuf.
func Message(out proto.Message) interface {
	cfgclient.Resolver
	cfgclient.FormattingResolver
} {
	if out == nil {
		panic("cannot pass a nil proto.Message instance")
	}
	return &resolver{
		messageName: proto.MessageName(out),
		single:      out,
		singleType:  reflect.TypeOf(out),
	}
}

// Slice is a cfgclient.MultiResolver which resolves a slice of configurations
// into a TextProto.
//
// out must be a pointer to a slice of some proto.Message implementation. If it
// isn't, this function will panic.
//
// For example:
//
//	var out []*MyProtoMessages
//	TextProtoSlice(&out)
func Slice(out interface{}) interface {
	cfgclient.MultiResolver
	cfgclient.FormattingResolver
} {
	r := resolver{}

	v := reflect.ValueOf(out)
	if !r.loadMulti(v) {
		panic(errors.Reason("%s is not a pointer to a slice of protobuf message types", v.Type()).Err())
	}
	return &r
}

type resolver struct {
	messageName string

	single     proto.Message
	singleType reflect.Type

	multiDest reflect.Value
}

func (r *resolver) loadMulti(v reflect.Value) bool {
	t := v.Type()
	if t.Kind() != reflect.Ptr {
		return false
	}
	valElem := t.Elem()
	if valElem.Kind() != reflect.Slice {
		return false
	}
	sliceContent := valElem.Elem()
	if sliceContent.Kind() != reflect.Ptr {
		return false
	}
	if !sliceContent.Implements(typeOfProtoMessage) {
		return false
	}

	r.messageName = proto.MessageName(archetypeMessage(sliceContent))
	r.singleType = sliceContent
	r.multiDest = v.Elem()
	return true
}

// Resolve implements cfgclient.MultiResolver.
func (r *resolver) Resolve(it *backend.Item) error {
	return r.resolveItem(r.single, it.Content, it.FormatSpec.Formatter)
}

// PrepareMulti implements cfgclient.MultiResolver.
func (r *resolver) PrepareMulti(size int) {
	slice := reflect.MakeSlice(r.multiDest.Type(), size, size)
	r.multiDest.Set(slice)
}

// ResolveItemAt implements cfgclient.MultiResolver.
func (r *resolver) ResolveItemAt(i int, it *backend.Item) error {
	msgV := archetypeInstance(r.singleType)
	if err := r.resolveItem(msgV.Interface().(proto.Message), it.Content, it.FormatSpec.Formatter); err != nil {
		return err
	}
	r.multiDest.Index(i).Set(msgV)
	return nil
}

func (r *resolver) resolveItem(out proto.Message, content string, format string) error {
	switch format {
	case "":
		// Not formatted (text protobuf).
		if err := luciProto.UnmarshalTextML(content, out); err != nil {
			return errors.Annotate(err, "failed to unmarshal text protobuf").Err()
		}
		return nil

	case BinaryFormat:
		if err := parseBinaryContent(content, out); err != nil {
			return errors.Annotate(err, "failed to unmarshal binary protobuf").Err()
		}
		return nil

	default:
		return errors.Reason("unsupported content format: %q", format).Err()
	}
}

func (r *resolver) Format() backend.FormatSpec {
	return backend.FormatSpec{BinaryFormat, r.messageName}
}

// Formatter is a cfgclient.Formatter implementation bound to a specific
// protobuf message.
//
// It takes a text protobuf representation of that message as input and returns
// a binary protobuf representation as output.
type Formatter struct{}

// FormatItem implements cfgclient.Formatter.
func (f *Formatter) FormatItem(c, fd string) (string, error) {
	archetype := proto.MessageType(fd)
	if archetype == nil {
		return "", errors.Reason("unknown proto.Message type %q in formatter data", fd).Err()
	}
	msg := archetypeMessage(archetype)

	// Convert from config to protobuf.
	if err := luciProto.UnmarshalTextML(c, msg); err != nil {
		return "", errors.Annotate(err, "failed to unmarshal text protobuf content").Err()
	}

	// Binary format.
	bc, err := makeBinaryContent(msg)
	if err != nil {
		return "", errors.Annotate(err, "").Err()
	}
	return bc, nil
}

// t is a pointer to a proto.Message instance.
func archetypeInstance(t reflect.Type) reflect.Value {
	return reflect.New(t.Elem())
}

func archetypeMessage(t reflect.Type) proto.Message {
	return archetypeInstance(t).Interface().(proto.Message)
}

// makeBinaryContent constructs a binary content string from text protobuf
// content.
//
// The binary content is formatted by concatenating two "cmpbin" binary values
// together:
// [Message Name] | [Marshaled Message Data]
func makeBinaryContent(msg proto.Message) (string, error) {
	d, err := proto.Marshal(msg)
	if err != nil {
		return "", errors.Annotate(err, "failed to marshal message").Err()
	}

	var buf bytes.Buffer
	if _, err := cmpbin.WriteString(&buf, proto.MessageName(msg)); err != nil {
		return "", errors.Annotate(err, "failed to write message name").Err()
	}
	if _, err := cmpbin.WriteBytes(&buf, d); err != nil {
		return "", errors.Annotate(err, "failed to write binary message content").Err()
	}
	return buf.String(), nil
}

// parseBinaryContent parses a binary content string, pulling out the message
// type and marshalled message data. It then unmarshals the specified type into
// a new message based on the archetype.
//
// If the binary content's declared type doesn't match the archetype, or if the
// binary content is invalid, an error will be returned.
func parseBinaryContent(v string, msg proto.Message) error {
	r := strings.NewReader(v)
	encName, _, err := cmpbin.ReadString(r)
	if err != nil {
		return errors.Annotate(err, "failed to read message name").Err()
	}

	// Construct a message for this.
	if name := proto.MessageName(msg); name != encName {
		return errors.Reason("message name %q doesn't match encoded name %q", name, encName).Err()
	}

	// We have the right message, unmarshal.
	d, _, err := cmpbin.ReadBytes(r)
	if err != nil {
		return errors.Annotate(err, "failed to read binary message content").Err()
	}
	if err := proto.Unmarshal(d, msg); err != nil {
		return errors.Annotate(err, "failed to unmarshal message").Err()
	}
	return nil
}
