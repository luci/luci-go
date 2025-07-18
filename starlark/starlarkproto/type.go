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

package starlarkproto

import (
	"fmt"
	"sort"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/starlark/typed"
)

// MessageType represents a proto message type and acts as its constructor: it
// is a Starlark callable that produces instances of Message.
//
// Implements starlark.HasAttrs interface. Attributes represent constructors for
// nested messages and int values of enums. Note that starlark.HasSetField is
// not implemented, making values of MessageType immutable.
//
// Stringifying an instance of MessageType (e.g. with `str(...)` in Starlark)
// returns its full proto type name.
//
// Given a MessageDescriptor, MessageType can be instantiated through Loader as
// loader.MessageType(...).
type MessageType struct {
	*starlark.Builtin // the callable, initialize in Loader

	loader *Loader                                 // owning Loader
	desc   protoreflect.MessageDescriptor          // original descriptor
	attrs  starlark.StringDict                     // nested symbols, e.g. submessages and enums
	fields map[string]protoreflect.FieldDescriptor // message fields (including oneof alternatives)
	keys   []string                                // sorted keys of 'fields' map
}

// initLocked preprocesses message descriptor.
//
// Called under the loader lock.
func (t *MessageType) initLocked() {
	fields := t.desc.Fields() // note: this already includes oneof alternatives
	t.fields = make(map[string]protoreflect.FieldDescriptor, fields.Len())
	t.keys = make([]string, fields.Len())
	for i := range fields.Len() {
		fd := fields.Get(i)
		key := string(fd.Name())
		t.fields[key] = fd
		t.keys[i] = key
	}
}

// Descriptor returns protobuf type information for this message type.
func (t *MessageType) Descriptor() protoreflect.MessageDescriptor {
	return t.desc
}

// Loader returns the loader this message type was loaded from.
func (t *MessageType) Loader() *Loader {
	return t.loader
}

// Message instantiates a new empty message of this type.
func (t *MessageType) Message() *Message {
	return &Message{
		typ:    t,
		fields: starlark.StringDict{},
	}
}

// MessageFromProto instantiates a new message of this type and populates it
// based on values in the given proto.Message that should have a matching type.
//
// Here "matching type" means p.ProtoReflect().Descriptor() *is* t.Descriptor().
// Panics otherwise.
func (t *MessageType) MessageFromProto(p proto.Message) *Message {
	refl := p.ProtoReflect()
	if got := refl.Descriptor(); got != t.desc {
		panic(fmt.Errorf("bad message type: got %s, want %s", got.FullName(), t.desc.FullName()))
	}
	m := t.Message()
	refl.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		err := m.SetField(string(fd.Name()), toStarlark(t.loader, fd, v))
		if err != nil {
			panic(fmt.Errorf("internal error: field %q: %s", fd.Name(), err))
		}
		return true
	})
	return m
}

// Converter returns an object that can convert Starlark dicts and Nones to
// values of this message type.
//
// Can be used by typed.List and typed.Dict.
func (t *MessageType) Converter() typed.Converter {
	// Note: it is important that t.Converter() == t.Converter() at all times.
	// Converters are compared by identity when checking type matches.
	return messageConverter{t}
}

// Starlark.Value interface.

// Type returns "proto.MessageType", it's the type of the message type itself.
//
// It is sort of like a meta-type, i.e. like "type" type in Python.
func (t *MessageType) Type() string {
	return "proto.MessageType"
}

// String returns the full proto type name.
func (t *MessageType) String() string {
	return string(t.desc.FullName())
}

// Attr returns either a nested message or an enum value.
func (t *MessageType) Attr(name string) (starlark.Value, error) {
	return t.attrs[name], nil // (nil, nil) means "not found"
}

// AttrNames return names of all nested messages and enum values.
func (t *MessageType) AttrNames() []string {
	keys := make([]string, 0, len(t.attrs))
	for k := range t.attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// typed.Converter implementation.

type messageConverter struct {
	typ *MessageType
}

// Convert returns 'x' as is if it already has type 'c.typ', otherwise it
// initializes a new message of type 'c.typ' from 'x'.
//
// 'x' can be either None (in which case an empty message is initialized) or an
// iterable mapping (e.g. a dict).
func (c messageConverter) Convert(x starlark.Value) (starlark.Value, error) {
	if msg, ok := x.(*Message); ok {
		if msg.typ == c.typ {
			return msg, nil
		}
		return nil, fmt.Errorf("got %s, want %s", msg.Type(), c.Type())
	}

	if x == starlark.None {
		return c.typ.Message(), nil
	}

	if d, ok := x.(starlark.IterableMapping); ok {
		m := c.typ.Message()
		if err := m.FromDict(d); err != nil {
			return nil, err
		}
		return m, nil
	}

	return nil, fmt.Errorf("got %s, want %s", x.Type(), c.Type())
}

// Type returns name of the type 'c' converts to.
func (c messageConverter) Type() string {
	return fmt.Sprintf("proto.Message<%s>", c.typ.desc.FullName())
}
