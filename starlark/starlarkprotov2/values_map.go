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
func toStarlarkDict(l *Loader, fd protoreflect.FieldDescriptor, m protoreflect.Map) (*typed.Dict, error) {
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
	var err error
	m.Range(func(k protoreflect.MapKey, _ protoreflect.Value) bool {
		var sv starlark.Value
		if sv, err = toStarlarkSingular(l, keyFD, k.Value()); err != nil {
			err = fmt.Errorf("key %s: %s", k, err)
			return false
		}
		keys = append(keys, key{k, sv})
		return true
	})
	if err != nil {
		return nil, err
	}

	// Sort keys as Starlark values.
	var sortErr error
	sort.Slice(keys, func(i, j int) bool {
		lt, err := starlark.Compare(syntax.LT, keys[i].sv, keys[j].sv)
		if err != nil {
			sortErr = err
		}
		return lt
	})
	if sortErr != nil {
		return nil, fmt.Errorf("failed to sort map keys: %s", sortErr)
	}

	// Construct starlark.Dict from sorted keys and values from the proto map.
	d := newStarlarkDict(l, fd, len(keys))
	for _, k := range keys {
		sv, err := toStarlarkSingular(l, valFD, m.Get(k.pv))
		if err != nil {
			return nil, fmt.Errorf("value of key %s: %s", k.pv, err)
		}
		if err := d.SetKey(k.sv, sv); err != nil {
			return nil, fmt.Errorf("value of key %s: %s", k.pv, err)
		}
	}

	return d, nil
}

// assignProtoMap does m.fd = iter, where fd is a map.
//
// Assumes type checks have been done already (this is responsibility of
// prepareRHS). Panics on type mismatch.
func assignProtoMap(m protoreflect.Message, fd protoreflect.FieldDescriptor, dict *typed.Dict) {
	mp := m.Mutable(fd).Map()
	if mp.Len() != 0 {
		panic("the proto map is not empty")
	}

	keyFD := fd.MapKey()
	valFD := fd.MapValue()

	it := dict.Iterate()
	defer it.Done()

	var sk starlark.Value
	for it.Next(&sk) {
		switch sv, ok, err := dict.Get(sk); {
		case err != nil:
			panic(fmt.Errorf("key %s: %s", sk, err))
		case !ok:
			panic(fmt.Errorf("key %s: suddenly gone while iterating", sk))
		default:
			mp.Set(toProtoSingular(keyFD, sk).MapKey(), toProtoSingular(valFD, sv))
		}
	}
}
