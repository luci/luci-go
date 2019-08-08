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
	"sort"

	"go.starlark.net/starlark"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// MessageType represents a proto message type and acts as its constructor: it
// is a Starlark callable that produces instances of Message.
//
// Implements starlark.HasAttrs interface. Attributes represent constructors for
// nested messages and int values of enums. Note that starlark.HasSetField is
// not implemented, making values of MessageType immutable.
//
// Given a MessageDescriptor can be instantiated through Loader as
// loader.MessageType(...).
type MessageType struct {
	*starlark.Builtin // the callable, initialize in Loader

	loader *Loader                                 // owning Loader
	desc   protoreflect.MessageDescriptor          // original descriptor
	attrs  starlark.StringDict                     // nested symbols, e.g. submessages and enums
	fields map[string]protoreflect.FieldDescriptor // message fields (including oneof alternatives)
}

// initLocked preprocesses message descriptor.
//
// Called under the loader lock.
func (t *MessageType) initLocked() {
	fields := t.desc.Fields() // note: this already includes oneof alternatives
	t.fields = make(map[string]protoreflect.FieldDescriptor, fields.Len())
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		t.fields[string(fd.Name())] = fd
	}
}

// Descriptor returns protobuf type information for this message type.
func (t *MessageType) Descriptor() protoreflect.MessageDescriptor {
	return t.desc
}

// NewMessage instantiates a new empty message of this type.
func (t *MessageType) NewMessage() *Message {
	return &Message{
		typ:    t,
		fields: starlark.StringDict{},
	}
}

// Starlark.Value interface.

// Type returns full proto message name.
func (t *MessageType) Type() string {
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
