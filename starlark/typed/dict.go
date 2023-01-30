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

package typed

import (
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// Dict is a Starlark value that looks like a regular dict, but implements type
// checks and implicit conversions for keys and values when assigning elements.
//
// Note that keys(), values() and items() methods return regular lists, not
// typed ones.
//
// Differences from regular dicts:
//   - Not comparable by value to regular dicts, only to other typed dicts.
//     go.starlark.net has no way to express that typed dicts should be
//     comparable to regular dicts.
//   - update() doesn't support keyword arguments. Using keyword arguments
//     implies string keys which make no sense in generic typed Dict interface,
//     since generally strings are not valid keys. This also substantially
//     simplifies the implementation of update().
type Dict struct {
	keyT Converter      // defines the type of keys
	valT Converter      // defines the type of values
	dict *starlark.Dict // the underlying untyped dict
}

// Note that these interfaces intersect, so we can't mush them into a single
// interface to check *Dict conforms it.
var (
	_ starlark.Value           = (*Dict)(nil)
	_ starlark.Sequence        = (*Dict)(nil)
	_ starlark.IterableMapping = (*Dict)(nil)
	_ starlark.HasAttrs        = (*Dict)(nil)
	_ starlark.HasSetKey       = (*Dict)(nil)
	_ starlark.Comparable      = (*Dict)(nil)
)

// NewDict returns a dict with give preallocated capacity.
func NewDict(key, val Converter, size int) *Dict {
	return &Dict{
		keyT: key,
		valT: val,
		dict: starlark.NewDict(size),
	}
}

// AsTypedDict allocates a new dict<k,v>, and copies it from 'x'.
//
// Returns an error if 'x' is not an iterable mapping or some of its (k, v)
// pairs can't be converted to requested types.
func AsTypedDict(k, v Converter, x starlark.Value) (*Dict, error) {
	m, ok := x.(starlark.IterableMapping)
	if !ok {
		return nil, fmt.Errorf("got %s, want an iterable mapping", x.Type())
	}
	items := m.Items()
	d := NewDict(k, v, len(items))
	for _, kv := range items {
		if err := d.SetKey(kv[0], kv[1]); err != nil {
			return nil, err
		}
	}
	return d, nil
}

// KeyConverter returns the type converter used for keys of this dict.
func (d *Dict) KeyConverter() Converter { return d.keyT }

// ValueConverter returns the type converter used for values of this dict.
func (d *Dict) ValueConverter() Converter { return d.valT }

// conv calls Convert on a (k, v) pair, annotating the error.
func (d *Dict) conv(k, v starlark.Value) (starlark.Value, starlark.Value, error) {
	var err error
	k, err = d.keyT.Convert(k)
	if err != nil {
		return nil, nil, fmt.Errorf("bad key: %s", err)
	}
	v, err = d.valT.Convert(v)
	if err != nil {
		return nil, nil, fmt.Errorf("bad value: %s", err)
	}
	return k, v, nil
}

// starlark.Dict methods we reimplement or proxy.

func (d *Dict) Type() string {
	return fmt.Sprintf("dict<%s,%s>", d.keyT.Type(), d.valT.Type())
}

func (d *Dict) String() string {
	return fmt.Sprintf("%s(%s)", d.Type(), d.dict.String())
}

func (d *Dict) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: %s", d.Type())
}

func (d *Dict) SetKey(k, v starlark.Value) error {
	k, v, err := d.conv(k, v)
	if err != nil {
		return err
	}
	return d.dict.SetKey(k, v)
}

func (d *Dict) AttrNames() []string {
	// Note: we hardcode the list of methods (rather than call d.dict.AttrNames)
	// to be able to notice (via the failing test) when go.starlark.net adds some
	// new methods that we might need to "wrap" in Attr(...).
	return []string{
		"clear",
		"get",
		"items",
		"keys",
		"pop",
		"popitem",
		"setdefault",
		"update",
		"values",
	}
}

func (d *Dict) Attr(name string) (starlark.Value, error) {
	switch name {
	case "setdefault":
		return d.implSetDefault(), nil
	case "update":
		return d.implUpdate(), nil
	default:
		return d.dict.Attr(name)
	}
}

func (d *Dict) CompareSameType(op syntax.Token, y starlark.Value, depth int) (bool, error) {
	switch op {
	case syntax.EQL:
		return dictsEqual(d, y.(*Dict), depth)
	case syntax.NEQ:
		eq, err := dictsEqual(d, y.(*Dict), depth)
		return !eq, err
	default:
		return false, fmt.Errorf("%q is not implemented for %s", op, d.Type())
	}
}

func dictsEqual(l, r *Dict, depth int) (bool, error) {
	if l.keyT != r.keyT || l.valT != r.valT {
		return false, nil
	}
	return l.dict.CompareSameType(syntax.EQL, r.dict, depth)
}

