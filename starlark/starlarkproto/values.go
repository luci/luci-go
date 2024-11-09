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

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/starlark/typed"
)

// toStarlark converts a protobuf value (whose kind is described by the given
// descriptor) to a Starlark value, using the loader to instantiate messages.
//
// Type of 'v' should match 'fd' (panics otherwise). This is always the case
// because we extract both 'v' and 'fd' from the same protoreflect message.
func toStarlark(l *Loader, fd protoreflect.FieldDescriptor, v protoreflect.Value) starlark.Value {
	switch {
	case fd.IsList():
		if !v.IsValid() { // unset repeated field => empty list<T>
			return newStarlarkList(l, fd)
		}
		return toStarlarkList(l, fd, v.List())

	case fd.IsMap():
		if !v.IsValid() { // unset map field => empty dict<K,V>
			return newStarlarkDict(l, fd, 0)
		}
		return toStarlarkDict(l, fd, v.Map())

	default:
		return toStarlarkSingular(l, fd, v)
	}
}

// prepareRHS examines right hand side of some assignment `msg.fd = v`.
//
// It checks whether 'v' can be assigned to a field described by 'fd', and
// converts it to necessary type (using 'converter(fd)') if required.
//
// On success returns a value of a type matching 'fd'. It can either be 'v' as
// is, or a type-converter copy of 'v'.
//
// See doc.go for the conversion rules.
func prepareRHS(l *Loader, fd protoreflect.FieldDescriptor, v starlark.Value) (starlark.Value, error) {
	switch {
	case fd.IsList():
		// Reuse list<T> by reference if it has matching type. Reject it right away
		// if it is some other list<T'>, T'!=T. Otherwise make a copy.
		myT := converter(l, fd)
		if tpd, ok := v.(*typed.List); ok {
			if tpd.Converter() == myT {
				return v, nil // exact same type, reuse by reference
			}
			return nil, fmt.Errorf("want list<%s> or just list", myT.Type())
		}
		v, err := typed.AsTypedList(myT, v) // make a copy
		if err != nil {
			return nil, fmt.Errorf("when constructing list<%s>: %s", myT.Type(), err)
		}
		return v, nil

	case fd.IsMap():
		// Reuse dict<K,V> by reference if it has matching type. Reject it right
		// away if it is some other dict<K',V'>, K'!=K || V'!=V. Otherwise make
		// a copy.
		myK := converter(l, fd.MapKey())
		myV := converter(l, fd.MapValue())
		if tpd, ok := v.(*typed.Dict); ok {
			if tpd.KeyConverter() == myK && tpd.ValueConverter() == myV {
				return v, nil // exact same type, reuse by reference
			}
			return nil, fmt.Errorf("want dict<%s,%s> or just dict", myK.Type(), myV.Type())
		}
		v, err := typed.AsTypedDict(myK, myV, v) // make a copy
		if err != nil {
			return nil, fmt.Errorf("when constructing dict<%s,%s>: %s", myK.Type(), myV.Type(), err)
		}
		return v, nil

	default:
		// Attempt to type-cast 'v' to the necessary type. In particular this
		// converts dict to messages, checks int ranges, etc.
		return converter(l, fd).Convert(v)
	}
}

// assign does 'm.fd = v'.
//
// Here 'v' is a result of some prepareRHS(.., fd, ...) or toStarlark(fd, ...)
// call. Panics if type of 'v' doesn't match 'fd'. This should not be possible.
//
// m.fd is assumed to be in the initial state (i.e. if it is a list or a map,
// they are empty). Panics otherwise.
func assign(m protoreflect.Message, fd protoreflect.FieldDescriptor, v starlark.Value) {
	switch {
	case fd.IsList():
		assignProtoList(m, fd, v.(*typed.List))
	case fd.IsMap():
		assignProtoMap(m, fd, v.(*typed.Dict))
	default:
		m.Set(fd, toProtoSingular(fd, v))
	}
}
