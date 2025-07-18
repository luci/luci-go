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

// List is a Starlark value that looks like a regular list, but implements type
// checks and implicit conversions when assigning elements.
//
// Differences from regular lists:
//   - Not comparable by value to regular lists, only to other typed lists.
//     go.starlark.net has no way to express that typed lists should be
//     comparable to regular lists.
//   - Only == and != comparison operators are supported.
//   - `l += ...` is same as `l = l + ...`, not `l.extend(...)`
type List struct {
	itemT Converter      // defines the type of elements
	list  *starlark.List // the underlying untyped list
}

// Note that these interfaces intersect, so we can't mush them into a single
// interface to check *List conforms it.
var (
	_ starlark.Value       = (*List)(nil)
	_ starlark.Sequence    = (*List)(nil)
	_ starlark.Indexable   = (*List)(nil)
	_ starlark.HasBinary   = (*List)(nil)
	_ starlark.HasAttrs    = (*List)(nil)
	_ starlark.HasSetIndex = (*List)(nil)
	_ starlark.Comparable  = (*List)(nil)
)

// NewList returns a list with given elements, converted to necessary type
// through 'item' converter.
//
// All subsequently added elements will be converter through this converter too.
func NewList(item Converter, elems []starlark.Value) (*List, error) {
	l := &List{itemT: item}
	vals := make([]starlark.Value, len(elems))
	for i, e := range elems {
		var err error
		if vals[i], err = l.conv(e, i); err != nil {
			return nil, err
		}
	}
	l.list = starlark.NewList(vals)
	return l, nil
}