// starlark.Dict methods delegated without any modifications.

func (d *Dict) Clear() error                                                   { return d.dict.Clear() }
func (d *Dict) Delete(k starlark.Value) (starlark.Value, bool, error)          { return d.dict.Delete(k) }
func (d *Dict) Get(k starlark.Value) (v starlark.Value, found bool, err error) { return d.dict.Get(k) }
func (d *Dict) Items() []starlark.Tuple                                        { return d.dict.Items() }
func (d *Dict) Keys() []starlark.Value                                         { return d.dict.Keys() }
func (d *Dict) Len() int                                                       { return d.dict.Len() }
func (d *Dict) Iterate() starlark.Iterator                                     { return d.dict.Iterate() }
func (d *Dict) Freeze()                                                        { d.dict.Freeze() }
func (d *Dict) Truth() starlark.Bool                                           { return d.dict.Truth() }

// Reimplemented builtins.

// implSetDefault reimplements 'setdefault' via Get and SetKey.
func (d *Dict) implSetDefault() *starlark.Builtin {
	return starlark.NewBuiltin("setdefault", func(th *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var key, dflt starlark.Value = nil, starlark.None
		if err := starlark.UnpackPositionalArgs("setdefault", args, kwargs, 1, &key, &dflt); err != nil {
			return nil, err
		}

		// Convert first to avoid "lazy" type checking for 'dflt'.
		key, dflt, err := d.conv(key, dflt)
		if err != nil {
			return nil, fmt.Errorf("setdefault: %s", err)
		}

		// Do regular d.dict.setdefault(...) logic.
		if v, ok, err := d.dict.Get(key); err != nil {
			return nil, fmt.Errorf("setdefault: %s", err)
		} else if ok {
			return v, nil
		} else if err := d.dict.SetKey(key, dflt); err != nil {
			return nil, fmt.Errorf("setdefault: %s", err)
		} else {
			return dflt, nil
		}
	})
}

// implUpdate reimplements 'update' via SetKey(...).
//
// This is a simplified version that doesn't accept kwargs, since generally
// their stringy keys make no sense for typed dicts.
func (d *Dict) implUpdate() *starlark.Builtin {
	return starlark.NewBuiltin("update", func(th *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		switch {
		case len(kwargs) > 0:
			return nil, fmt.Errorf("update: got unexpected keyword argument")
		case len(args) != 1:
			return nil, fmt.Errorf("update: got %d arguments, want exactly 1", len(args))
		}
		v := args[0]

		// Fastest path: adding items from a typed dict with the exact same
		// signature: can just directly update the dict skipping the conversion.
		if td, ok := v.(*Dict); ok && td.keyT == d.keyT && td.valT == d.valT {
			for i, item := range td.dict.Items() {
				if err := d.dict.SetKey(item[0], item[1]); err != nil {
					return nil, fmt.Errorf("update: element #%d: %s", i, err)
				}
			}
			return starlark.None, nil
		}

		// Fast path: adding items from an IterableMapping (e.g. a regular dict).
		// Need to convert (k, v) by passing them through the converter in SetKey.
		if im, ok := v.(starlark.IterableMapping); ok {
			for i, item := range im.Items() {
				if err := d.SetKey(item[0], item[1]); err != nil {
					return nil, fmt.Errorf("update: element #%d: %s", i, err)
				}
			}
			return starlark.None, nil
		}

		// Slow generic path: an iterable with (k,v) pairs (represented by iterables
		// themselves).
		iter := starlark.Iterate(v)
		if iter == nil {
			return nil, fmt.Errorf("update: got %s, want iterable", v.Type())
		}
		defer iter.Done()

		var pair starlark.Value
		for i := 0; iter.Next(&pair); i++ {
			k, v, err := toKV(pair)
			if err != nil {
				return nil, fmt.Errorf("update: element #%d %s", i, err)
			}
			if err := d.SetKey(k, v); err != nil {
				return nil, fmt.Errorf("update: element #%d: %s", i, err)
			}
		}

		return starlark.None, nil
	})
}

func toKV(pair starlark.Value) (k starlark.Value, v starlark.Value, err error) {
	iter := starlark.Iterate(pair)
	if iter == nil {
		return nil, nil, fmt.Errorf("is not iterable (%s)", pair.Type())
	}
	defer iter.Done()

	switch l := starlark.Len(pair); {
	case l < 0:
		return nil, nil, fmt.Errorf("has unknown length (%s)", pair.Type())
	case l != 2:
		return nil, nil, fmt.Errorf("has length %d, want 2", l)
	}

	// If len(...) == 2, iter MUST return 2 items.
	if !iter.Next(&k) {
		return nil, nil, fmt.Errorf("has bad iterable implementation (%s)", pair.Type())
	}
	if !iter.Next(&v) {
		return nil, nil, fmt.Errorf("has bad iterable implementation (%s)", pair.Type())
	}
	return
}
