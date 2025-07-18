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
	"strings"

	"github.com/protocolbuffers/txtpbfmt/parser"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Message is a Starlark value that implements a struct-like type structured
// like a protobuf message.
//
// Implements starlark.Value, starlark.HasAttrs, starlark.HasSetField and
// starlark.Comparable interfaces.
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

var (
	_ starlark.Value       = (*Message)(nil)
	_ starlark.HasAttrs    = (*Message)(nil)
	_ starlark.HasSetField = (*Message)(nil)
	_ starlark.Comparable  = (*Message)(nil)
)

// Public API used by the hosting environment.

// MessageType returns type information about this message.
func (m *Message) MessageType() *MessageType {
	return m.typ
}

// IsFrozen returns true if this message was frozen already.
func (m *Message) IsFrozen() bool {
	return m.frozen
}

// ToProto returns a new populated proto message of an appropriate type.
func (m *Message) ToProto() proto.Message {
	msg := dynamicpb.NewMessage(m.typ.desc)
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
//	for k in d:
//	  setattr(msg, k, d[k])
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

// HasProtoField returns true if the message has the given field initialized.
func (m *Message) HasProtoField(name string) bool {
	// If the field was set through Starlark already, it exists. This also covers
	// "selected" oneof alternatives.
	if _, ok := m.fields[name]; ok {
		return true
	}

	// Check we have this field defined in the schema at all.
	fd, err := m.fieldDesc(name)
	if err != nil {
		return false
	}

	// If this is a part of some oneof set, the field is assumed set only if it
	// was explicitly initialized in Starlark (already checked above). So if we
	// are here, then this particular oneof alternative wasn't used.
	if fd.ContainingOneof() != nil {
		return false
	}

	// Repeated and map fields are assumed to be always preset. They just may be
	// empty.
	if fd.IsList() || fd.IsMap() {
		return true
	}

	// Singular message-typed fields are set only if they were explicitly
	// initialized (and if we are here, they were not).
	if kind := fd.Kind(); kind == protoreflect.MessageKind || kind == protoreflect.GroupKind {
		return false
	}

	// Singular fields of primitive types are always set, since there's no way to
	// distinguish fields initialized with a default value from unset fields.
	return true
}

// Basic starlark.Value interface.

// String returns compact text serialization of this message.
func (m *Message) String() string {
	raw := m.ToProto().(interface{ String() string }).String()
	formatted, err := parser.FormatWithConfig([]byte(raw), parser.Config{
		SkipAllColons: true,
	})
	if err != nil {
		return fmt.Sprintf("<bad proto: %q>", err)
	}
	return strings.TrimSpace(string(formatted))
}

// Type returns full proto message name.
func (m *Message) Type() string {
	// The receiver is nil when doing type checks with starlark.UnpackArgs. It
	// asks the nil message for its type for the error message.
	if m == nil {
		return "proto.Message"
	}
	return fmt.Sprintf("proto.Message<%s>", m.typ.desc.FullName())
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
	return m.attrImpl(name, true)
}

// attrImpl is the actual implementation of Attr.
//
// 'mut' controls how attrImpl behaves if 'name' field is not set.
//
// If 'mut' is true, the field will be set to its default value and this value
// is returned. This updates 'm' as a side effect.
//
// If 'mut' is false, and the field is not message-valued, its default value
// is returned (but 'm' itself is not updated). This applies to repeated fields
// and maps as well. But if the field is message-valued, None is returned.
func (m *Message) attrImpl(name string, mut bool) (starlark.Value, error) {
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

	// Don't auto-initialize message-valued fields if 'mut' is false. This case is
	// special because 'm.msg = Msg{}' and 'm.msg = None' lead to observably
	// different outcomes and we should account for that in 'messagesEqual'.
	if !mut && !fd.IsList() && !fd.IsMap() &&
		(fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind) {
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
	if mut {
		if m.frozen {
			def.Freeze()
		}
		m.fields[name] = def
	}
	return def, nil
}

// AttrNames lists available attributes.
func (m *Message) AttrNames() []string {
	return m.typ.keys
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
		for i := range alts.Len() {
			if altfd := alts.Get(i); altfd != fd {
				delete(m.fields, string(altfd.Name()))
			}
		}
	}

	return nil
}

// Comparable interface to implement '==' and '!='.

// CompareSameType does 'm <op> y' comparison.
func (m *Message) CompareSameType(op syntax.Token, y starlark.Value, depth int) (bool, error) {
	switch op {
	case syntax.EQL:
		return messagesEqual(m, y.(*Message), depth)
	case syntax.NEQ:
		eq, err := messagesEqual(m, y.(*Message), depth)
		return !eq, err
	default:
		return false, fmt.Errorf("%q is not implemented for %s", op, m.Type())
	}
}

// messagesEqual compares two messages by value, recursively.
func messagesEqual(l, r *Message, depth int) (bool, error) {
	switch {
	case l == r:
		return true, nil // equal by identity
	case l.typ != r.typ:
		return false, nil // messages of different types are never equal
	}
	// We go through attrImpl(...) to correctly handle default values and oneof's.
	// We don't want to mutate messages though.
	for _, key := range l.typ.keys {
		lv, err := l.attrImpl(key, false)
		if err != nil {
			return false, err
		}
		rv, err := r.attrImpl(key, false)
		if err != nil {
			return false, err
		}
		if eq, err := starlark.EqualDepth(lv, rv, depth-1); !eq || err != nil {
			return false, err
		}
	}
	return true, nil
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
		return nil, fmt.Errorf("internal error: descriptor of %s is not available, can't use this type", m.Type())
	default:
		return nil, fmt.Errorf("%s has no field %q", m.Type(), name)
	}
}

// checkMutable returns an error if the message is frozen.
func (m *Message) checkMutable() error {
	if m.frozen {
		return fmt.Errorf("cannot modify frozen %s", m.Type())
	}
	return nil
}