// AsTypedList allocates a new list<t>, and copies it from 'x'.
//
// Returns an error if 'x' is not an iterable or some of its elements can't be
// converted to 't'.
func AsTypedList(t Converter, x starlark.Value) (*List, error) {
	it := starlark.Iterate(x)
	if it == nil {
		return nil, fmt.Errorf("got %s, want an iterable", x.Type())
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

	return NewList(t, vals)
}

// conv calls Convert, annotating the error with item's index if idx >= 0.
func (l *List) conv(v starlark.Value, idx int) (starlark.Value, error) {
	switch v, err := l.itemT.Convert(v); {
	case err == nil:
		return v, nil
	case idx >= 0:
		return nil, fmt.Errorf("item #%d: %s", idx, err)
	default:
		return nil, err
	}
}

// Converter returns the type converter used by this list.
func (l *List) Converter() Converter { return l.itemT }

// Binary implements 'l <op> y' (or 'y <op> l', depending on 'side').
//
// Note that *starlark.List has its binary operators implement natively by the
// Starlark evaluator, not via HasBinary interface.
func (l *List) Binary(op syntax.Token, y starlark.Value, side starlark.Side) (starlark.Value, error) {
	switch op {
	case syntax.IN, syntax.NOT_IN:
		return starlark.Binary(op, y, l.list)

	case syntax.STAR: // 2*[1,2] => [1,2,1,2], also typed
		if _, ok := y.(starlark.Int); !ok {
			goto unsupported
		}
		return l.asTypedList(starlark.Binary(op, l.list, y))

	case syntax.PLUS:
		// Convert regular lists to typed lists first.
		if yl, ok := y.(*starlark.List); ok {
			vals := make([]starlark.Value, yl.Len())
			for i := range yl.Len() {
				vals[i] = yl.Index(i)
			}
			asTyped, err := NewList(l.itemT, vals)
			if err != nil {
				return nil, err
			}
			return l.Binary(op, asTyped, side)
		}

		// When adding typed lists their types must match.
		if yl, ok := y.(*List); ok {
			if yl.itemT != l.itemT {
				goto unsupported
			}
			lhs, rhs := maybeSwap(l.list, yl.list, side)
			return l.asTypedList(starlark.Binary(op, lhs, rhs))
		}
	}

unsupported:
	return nil, fmt.Errorf("unknown binary op: %s %s %s", l.Type(), op, y.Type())
}

func (l *List) asTypedList(v starlark.Value, err error) (starlark.Value, error) {
	if err != nil {
		return nil, err
	}
	if asL, ok := v.(*starlark.List); ok {
		return &List{itemT: l.itemT, list: asL}, nil
	}
	return nil, fmt.Errorf("expected a list, but got %q", v.Type())
}

func maybeSwap(l, r starlark.Value, side starlark.Side) (starlark.Value, starlark.Value) {
	if side == starlark.Left {
		return l, r
	}
	return r, l
}

// starlark.List methods we reimplement or proxy.

func (l *List) Append(v starlark.Value) error {
	v, err := l.conv(v, -1)
	if err != nil {
		return err
	}
	return l.list.Append(v)
}

func (l *List) AttrNames() []string {
	// Note: we hardcode the list of methods (rather than call l.list.AttrNames)
	// to be able to notice (via the failing test) when go.starlark.net adds some
	// new methods that we might need to "wrap" in Attr(...).
	return []string{
		"append",
		"clear",
		"extend",
		"index",
		"insert",
		"pop",
		"remove",
	}
}

func (l *List) Attr(name string) (starlark.Value, error) {
	if name == "append" {
		return l.implAppend(), nil // fully reimplemented, no need for orig
	}
	orig, err := l.list.Attr(name)
	if err != nil {
		return nil, err
	}
	switch name {
	case "extend":
		return l.implExtend(orig.(*starlark.Builtin)), nil
	case "insert":
		return l.implInsert(orig.(*starlark.Builtin)), nil
	default:
		return orig, nil // no need to wrap
	}
}

func (l *List) SetIndex(i int, v starlark.Value) error {
	// Note: 'i' is never negative here, the Starlark evaluator deals with
	// negative indexes before calling SetIndex.
	v, err := l.conv(v, i)
	if err != nil {
		return err
	}
	return l.list.SetIndex(i, v)
}

func (l *List) Slice(start, end, step int) starlark.Value {
	return &List{
		itemT: l.itemT,
		list:  l.list.Slice(start, end, step).(*starlark.List),
	}
}

func (l *List) Type() string {
	return fmt.Sprintf("list<%s>", l.itemT.Type())
}

func (l *List) String() string {
	return fmt.Sprintf("list<%s>(%s)", l.itemT.Type(), l.list.String())
}

func (l *List) CompareSameType(op syntax.Token, y starlark.Value, depth int) (bool, error) {
	switch op {
	case syntax.EQL:
		return listsEqual(l, y.(*List), depth)
	case syntax.NEQ:
		eq, err := listsEqual(l, y.(*List), depth)
		return !eq, err
	default:
		return false, fmt.Errorf("%q is not implemented for %s", op, l.Type())
	}
}

func listsEqual(l, r *List, depth int) (bool, error) {
	if l.itemT != r.itemT {
		return false, nil
	}
	return l.list.CompareSameType(syntax.EQL, r.list, depth)
}

// starlark.List methods delegated without any modifications.

func (l *List) Clear() error               { return l.list.Clear() }
func (l *List) Freeze()                    { l.list.Freeze() }
func (l *List) Hash() (uint32, error)      { return l.list.Hash() }
func (l *List) Index(i int) starlark.Value { return l.list.Index(i) }
func (l *List) Iterate() starlark.Iterator { return l.list.Iterate() }
func (l *List) Len() int                   { return l.list.Len() }
func (l *List) Truth() starlark.Bool       { return l.list.Truth() }

// Reimplemented builtins.

// implAppend reimplements 'append' via Append(...).
func (l *List) implAppend() *starlark.Builtin {
	return starlark.NewBuiltin("append", func(th *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var object starlark.Value
		if err := starlark.UnpackPositionalArgs("append", args, kwargs, 1, &object); err != nil {
			return nil, err
		}
		if err := l.Append(object); err != nil {
			return nil, fmt.Errorf("append: %s", err)
		}
		return starlark.None, nil
	})
}

// implExtend reimplements 'extend' via Append(...).
//
// Uses original extend() for the fast path to optimize memory allocations.
func (l *List) implExtend(orig *starlark.Builtin) *starlark.Builtin {
	return starlark.NewBuiltin("extend", func(th *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var iterable starlark.Iterable
		if err := starlark.UnpackPositionalArgs("extend", args, kwargs, 1, &iterable); err != nil {
			return nil, err
		}

		// Fast path: the iterable has the exact same type as us, we can skip the
		// conversion completely, since per Convert idempotency property this will
		// be a noop anyway. By calling original 'extend' implementation we are
		// basically doing one big memcpy(...) instead of adding items one by one.
		if it, ok := iterable.(*List); ok && it.itemT == l.itemT {
			return orig.CallInternal(th, starlark.Tuple{it.list}, nil)
		}

		// Slow path: append items one by one.
		iter := iterable.Iterate()
		defer iter.Done()
		var x starlark.Value
		for i := 0; iter.Next(&x); i++ {
			if err := l.Append(x); err != nil {
				return nil, fmt.Errorf("extend: item #%d: %s", i, err)
			}
		}
		return starlark.None, nil
	})
}

// implInsert implements 'insert' by wrapping the original 'insert'.
//
// It's not easily reimplementable, since starlark.List doesn't expose its
// internal list in a simple way.
func (l *List) implInsert(orig *starlark.Builtin) *starlark.Builtin {
	return starlark.NewBuiltin("insert", func(th *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var index starlark.Value
		var object starlark.Value
		if err := starlark.UnpackPositionalArgs("insert", args, kwargs, 2, &index, &object); err != nil {
			return nil, err
		}
		x, err := l.conv(object, -1)
		if err != nil {
			return nil, fmt.Errorf("insert: %s", err)
		}
		return orig.CallInternal(th, starlark.Tuple{index, x}, nil)
	})
}
