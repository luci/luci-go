// Copyright 2024 The LUCI Authors.
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

package properties

import (
	"context"
	"fmt"
	"reflect"

	"go.chromium.org/luci/common/errors"
)

// registeredProperty is the non-generic implementation of RegisteredProperty.
type registeredProperty struct {
	registry  *Registry
	namespace string
	typ       reflect.Type
}

func matchesRegistry(r1, r2 *Registry) error {
	if r1 == nil {
		return errors.New("properties - Using uninitialized RegisteredProperty (use properties.Register()).")
	}
	if r1 != r2 {
		return errors.New("properties - Using Registered property on wrong properties.Registry.")
	}
	return nil
}

func (rp registeredProperty) getInput(s *State) (ret any) {
	if s == nil {
		return nil
	}

	if err := matchesRegistry(rp.registry, s.registry); err != nil {
		panic(fmt.Errorf("RegisteredPropertyIn[%s](%q): %w", rp.typ, rp.namespace, err))
	}

	return s.initialData[rp.namespace]
}

func (rp registeredProperty) mutateOutput(s *State, cb func(any) (mutated bool)) {
	if s == nil {
		panic(fmt.Errorf("RegisteredPropertyOut[%s](%q): State is nil", rp.typ, rp.namespace))
	}

	if err := matchesRegistry(rp.registry, s.registry); err != nil {
		panic(fmt.Errorf("RegisteredPropertyOut[%s](%q): %w", rp.typ, rp.namespace, err))
	}

	s.outputState[rp.namespace].interact(s, cb)
}

func (rp registeredProperty) setOutput(s *State, newVal any) {
	if s == nil {
		panic(fmt.Errorf("RegisteredPropertyOut[%T](%q): State is nil", newVal, rp.namespace))
	}

	if err := matchesRegistry(rp.registry, s.registry); err != nil {
		panic(fmt.Errorf("RegisteredPropertyOut[%T](%q): %w", newVal, rp.namespace, err))
	}

	s.outputState[rp.namespace].set(s, newVal)
}

// RegisteredPropertyIn is the typesafe interface to read the input value of
// a property.
//
// Obtain this via [RegisterIn] or [MustRegisterIn].
type RegisteredPropertyIn[InT any] struct {
	registeredProperty
}

// GetInputFromState is the same as GetInput, but uses a directly supplied State
// instead of getting it from `ctx`.
func (rp RegisteredPropertyIn[InT]) GetInputFromState(s *State) InT {
	dat, _ := rp.getInput(s).(InT)
	return dat
}

// GetInput returns the initial value for this registered property from the
// State embedded in `ctx`.
//
// If the input did not contain a value for this property, or State is not
// present in `ctx`, this returns `nil`.
//
// This will always return the same value - you MUST NOT mutate `T`. If you
// need to do so, make a copy of it (e.g. with proto.Clone) and then modify the
// copy.
//
// This will panic if:
//   - This RegisteredProperty is uninitialized (i.e. constructed without
//     calling Register).
//   - This RegisteredProperty is registered with a different Registry from the
//     one embedded in `ctx`.
func (rp RegisteredPropertyIn[InT]) GetInput(ctx context.Context) InT {
	return rp.GetInputFromState(GetState(ctx))
}

// RegisteredPropertyOut is the typesafe interface to mutate the output value of
// a property.
//
// Obtain this via [RegisterOut] or [MustRegisterOut].
type RegisteredPropertyOut[OutT any] struct {
	registeredProperty
}

// MutateOutputFromState is the same as MutateOutput, but uses a directly-supplied State
// instead of getting it from `ctx`.
func (rp RegisteredPropertyOut[OutT]) MutateOutputFromState(s *State, cb func(OutT) (mutated bool)) {
	rp.mutateOutput(s, func(a any) bool { return cb(a.(OutT)) })
}

// MutateOutput calls `cb` under a Mutex with the current value of this
// output property from the State embedded in `ctx`.
//
// NOTE: The initial value of the output property is always an empty non-nil
// message/struct. As a convenience, *structpb.Struct types also have their
// Fields value initialized.
//
// This callback may observe the current value and optionally modify it. If
// the callback modifies the value, it must return `mutated=true`. If the
// value is mutated, this will trigger any `notify` callbacks associated with
// *State.
//
// If you need to carry any portion of T outside of the callback, clone this
// inside of `cb` (e.g. with proto.Clone for protos, or other mechanisms for
// ad-hoc structs). Failure to do this will result in data races which `go
// test -race` may, or may not, reveal.
//
// This will panic if:
//   - This RegisteredProperty is uninitialized (i.e. constructed without
//     calling Register).
//   - This RegisteredProperty is registered with a different Registry from the
//     one embedded in `ctx`.
//   - State is nil.
func (rp RegisteredPropertyOut[OutT]) MutateOutput(ctx context.Context, cb func(OutT) (mutated bool)) {
	rp.MutateOutputFromState(GetState(ctx), cb)
}

// SetOutputFromState is the same as SetOutput, but uses a directly-supplied State
// instead of getting it from `ctx`.
func (rp RegisteredPropertyOut[OutT]) SetOutputFromState(s *State, newVal OutT) {
	if reflect.ValueOf(newVal).IsZero() {
		// make sure we always set to an empty value, not really a nil map or a nil
		// proto struct.
		rp.setOutput(s, makeOutType(rp.typ))
	} else {
		rp.setOutput(s, newVal)
	}
}

// SetOutput directly sets the value of this output property under a Mutex on
// the State embedded in `ctx`.
//
// Note that this does not do any cloning/copying of the value - if you need to
// maintain a mutable referEnce to `newVal` after this function, copy the value
// before passing it in.
//
// This will panic if:
//   - This RegisteredProperty is uninitialized (i.e. constructed without
//     calling Register).
//   - This RegisteredProperty is registered with a different Registry from the
//     one embedded in `ctx`.
//   - State is nil.
func (rp RegisteredPropertyOut[OutT]) SetOutput(ctx context.Context, newVal OutT) {
	rp.SetOutputFromState(GetState(ctx), newVal)
}

// RegisteredProperty is the typesafe interface to read the input value of
// a property, as well as mutate the output value of a property.
//
// Note that this exports all the methods of RegisteredPropertyIn[InT] and
// RegisteredPropertyOut[OutT], but has no unique methods of its own. It is also
// possible to directly pass the RegisteredPropertyIn or RegisteredPropertyOut
// as a way to split this into a read-only or write-only portion.
//
// Obtain this via [Register], [MustRegister], [RegisterInOut] or
// [MustRegisterInOut].
type RegisteredProperty[InT, OutT any] struct {
	RegisteredPropertyIn[InT]
	RegisteredPropertyOut[OutT]
}
