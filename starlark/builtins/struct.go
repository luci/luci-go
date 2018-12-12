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

	// GenStruct is genstruct(name) builtin.
	//
	//  def genstruct(name):
	//    """Returns a callable constructor for "branded" struct instances."""
	GenStruct = starlark.NewBuiltin("genstruct", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var name string
		if err := starlark.UnpackArgs("genstruct", args, kwargs, "name", &name); err != nil {
			return nil, err
		}
		return &ctor{name: name}, nil
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

// Ctor is a callable that produces starlark structs that have it as a
// constructor.
type ctor struct{ name string }

var _ starlark.Callable = (*ctor)(nil)

func (c *ctor) Name() string          { return c.name }
func (c *ctor) String() string        { return c.name }
func (c *ctor) Type() string          { return "ctor" }
func (c *ctor) Freeze()               {} // immutable
func (c *ctor) Truth() starlark.Bool  { return starlark.True }
func (c *ctor) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: ctor") }

func (c *ctor) CallInternal(_ *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 {
		return nil, fmt.Errorf("%s: unexpected positional arguments", c)
	}
	return starlarkstruct.FromKeywords(c, kwargs), nil
}
