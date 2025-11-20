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

// Package delta contains functions and types to apply diffs to protobuf
// messages in Go programs.
//
// The intent is to be able to concisely express the construction of protobuf
// messages by decomposing mutations into functions which return Diff objects,
// which can then be [Collect]'d into an instance of the base message.
//
// The diffs contain both the change (partial message, per-field update method
// selection) and also an optional error. The error allows accumulation of
// errors until some logical point in the program, rather than having to
// interleave error handling with message construction.
package delta

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Diff contains one or more modifications for a message of type `T`.
//
// Diff objects are immutable.
//
// A manually created &Diff[T]{} or `nil` are valid, but are no-ops.
//
// See [Template.New] and [RawTemplate.NewRaw] for ways to create useful Diff objects.
type Diff[T proto.Message] struct {
	// The modifications.
	//
	// Only fields which are set will have an effect in Apply, all unset fields
	// are ignored.
	//
	// If set, `applyMode` is also set and `err` is nil.
	msg T

	// A mapping of field descriptor INDEX (not number) to ApplyMode.
	//
	// Nil if this Diff is a no-op or an error.
	//
	// If set, `msg` is also set and `err` is nil.
	applyMode []ApplyMode

	// An error to return from Apply.
	//
	// If set, `msg` and `applyMode` are nil.
	err error
}

// Apply applies all changes from the Diff to `msg`, or return the Diff's
// contained error.
func (d *Diff[T]) Apply(msg T) error {
	if d == nil {
		return nil
	}
	if d.err != nil {
		return d.err
	}

	msgR := msg.ProtoReflect()
	d.msg.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch mode := d.applyMode[fd.Index()]; mode {
		case ModeSet:
			msgR.Set(fd, v)

		case ModeAppend:
			lst := msgR.Mutable(fd).List()
			newElements := v.List()
			for i := 0; i < newElements.Len(); i++ {
				lst.Append(newElements.Get(i))
			}

		case ModeMaxEnum:
			msgR.Set(fd, protoreflect.ValueOfEnum(max(v.Enum(), msgR.Get(fd).Enum())))

		case ModeMerge:
			proto.Merge(msg, d.msg)

		case ModeUpdate:
			mp := msgR.Mutable(fd).Map()
			v.Map().Range(func(mk protoreflect.MapKey, v protoreflect.Value) bool {
				mp.Set(mk, v)
				return true
			})

		default:
			panic(fmt.Errorf("impossible: unknown mode %q", mode))
		}
		return true
	})
	return nil
}

func sameTemplate(a, b []ApplyMode) bool {
	// nil applyMode slice means either a no-op Diff or an error-only Diff.
	if a == nil || b == nil {
		return true
	}
	// otherwise they must be the same length
	aLen := len(a)
	if aLen != len(b) {
		return false
	} else if aLen == 0 {
		// if this is a diff for a zero-field message... sure, I guess they are the
		// same.
		return true
	}
	// otherwise if the base of the underlying array the same, then they are the
	// same.
	return &a[0] == &b[0]
}

// Combine combines the changes and errors from `diffs` into a single new Diff.
//
// All provided diffs must either originate from the same template or contain
// errors. Otherwise this this will return a *Diff with an error indicating as
// such.
//
// The mutation in the returned diff will be computed using a simple overlay;
// the last diff to set a given field wins. So if the first diff sets fields
// {a=a1, b=b1}, and the second sets {b=b2, c=c2}, then the final diff will
// contain {a=a1, b=b2, c=c2}. This is done without regard to ApplyMode.
func Combine[T proto.Message](diffs ...*Diff[T]) *Diff[T] {
	if len(diffs) == 0 {
		return nil
	} else if len(diffs) == 1 {
		return diffs[0]
	}

	var errs []error
	var tmpl []ApplyMode
	for _, diff := range diffs {
		if diff == nil {
			continue
		}
		if !sameTemplate(tmpl, diff.applyMode) {
			return &Diff[T]{
				err: errors.New("cannot delta.Combine diffs from a different templates."),
			}
		}
		if tmpl == nil && diff.applyMode != nil {
			tmpl = diff.applyMode
		}
		if diff.err != nil {
			errs = append(errs, diff.err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return &Diff[T]{err: err}
	}

	var zero T
	msg := zero.ProtoReflect().New()

	for _, diff := range diffs {
		if diff == nil {
			continue
		}
		diff.msg.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
			msg.Set(fd, v)
			return true
		})
	}
	return &Diff[T]{msg: msg.Interface().(T), applyMode: tmpl}
}
