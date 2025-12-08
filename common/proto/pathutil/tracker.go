// Copyright 2025 The LUCI Authors.
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

// Package pathutil implements a helper for assembling a protopath.Path, and
// for walking a proto message using such a path.
//
// This is useful for writing validators/normalizers which need to keep track
// of the current path inside of a message while validating (by generating
// error messages), or to identify partially initialized areas of the message
// during normalization and quickly get back to them without having to walk
// the entire message more than once.
//
// # A Note on `literalField`
//
// This package is designed to be used when the tracking code is tightly
// coupled to the structure of the proto message, and will panic on invalid
// field accesses.
//
// To help avoid situations where misuse of variables could lead to runtime
// panics, this package uses an unexported type `literalField` to require the
// caller to either directly use a literal string, or to use a `const` string.
//
// For example:
//
//	t := myTrackerFactory.New(0)
//
//	t.Field("some_field", ...)  // OK
//
//	const anotherField = "another_field"
//	t.Field(anotherField, ...)  // OK
//
//	dynamicFieldName := funcThatComputesFieldName()
//	// t.Field(dynamicFieldName, ...)  // Does not compile
//
// If you need to pass context down the stack, use one of [Tracker.Field],
// [Tracker.ListIndex] or [Tracker.MapIndex].
//
// For example:
//
//	// We have 2 fields "a" and "b" both of type MyMessage, which need to be
//	// validated.
//	t.Field("a", func() {
//	  validateMyMessage(t, parent.GetA())
//	})
//	t.Field("b", func() {
//	  validateMyMessage(t, parent.GetB())
//	})
//
// This is as opposed to trying to do `validateMyMessage(t, parent, "a")` or
// something like this.
package pathutil

import (
	"fmt"
	"slices"

	"google.golang.org/protobuf/reflect/protopath"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Tracker is a stateful object which keeps track of a `protopath.Path` while
// traversing a proto message.
//
// This internally keeps a stack of the current location inside the wider
// proto message, and allows direct emission of errors which will be
// contextualized with the current path.
type Tracker struct {
	cachedSteps map[protoreflect.MessageDescriptor]messageFieldSteps
	maxDepth    int

	path  protopath.Path
	stack []messageFieldSteps

	errs Errors
}

type literalField string

func (t *Tracker) pushField(field literalField, extra ...protopath.Step) (pop func()) {
	step, ok := t.stack[len(t.stack)-1][field]
	if !ok {
		panic(fmt.Errorf(
			"validate: field %q in message %q does not exist",
			field, t.stack[len(t.stack)-1]))
	}
	fd := step.FieldDescriptor()
	if fd.IsMap() {
		fd = fd.MapValue()
	}

	msg := fd.Message()
	if msg != nil && !t.canDeepen() {
		return func() {}
	}

	pathIdx := len(t.path)
	stackIdx := len(t.stack)

	t.path = slices.Grow(t.path, 1+len(extra))
	t.path = append(t.path, step)
	t.path = append(t.path, extra...)

	t.stack = append(t.stack, t.cachedSteps[msg])
	return func() {
		t.path = t.path[:pathIdx]
		t.stack = t.stack[:stackIdx]
	}
}

// Field updates the Tracker's path to add `field` for the duration of `cb`.
//
// Example:
//
//	// current path is `(MyMessage).a`
//	t.Field("deeper", func() {
//	  // current path is `(MyMessage).a.deeper`
//	})
//	// Path is `(MyMessage).a` again
func (t *Tracker) Field(field literalField, cb func()) {
	defer t.pushField(field)()
	cb()
}

// ListIndex updates the Tracker's path to add `field[idx]` for the duration of
// `cb`.
//
// See [TrackList] for a friendly interface for this.
//
// Example:
//
//	// current path is `(MyMessage).a`
//	for idx, value := range msg.GetDeepList() {
//	  t.ListIndex("deep_list", 10, func() {
//	    // current path is `(MyMessage).a.deep_list[idx]`
//	  })
//	}
//	// Path is `(MyMessage).a` again
func (t *Tracker) ListIndex(field literalField, idx int, cb func()) {
	defer t.pushField(field, protopath.ListIndex(idx))()
	cb()
}

// MapIndex updates the Tracker's path to add `field[key]` for the duration of
// `cb`.
//
// See [TrackMap] for a friendly interface for this.
//
// Example:
//
//	// current path is `(MyMessage).a`
//	for key, value := range msg.GetDeepMap() {
//	  t.MapIndex("deep_map", key, func() {
//	    // current path is `(MyMessage).a.deep_map[key]`
//	  })
//	}
//	// Path is `(MyMessage).a` again
//
// `key` must be a valid map key type (which it will be, if it came from the
// proto message) or this will panic.
func (t *Tracker) MapIndex(field literalField, key any, cb func()) {
	defer t.pushField(field, protopath.MapIndex(protoreflect.MapKey(protoreflect.ValueOf(key))))()
	cb()
}

// CurDepth returns the number of fields traversed from the root message.
func (t *Tracker) CurDepth() int {
	return len(t.stack) - 1
}

func (t *Tracker) canDeepen() bool {
	if t.maxDepth > 0 && t.CurDepth() == t.maxDepth {
		t.Err("reached maximum depth %d", t.maxDepth)
		return false
	}
	return true
}

// Err records a new [Error] to this Tracker, capturing the current path,
// and rendering the error with `fmt.Errorf`.
func (t *Tracker) Err(format string, args ...any) {
	t.errs = append(t.errs, &Error{
		Path:    slices.Clone(t.path),
		Wrapped: fmt.Errorf(format, args...),
	})
}

// FieldErr records a new [Error] to this Tracker, capturing the current path
// plus a new Field.
//
// This is equivalent to, but more efficient/less verbose than:
//
//	t.Field(field, func() {
//	  t.Err(format, args)
//	})
func (t *Tracker) FieldErr(field literalField, format string, args ...any) {
	defer t.pushField(field)()
	t.Err(format, args...)
}

// ListIndexErr records a new [Error] to this Tracker, capturing the current
// path plus a new Field and list index.
//
// This is equivalent to, but more efficient/less verbose than:
//
//	t.ListIndex(field, idx, func() {
//	  t.Err(format, args)
//	})
func (t *Tracker) ListIndexErr(field literalField, idx int, format string, args ...any) {
	defer t.pushField(field, protopath.ListIndex(idx))()
	t.Err(format, args...)
}

// MapIndexErr records a new [Error] to this Tracker, capturing the current
// path plus a new Field and map index.
//
// This is equivalent to, but more efficient/less verbose than:
//
//	t.MapIndex(field, idx, func() {
//	  t.Err(format, args)
//	})
func (t *Tracker) MapIndexErr(field literalField, key any, format string, args ...any) {
	defer t.pushField(field, protopath.MapIndex(protoreflect.MapKey(protoreflect.ValueOf(key))))()
	t.Err(format, args...)
}

// CurrentPath returns a copy of the current path.
func (t *Tracker) CurrentPath() protopath.Path {
	return slices.Clone(t.path)
}

// Errors returns all accumulated errors on this tracker.
//
// This only contains errors emitted by [Tracker.Err], [Tracker.ListIndexErr],
// or [Tracker.MapIndexErr].
//
// All errors in the returned slice are [*Error].
func (t *Tracker) Errors() Errors {
	return t.errs
}
