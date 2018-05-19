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

package interpreter

import (
	"fmt"

	"github.com/google/skylark"
)

// mutableImpl implements mutable() builtin.
//
// It returns an immutable handle to a mutable value. This allows loaded modules
// (that are frozen) to have a mutable state.
//
// Values of type 'mutable' are true-ish if they hold some value (regardless of
// what it is, unless it is None), and false-ish if they hold no value (i.e.
// it is None).
//
// For example bool(mutable(0)) is True, but bool(mutable()) is False.
func mutableImpl(thread *skylark.Thread, _ *skylark.Builtin, args skylark.Tuple, kwargs []skylark.Tuple) (skylark.Value, error) {
	if len(kwargs) != 0 {
		return nil, fmt.Errorf("'mutable' doesn't accept kwargs")
	}
	if len(args) > 1 {
		return nil, fmt.Errorf("'mutable' got %d arguments, wants 1 or 0", len(args))
	}
	v := &mutableValue{}
	if len(args) == 1 {
		v.v = args[0]
	} else {
		v.v = skylark.None
	}
	return v, nil
}

// mutableValue implements skylark.Value and skylark.HasAttrs.
type mutableValue struct {
	v skylark.Value
}

func (v *mutableValue) String() string        { return v.v.String() }
func (v *mutableValue) Type() string          { return "mutable" }
func (v *mutableValue) Freeze()               {} // non-freezable by design
func (v *mutableValue) Truth() skylark.Bool   { return v.v != skylark.None }
func (v *mutableValue) Hash() (uint32, error) { return 0, fmt.Errorf("non hashable") }
func (v *mutableValue) AttrNames() []string   { return []string{"get", "set"} }

func (v *mutableValue) Attr(name string) (skylark.Value, error) {
	switch name {
	case "get":
		return mutableGetImpl.BindReceiver(v), nil
	case "set":
		return mutableSetImpl.BindReceiver(v), nil
	default:
		return nil, nil
	}
}

var mutableGetImpl = skylark.NewBuiltin(
	"get", func(_ *skylark.Thread, fn *skylark.Builtin, args skylark.Tuple, kwargs []skylark.Tuple) (skylark.Value, error) {
		if len(kwargs) != 0 || len(args) != 0 {
			return nil, fmt.Errorf("'get' doesn't accept arguments")
		}
		return fn.Receiver().(*mutableValue).v, nil
	},
)

var mutableSetImpl = skylark.NewBuiltin(
	"set", func(_ *skylark.Thread, fn *skylark.Builtin, args skylark.Tuple, kwargs []skylark.Tuple) (skylark.Value, error) {
		if len(kwargs) != 0 || len(args) != 1 {
			return nil, fmt.Errorf("'set' accepts 1 positional argument")
		}
		fn.Receiver().(*mutableValue).v = args[0]
		return skylark.None, nil
	},
)
