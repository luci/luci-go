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
	"sort"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/starlark/typed"
)

// newStarlarkDict returns a new typed.Dict of an appropriate type.
func newStarlarkDict(l *Loader, fd protoreflect.FieldDescriptor, capacity int) *typed.Dict {
	return typed.NewDict(
		converter(l, fd.MapKey()),
		converter(l, fd.MapValue()),
		capacity)
}

// toStarlarkDict does protoreflect.Map => typed.Dict conversion.
//
// Panics if type of 'm' (or some of its items) doesn't match 'fd'.
func toStarlarkDict(l *Loader, fd protoreflect.FieldDescriptor, m protoreflect.Map) *typed.Dict {
	keyFD := fd.MapKey()
	valFD := fd.MapValue()

	// A key of a protoreflect.Map, together with its Starlark copy.
	type key struct {
		pv protoreflect.MapKey // as proto value
		sv starlark.Value      // as starlark value
	}

	// Protobuf maps are unordered, but Starlark dicts retain order, so collect
	// keys first to sort them before adding to a dict.
	keys := make([]key, 0, m.Len())
	m.Range(func(k protoreflect.MapKey, _ protoreflect.Value) bool {
		keys = append(keys, key{k, toStarlarkSingular(l, keyFD, k.Value())})
		return true
	})

	// Sort keys as Starlark values. Proto map keys are (u)int(32|64) or bools,
	// they *must* be sortable, so panic on errors.
	sort.Slice(keys, func(i, j int) bool {
		lt, err := starlark.Compare(syntax.LT, keys[i].sv, keys[j].sv)
		if err != nil {
			panic(fmt.Errorf("internal error: when sorting dict keys: %s", err))
		}
		return lt
	})

	// Construct typed.Dict from sorted keys and values from the proto map.
	d := newStarlarkDict(l, fd, len(keys))
	for _, k := range keys {
		sv := toStarlarkSingular(l, valFD, m.Get(k.pv))
		if err := d.SetKey(k.sv, sv); err != nil {
			panic(fmt.Errorf("internal error: value of key %s: %s", k.pv, err))
		}
	}

	return d
}

// assignProtoMap does m.fd = iter, where fd is a map.
//
// Assumes type checks have been done already (this is responsibility of
// prepareRHS). Panics on type mismatch.
func assignProtoMap(m protoreflect.Message, fd protoreflect.FieldDescriptor, dict *typed.Dict) {
	mp := m.Mutable(fd).Map()
	if mp.Len() != 0 {
		panic(fmt.Errorf("internal error: the proto map is not empty"))
	}

	keyFD := fd.MapKey()
	valFD := fd.MapValue()

	it := dict.Iterate()
	defer it.Done()

	var sk starlark.Value
	for it.Next(&sk) {
		switch sv, ok, err := dict.Get(sk); {
		case err != nil:
			panic(fmt.Errorf("internal error: key %s: %s", sk, err))
		case !ok:
			panic(fmt.Errorf("internal error: key %s: suddenly gone while iterating", sk))
		default:
			mp.Set(toProtoSingular(keyFD, sk).MapKey(), toProtoSingular(valFD, sv))
		}
	}
}
