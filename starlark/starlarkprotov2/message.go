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

	"go.chromium.org/luci/starlark/typed"
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
func (m *Message) FromDict(d starlark.IterableMapping) error {
	iter := d.Iterate()
	defer iter.Done()

	var k starlark.Value
	for iter.Next(&k) {
		key, ok := k.(starlark.String)
		if !ok {
			return fmt.Errorf("got %q dict key, want \"string\"", k.Type())
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

	var def starlark.Value

	// If this is a repeated field or a dict, instantiate corresponding empty
	// typed.List or typed.Dict instance.
	loader := m.typ.loader
	switch {
	case fd.IsList():
		def, _ = typed.NewList(converter(loader, fd), nil)
	case fd.IsMap():
		def = typed.NewDict(
			converter(loader, fd.MapKey()),
			converter(loader, fd.MapValue()),
			0,
		)
	default:
		// If this is a singular field, auto-initialize it to its default value.
		// In particular this is important when chaining through fields `a.b.c.d`.
		// We want intermediates to be silently auto-initialized.
		if def, err = toStarlark(m.typ.loader, fd, fd.Default()); err != nil {
			return nil, err
		}
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

	origVal := val
	loader := m.typ.loader
	switch {
	case fd.IsList():
		// Reuse typed lists of correct type by reference, copy the rest.
		val, err = asTypedList(val, converter(loader, fd))
	case fd.IsMap():
		// Reuse typed dicts of correct type by reference, copy the rest.
		val, err = asTypedDict(val, converter(loader, fd.MapKey()), converter(loader, fd.MapValue()))
	case fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind:
		// Reuse messages of correct type by reference, convert dicts to messages.
		val, err = loader.MessageType(fd.Message()).Convert(val)
	default:
		// Check types compatibility, int ranges, etc.
		err = canAssign(fd, val)
	}

	if err != nil {
		return fmt.Errorf("can't assign value of type %q to field %q in proto %q - %s", origVal.Type(), name, m.Type(), err)
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

// asTypedList returns x as is if it is already list<c>, otherwise allocates
// new list<c>, and copies it from 'x'.
func asTypedList(x starlark.Value, c typed.Converter) (*typed.List, error) {
	if tp, ok := x.(*typed.List); ok && tp.Converter() == c {
		return tp, nil // the exact same type, can reuse the reference
	}

	it := starlark.Iterate(x)
	if it == nil {
		return nil, fmt.Errorf("got %q, want an iterable", x.Type())
	}
	defer it.Done()

	var vals []starlark.Value
	if l := starlark.Len(x); l > 0 {
		vals = make([]starlark.Value, 0, l)
	}
	var itm starlark.Value
	for it.Next(&itm) {
		vals = append(vals, itm)
	}

	return typed.NewList(c, vals)
}

// asTypedDict returns x as is if it is already dict<k,v>, otherwise allocates
// new dict<k,v>, and copies it from 'x'.
func asTypedDict(x starlark.Value, k, v typed.Converter) (*typed.Dict, error) {
	if tp, ok := x.(*typed.Dict); ok && tp.KeyConverter() == k && tp.ValueConverter() == v {
		return tp, nil // the exact same type, can reuse the reference
	}

	m, ok := x.(starlark.IterableMapping)
	if !ok {
		return nil, fmt.Errorf("got %q, want an iterable mapping", x.Type())
	}
	items := m.Items()

	d := typed.NewDict(k, v, len(items))
	for _, kv := range items {
		if err := d.SetKey(kv[0], kv[1]); err != nil {
			return nil, err
		}
	}
	return d, nil
}
