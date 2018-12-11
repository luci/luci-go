// Copyright 2018 The LUCI Authors.
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

package builtins

import (
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

var (
	// Struct is struct(**kwargs) builtin.
	//
	//  def struct(**kwargs):
	//    """Returns an immutable object with fields set to given values."""
	Struct = starlark.NewBuiltin("struct", starlarkstruct.Make)

	// GenSym is gensym(name) builtin.
	//
	//  def gensym(name):
	//    """Returns a callable constructor for "branded" struct instances."""
	GenSym = starlark.NewBuiltin("gensym", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var name string
		if err := starlark.UnpackArgs("gensym", args, kwargs, "name", &name); err != nil {
			return nil, err
		}
		return &symbol{name: name}, nil
	})

	// Ctor is ctor(obj) builtin.
	//
	//  def ctor(obj):
	//    """Returns a constructor used to construct this struct or None."""
	Ctor = starlark.NewBuiltin("ctor", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var obj starlark.Value
		if err := starlark.UnpackArgs("ctor", args, kwargs, "obj", &obj); err != nil {
			return nil, err
		}
		if st, ok := obj.(*starlarkstruct.Struct); ok {
			return st.Constructor(), nil
		}
		return starlark.None, nil
	})
)

// Note: the implementation below is copied from
// https://github.com/google/starlark-go/blob/master/starlarkstruct/struct_test.go

// A symbol is a distinct value that acts as a constructor of "branded"
// struct instances, like a class symbol in Python or a "provider" in Bazel.
type symbol struct{ name string }

var _ starlark.Callable = (*symbol)(nil)

func (sym *symbol) Name() string          { return sym.name }
func (sym *symbol) String() string        { return sym.name }
func (sym *symbol) Type() string          { return "symbol" }
func (sym *symbol) Freeze()               {} // immutable
func (sym *symbol) Truth() starlark.Bool  { return starlark.True }
func (sym *symbol) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: %s", sym.Type()) }

func (sym *symbol) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 {
		return nil, fmt.Errorf("%s: unexpected positional arguments", sym)
	}
	return starlarkstruct.FromKeywords(sym, kwargs), nil
}
