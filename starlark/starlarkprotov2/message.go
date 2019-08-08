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
// Can be instantiated through Loader as loader.MessageType(...).NewMessage().
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
//
// Returns an error if some fields of this Starlark value can't be converted
// into protos. For example, a field declared as `repeated int64` can actually
// contains a list of strings on Starlark side, which will cause ToProto to
// return an error.
func (m *Message) ToProto() (proto.Message, error) {
	msg := dynamicpb.New(m.typ.desc)
	for k, v := range m.fields {
		if err := assign(msg, m.typ.fields[k], v); err != nil {
			return nil, fmt.Errorf("bad value for field %q of %q - %s", k, m.Type(), err)
		}
	}
	return msg, nil
}

// FromProto populates fields of this message based on values in proto.Message.
//
// Returns an error if type of `p` doesn't match `m`.
func (m *Message) FromProto(p proto.Message) error {
	refl := p.ProtoReflect()

	if got, want := refl.Descriptor(), m.typ.desc; got != want {
		return fmt.Errorf("bad message type: got %q, want %q", got.FullName(), want.FullName())
	}

	var err error
	refl.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		var sv starlark.Value
		if sv, err = toStarlark(m.typ.loader, fd, v); err != nil {
			err = fmt.Errorf("in %q: %s", fd.Name(), err)
			return false
		}
		err = m.SetField(string(fd.Name()), sv)
		return err == nil
	})

	return err
}

// FromDict populates fields of this message based on values in a starlark.Dict.
//
// Doesn't reset the message. Basically does this:
//
//   for k in d:
//     setattr(msg, k, d[k])
//
// Returns an error on type mismatch.
func (m *Message) FromDict(d *starlark.Dict) error {
	iter := d.Iterate()
	defer iter.Done()

	var k starlark.Value
	for iter.Next(&k) {
		key, ok := k.(starlark.String)
		if !ok {
			return fmt.Errorf("got %q dict key, expecting \"string\"", k.Type())
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
	msg, err := m.ToProto()
	if err != nil {
		return fmt.Sprintf("<!Bad %s: %s!>", m.Type(), err)
	}
	return fmt.Sprintf("%s", msg) // this is a compact text serialization
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
	return 0, fmt.Errorf("proto messages (and %q in particular) are not hashable", m.Type())
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
	def, err := toStarlark(m.typ.loader, fd, fd.Default())
	if err != nil {
		return nil, err
	}

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

	// When assigning to a message-valued field (singular or repeated), recognize
	// dicts and Nones and use them to instantiate values (perhaps empty) of the
	// corresponding proto type. This allows to construct deeply nested protobuf
	// messages just by using lists, dicts and primitive values. Python does this
	// too.
	if fd.Kind() == protoreflect.MessageKind {
		var err error
		if val, err = maybeMakeMessages(m.typ.loader, fd, val); err != nil {
			return fmt.Errorf("when constructing %q in proto %q - %s", name, m.Type(), err)
		}
	}

	// Do a light O(1) type check. It doesn't "recurse" into lists or tuples. So
	// it is still possible to assign e.g. a list of strings to a "repeated int64"
	// field. This will be discovered later in ToProto when trying to construct
	// a proto message from Starlark values.
	if err := canAssign(fd, val); err != nil {
		return fmt.Errorf("can't assign value of type %q to field %q in proto %q - %s", val.Type(), name, m.Type(), err)
	}
	if err := m.checkMutable(); err != nil {
		return err
	}
	m.fields[name] = val

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
		return nil, fmt.Errorf("internal error: descriptor of proto message %q is not available, can't use this type", m.Type())
	default:
		return nil, fmt.Errorf("proto message %q has no field %q", m.Type(), name)
	}
}

// checkMutable returns an error if the message is frozen.
func (m *Message) checkMutable() error {
	if m.frozen {
		return fmt.Errorf("cannot modify frozen proto message %q", m.Type())
	}
	return nil
}

// maybeMakeMessages recognizes when a dict is assigned to a message field or
// when a list or tuple of dicts or Nones is assigned to a repeated message
// field.
//
// It converts dicts or Nones to *Message of corresponding type and returns them
// as Starlark values to use in place of the passed value.
//
// Returns an error if some dict can't be converted to *Message of the requested
// type (e.g. it has wrong schema).
func maybeMakeMessages(l *Loader, fd protoreflect.FieldDescriptor, val starlark.Value) (starlark.Value, error) {
	// Assigning a dict to singular field.
	if dict, ok := val.(*starlark.Dict); ok && fd.Cardinality() != protoreflect.Repeated {
		msg := l.MessageType(fd.Message()).NewMessage()
		return msg, msg.FromDict(dict)
	}

	// Assigning a primitive sequence (not a map!) to a repeated field.
	seq, _ := val.(starlark.Sequence)
	mapping, _ := val.(starlark.Mapping)
	if seq != nil && mapping == nil && fd.Cardinality() == protoreflect.Repeated {
		// Return 'seq' as is (not a copy!) if it doesn't need to be transformed.
		// This is important in the following case:
		//   l = [m1]
		//   msg.repeated = l
		//   l.append(m2)  # msg.repeated is also mutated
		if !shouldMakeMessages(seq) {
			return val, nil
		}

		iter := seq.Iterate()
		defer iter.Done()

		// Make a copy of 'seq' by replacing dicts and Nones with messages.
		out := make([]starlark.Value, 0, seq.Len())
		typ := l.MessageType(fd.Message())

		var v starlark.Value
		for iter.Next(&v) {
			switch val := v.(type) {
			case starlark.NoneType:
				out = append(out, typ.NewMessage())
			case *starlark.Dict:
				msg := typ.NewMessage()
				if err := msg.FromDict(val); err != nil {
					return nil, err
				}
				out = append(out, msg)
			default:
				out = append(out, v)
			}
		}

		return starlark.NewList(out), nil
	}

	// Unrecognized combination, let SetField deal with it.
	return val, nil
}

// shouldMakeMessages returns true if seq has at least one dict or None that
// should be converted to a proto message.
func shouldMakeMessages(seq starlark.Sequence) bool {
	iter := seq.Iterate()
	defer iter.Done()
	var v starlark.Value
	for iter.Next(&v) {
		if v == starlark.None {
			return true
		}
		if _, ok := v.(*starlark.Dict); ok {
			return true
		}
	}
	return false
}
