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

package skylarkproto

import (
	"fmt"
	"reflect"

	"github.com/golang/protobuf/proto"
	"github.com/google/skylark"
)

// Message is a skylark value that implements a struct-like type structured
// like a protobuf message.
//
// Implements skylark.Value, skylark.HasAttrs and skylark.HasSetField
// interfaces.
type Message struct {
	typ    *MessageType       // type information
	fields skylark.StringDict // populated fields, keyed by proto field name
}

// NewMessage instantiates a message of the given type.
func NewMessage(typ *MessageType) *Message {
	return &Message{
		typ:    typ,
		fields: skylark.StringDict{},
	}
}

// Public API used by the hosting environment.

// MessageType returns detailed type information about the message.
func (m *Message) MessageType() *MessageType { return m.typ }

// ToProto returns a new populated proto message of an appropriate type.
//
// Returns an error if the data inside the skylark representation of the message
// has a wrong type.
func (m *Message) ToProto() (proto.Message, error) {
	ptr := reflect.New(m.typ.Type().Elem()) // ~ ptr := &ProtoMessage{}
	msg := ptr.Elem()                       // ~ msg := *ptr (a reference)

	for name, val := range m.fields {
		fd, ok := m.typ.fields[name]
		if !ok {
			panic("should not happen, SetField and Attr checks the structure already")
		}
		if err := assign(fd.value(msg), val); err != nil {
			return nil, fmt.Errorf("bad value for field %q of %q - %s", name, m.Type(), err)
		}
	}

	return ptr.Interface().(proto.Message), nil
}

// Basic skylark.Value interface.

func (m *Message) String() string {
	msg, err := m.ToProto()
	if err != nil {
		return fmt.Sprintf("<!Bad %s: %s!>", m.Type(), err)
	}
	return msg.String()
}

func (m *Message) Type() string {
	// The receiver is nil when doing type checks with skylark.UnpackArgs. It asks
	// the nil message for its type for the error message.
	if m == nil {
		return "proto.Message"
	}
	return m.typ.name
}

func (m *Message) Freeze() {} // TODO

func (m *Message) Truth() skylark.Bool { return skylark.True }

func (m *Message) Hash() (uint32, error) {
	return 0, fmt.Errorf("proto message %q is not hashable", m.Type())
}

// HasAttrs and HasSetField interfaces that make the message look like a struct.

func (m *Message) Attr(name string) (skylark.Value, error) {
	// The field was already set?
	val, ok := m.fields[name]
	if ok {
		return val, nil
	}

	// The field wasn't set, but it is defined by the proto schema? Need to
	// generate and return the default value then.
	if fd, ok := m.typ.fields[name]; ok {
		def, err := newDefaultValue(fd.typ)
		if err != nil {
			return nil, err
		}
		m.fields[name] = def
		return def, nil
	}

	return nil, fmt.Errorf("proto message %q has no field %q", m.Type(), name)
}

func (m *Message) AttrNames() []string {
	return m.typ.fieldNames
}

func (m *Message) SetField(name string, val skylark.Value) error {
	fd, ok := m.typ.fields[name]
	if !ok {
		return fmt.Errorf("proto message %q has no field %q", m.Type(), name)
	}

	// Setting a field to None removes it completely.
	if val == skylark.None {
		delete(m.fields, name)
		return nil
	}

	// Do a light type check. It doesn't "recurse" into lists or tuples. So it is
	// still possible to assign e.g. a list of strings to a "repeated int64"
	// field. This will be discovered later in ToProto when trying to construct
	// a proto message from Skylark values.
	if err := checkAssignable(fd.typ, val); err != nil {
		return fmt.Errorf("can't assign value of type %q to field %q in proto %q - %s", val.Type(), name, m.Type(), err)
	}
	m.fields[name] = val

	return nil
}
