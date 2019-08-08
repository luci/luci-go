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

	"go.starlark.net/starlark"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// assigner assigns some value to some field of 'm'.
//
// It allows to split assignment 'm.fd=val' into two phases:
//   1. Check type of 'val' matches 'fd', produce 'assigner' if so.
//   2. Actually do the assignment in 'assigner(m)'.
//
// This is useful since step (1) is used independently of step (2) in SetField,
// where we just want to check the type, but don't need to do any proto message
// manipulations yet.
type assigner func(m protoreflect.Message) error

// instantiator creates some new singular protoreflect.Value on demand.
//
// Its application is similar to the 'assigner' described above: during
// 'm.fd=val' assignment it allows to delay conversion of 'val' into proto to
// a later stage.
type instantiator func() (protoreflect.Value, error)

// toStarlark converts a protobuf value (whose kind is described by the given
// descriptor) to a Starlark value, using the loader to instantiate messages.
//
// Type of 'v' should match 'fd' (panics otherwise). This is always the case
// because we extract both 'v' and 'fd' from the same protoreflect message.
func toStarlark(l *Loader, fd protoreflect.FieldDescriptor, v protoreflect.Value) (starlark.Value, error) {
	switch {
	case fd.IsList():
		if !v.IsValid() { // unset repeated field => empty list
			return starlark.NewList(nil), nil
		}
		return toStarlarkList(l, fd, v.List())

	case fd.IsMap():
		if !v.IsValid() { // unset map field => empty dict
			return starlark.NewDict(0), nil
		}
		return toStarlarkDict(l, fd, v.Map())

	default:
		return toStarlarkSingular(l, fd, v)
	}
}

// assign does 'm.fd = v' (even if fd is a repeated field or a map).
//
// Returns an error if type of 'v' doesn't match 'fd'.
//
// m.fd is assumed to be in the initial state (i.e. if it is a list or a map,
// they are empty). Panics otherwise.
func assign(m protoreflect.Message, fd protoreflect.FieldDescriptor, v starlark.Value) error {
	a, err := getAssigner(fd, v)
	if err != nil {
		return err
	}
	return a(m)
}

// canAssign returns an error if the given Starlark value can't be assigned to
// a proto field with the given descriptor.
//
// Runs in O(1). Doesn't recurse into lists or tuples. Type mismatches there
// will be discovered in 'assign'.
func canAssign(fd protoreflect.FieldDescriptor, v starlark.Value) error {
	_, err := getAssigner(fd, v)
	return err
}

// getAssigner returns a callback that sets field 'fd' to 'v' in some message.
//
// Does initial light O(1) type checks. All heavier O(N) checks are done in
// the returned callback which actually does the assignment.
func getAssigner(fd protoreflect.FieldDescriptor, v starlark.Value) (assigner, error) {
	switch {
	case fd.IsList():
		iter, ok := v.(starlark.Iterable)
		if !ok {
			return nil, fmt.Errorf("can't assign %q to a repeated field", v.Type())
		}
		return func(m protoreflect.Message) error { return assignProtoList(m, fd, iter) }, nil

	case fd.IsMap():
		iter, ok := v.(starlark.IterableMapping)
		if !ok {
			return nil, fmt.Errorf("can't assign %q to a map field", v.Type())
		}
		return func(m protoreflect.Message) error { return assignProtoMap(m, fd, iter) }, nil

	default:
		futVal, err := prepProtoSingular(fd, v)
		if err != nil {
			return nil, err
		}
		return func(m protoreflect.Message) error {
			val, err := futVal()
			if err != nil {
				return err
			}
			m.Set(fd, val)
			return nil
		}, nil
	}
}
