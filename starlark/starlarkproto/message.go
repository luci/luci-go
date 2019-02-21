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
	"reflect"

	"github.com/golang/protobuf/proto"

	"go.starlark.net/starlark"
)

// Message is a starlark value that implements a struct-like type structured
// like a protobuf message.
//
// Implements starlark.Value, starlark.HasAttrs and starlark.HasSetField
// interfaces.
type Message struct {
	typ    *MessageType        // type information
	fields starlark.StringDict // populated fields, keyed by proto field name
}

// NewMessage instantiates a new empty message of the given type.
func NewMessage(typ *MessageType) *Message {
	return &Message{
		typ:    typ,
		fields: starlark.StringDict{},
	}
}

// Public API used by the hosting environment.

// MessageType returns detailed type information about the message.
func (m *Message) MessageType() *MessageType { return m.typ }

// ToProto returns a new populated proto message of an appropriate type.
//
// Returns an error if the data inside the starlark representation of
// the message has a wrong type.
func (m *Message) ToProto() (proto.Message, error) {
	ptr := m.typ.NewProtoMessage() // ~ ptr := &ProtoMessage{}
	msg := ptr.Elem()              // ~ msg := *ptr (a reference)

	for name, val := range m.fields {
		fd, ok := m.typ.fields[name]
		if !ok {
			panic("should not happen, SetField and Attr checks the structure already")
		}
		if err := assign(fd.onProtoReflection(msg, reflectToProto), val); err != nil {
			return nil, fmt.Errorf("bad value for field %q of %q - %s", name, m.Type(), err)
		}
	}

	return ptr.Interface().(proto.Message), nil
}

// FromProto populates fields of this message based on values in proto.Message.
//
// Returns an error on type mismatch.
func (m *Message) FromProto(p proto.Message) error {
	ptr := reflect.ValueOf(p)
	if ptr.Type() != m.typ.Type() {
		return fmt.Errorf("bad message type: got %s, expect %s", ptr.Type(), m.typ.Type())
	}

	msg := ptr.Elem()
	for name, fd := range m.typ.fields {
		// Get the field's value from the proto message as reflect.Value. For unused
		// oneof alternatives this is an invalid zero value, we skip them right
		// away. For other fields it is reflect.Value (of fd.typ type) that MAY be
		// nil inside (for unset fields). toStarlarkValue converts such values to
		// starlark.None.
		val := fd.onProtoReflection(msg, reflectFromProto)
		if !val.IsValid() {
			continue
		}
		// Convert the Go value to the corresponding Starlark value and assign it to
		// the field in 'm'.
		sv, err := toStarlarkValue(val)
		if err != nil {
			return fmt.Errorf("cannot recognize value of field %s: %s", name, err)
		}
		if err := m.SetField(name, sv); err != nil {
			return err
		}
	}

	return nil
}

// Basic starlark.Value interface.

// String implements starlark.Value.
func (m *Message) String() string {
	msg, err := m.ToProto()
	if err != nil {
		return fmt.Sprintf("<!Bad %s: %s!>", m.Type(), err)
	}
	return msg.String()
}

// Type implements starlark.Value.
func (m *Message) Type() string {
	// The receiver is nil when doing type checks with starlark.UnpackArgs. It asks
	// the nil message for its type for the error message.
	if m == nil {
		return "proto.Message"
	}
	return m.typ.name
}

// Freeze implements starlark.Value.
func (m *Message) Freeze() {} // TODO

// Truth implements starlark.Value.
func (m *Message) Truth() starlark.Bool { return starlark.True }

// Hash implements starlark.Value.
func (m *Message) Hash() (uint32, error) {
	return 0, fmt.Errorf("proto message %q is not hashable", m.Type())
}

// HasAttrs and HasSetField interfaces that make the message look like a struct.

// Attr implements starlark.HasAttrs.
func (m *Message) Attr(name string) (starlark.Value, error) {
	// The field was already set?
	val, ok := m.fields[name]
	if ok {
		return val, nil
	}

	// The field wasn't set, but it is defined by the proto schema? Need to
	// generate and return the default value then, except for oneof alternatives
	// that do not have defaults. This is needed to make sure callers are
	// explicitly picking a oneof alternative by assigning a value to it, rather
	// than have it picked implicitly be reading an attribute (which is weird).
	if fd, ok := m.typ.fields[name]; ok {
		if !fd.defaultable {
			return starlark.None, nil
		}
		def, err := newDefaultValue(fd.typ)
		if err != nil {
			return nil, err
		}
		m.fields[name] = def
		return def, nil
	}

	return nil, fmt.Errorf("proto message %q has no field %q", m.Type(), name)
}

// AttrNames implements starlark.HasAttrs.
func (m *Message) AttrNames() []string {
	return m.typ.fieldNames
}

// SetField implements starlark.HasSetField.
func (m *Message) SetField(name string, val starlark.Value) error {
	fd, ok := m.typ.fields[name]
	if !ok {
		return fmt.Errorf("proto message %q has no field %q", m.Type(), name)
	}

	// Setting a field to None removes it completely.
	if val == starlark.None {
		delete(m.fields, name)
		return nil
	}

	// Do a light type check. It doesn't "recurse" into lists or tuples. So it is
	// still possible to assign e.g. a list of strings to a "repeated int64"
	// field. This will be discovered later in ToProto when trying to construct
	// a proto message from Starlark values.
	if err := checkAssignable(fd.typ, val); err != nil {
		return fmt.Errorf("can't assign value of type %q to field %q in proto %q - %s", val.Type(), name, m.Type(), err)
	}
	m.fields[name] = val

	// onChanged hooks is used by oneof's to clear alternatives that weren't
	// picked.
	if fd.onChanged != nil {
		fd.onChanged(m.fields)
	}

	return nil
}
