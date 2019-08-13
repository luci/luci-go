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
	"fmt"
	"sort"

	"go.starlark.net/starlark"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Message is a Starlark value that implements a struct-like type structured
// like a protobuf message.
//
// Implements starlark.Value, starlark.HasAttrs and starlark.HasSetField
// interfaces.
//
// Can be instantiated through Loader as loader.MessageType(...).Message() or
// loader.MessageType(...).MessageFromProto(p).
//
// TODO(vadimsh): Currently not safe for a cross-goroutine use without external
// locking, even when frozen, due to lazy initialization of default fields on
// first access.
type Message struct {
	typ    *MessageType        // type information
	fields starlark.StringDict // populated fields, keyed by proto field name
	frozen bool                // true after Freeze()
}

// Public API used by the hosting environment.

// MessageType returns type information about this message.
func (m *Message) MessageType() *MessageType {
	return m.typ
}

// ToProto returns a new populated proto message of an appropriate type.
func (m *Message) ToProto() proto.Message {
	msg := dynamicpb.New(m.typ.desc)
	for k, v := range m.fields {
		assign(msg, m.typ.fields[k], v)
	}
	return msg
}

// FromDict populates fields of this message based on values in an iterable
// mapping (usually a starlark.Dict).
//
// Doesn't reset the message. Basically does this:
//
//   for k in d:
//     setattr(msg, k, d[k])
//
// Returns an error on type mismatch.
func (m *Message) FromDict(d starlark.IterableMapping) error {
	iter := d.Iterate()
	defer iter.Done()

	var k starlark.Value
	for iter.Next(&k) {
		key, ok := k.(starlark.String)
		if !ok {
			return fmt.Errorf("got %s dict key, want string", k.Type())
		}
		v, _, _ := d.Get(k)
		if err := m.SetField(key.GoString(), v); err != nil {
			return err
		}
	}

	return nil
}

// Basic starlark.Value interface.

// String returns compact text serialization of this message.
func (m *Message) String() string {
	return fmt.Sprintf("%s", m.ToProto())
}

// Type returns full proto message name.
func (m *Message) Type() string {
	// The receiver is nil when doing type checks with starlark.UnpackArgs. It
	// asks the nil message for its type for the error message.
	if m == nil {
		return "proto.Message"
	}
	return m.typ.Type()
}

// Freeze makes this message immutable.
func (m *Message) Freeze() {
	if !m.frozen {
		m.fields.Freeze()
		m.frozen = true
	}
}

// Truth always returns True.
func (m *Message) Truth() starlark.Bool { return starlark.True }

// Hash returns an error, indicating proto messages are not hashable.
func (m *Message) Hash() (uint32, error) {
	return 0, fmt.Errorf("proto messages (and %s in particular) are not hashable", m.Type())
}

// HasAttrs and HasSetField interfaces that make the message look like a struct.

// Attr is called when a field is read from Starlark code.
func (m *Message) Attr(name string) (starlark.Value, error) {
	// If the field was set through Starlark already, return its value right away.
	val, ok := m.fields[name]
	if ok {
		return val, nil
	}

	// Check we have this field at all.
	fd, err := m.fieldDesc(name)
	if err != nil {
		return nil, err
	}

	// If this is one alternative of some oneof field, do NOT instantiate it.
	// This is needed to make sure callers are explicitly picking a oneof
	// alternative by assigning a value to it, rather than have it be picked
	// implicitly be reading an attribute (which is weird).
	if fd.ContainingOneof() != nil {
		return starlark.None, nil
	}

	// If this is not a oneof field, auto-initialize it to its default value. In
	// particular this is important when chaining through fields `a.b.c.d`. We
	// want intermediates to be silently auto-initialized.
	//
	// Note that lazy initialization of fields is an implementation detail. This
	// is significant when considering frozen messages. From the caller's point of
	// view, all fields had had their default values even before the object was
	// frozen. So we lazy-initialize the field, even if the message is frozen, but
	// make sure the new field is frozen itself too.
	//
	// TODO(vadimsh): This is not thread safe and should be improved if a frozen
	// *Message is shared between goroutines. Generally frozen values are
	// assumed to be safe for cross-goroutine use, which is not the case here.
	// If this becomes important, we can force-initialize and freeze all default
	// fields in Freeze(), but this is generally expensive.
	def := toStarlark(m.typ.loader, fd, fd.Default())
	if m.frozen {
		def.Freeze()
	}
	m.fields[name] = def
	return def, nil
}

// AttrNames lists available attributes.
func (m *Message) AttrNames() []string {
	out := make([]string, 0, len(m.typ.fields))
	for k := range m.typ.fields {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// SetField is called when a field is assigned to from Starlark code.
func (m *Message) SetField(name string, val starlark.Value) error {
	// Check we have this field defined in the message.
	fd, err := m.fieldDesc(name)
	if err != nil {
		return err
	}

	// Setting a field to None removes it completely.
	if val == starlark.None {
		if err := m.checkMutable(); err != nil {
			return err
		}
		delete(m.fields, name)
		return nil
	}

	// Check the type, do implicit type casts.
	rhs, err := prepareRHS(m.typ.loader, fd, val)
	if err != nil {
		return fmt.Errorf("can't assign %s to field %q in %s: %s", val.Type(), name, m.Type(), err)
	}
	if err := m.checkMutable(); err != nil {
		return err
	}
	m.fields[name] = rhs

	// When assigning to a oneof alternative, clear its all other alternatives.
	if oneof := fd.ContainingOneof(); oneof != nil {
		alts := oneof.Fields()
		for i := 0; i < alts.Len(); i++ {
			if altfd := alts.Get(i); altfd != fd {
				delete(m.fields, string(altfd.Name()))
			}
		}
	}

	return nil
}

// fieldDesc returns FieldDescriptor of the corresponding field or an error
// message if there's no such field defined in the proto schema.
func (m *Message) fieldDesc(name string) (protoreflect.FieldDescriptor, error) {
	switch fd := m.typ.fields[name]; {
	case fd != nil:
		return fd, nil
	case m.typ.desc.IsPlaceholder():
		// This happens if 'm' lacks type information because its descriptor wasn't
		// in any of the sets passed to loader.AddDescriptorSet(...). This should
		// not really be happening since AddDescriptorSet(...) checks that all
		// references are resolved. But handle this case anyway for a clearer error
		// message if some unnoticed edge case pops up.
		return nil, fmt.Errorf("internal error: descriptor of proto message %s is not available, can't use this type", m.Type())
	default:
		return nil, fmt.Errorf("proto message %s has no field %q", m.Type(), name)
	}
}

// checkMutable returns an error if the message is frozen.
func (m *Message) checkMutable() error {
	if m.frozen {
		return fmt.Errorf("cannot modify frozen proto message %s", m.Type())
	}
	return nil
}
