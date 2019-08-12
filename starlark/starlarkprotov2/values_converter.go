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
	"math"

	"go.starlark.net/starlark"

	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/starlark/typed"
)

// converter returns typed.Converter that converts to types corresponding to
// the given descriptor.
//
// Ignores field's cardinality.
func converter(l *Loader, fd protoreflect.FieldDescriptor) typed.Converter {
	// Note: primitive type converters needs "stable" addresses, since converters
	// are compared by identity when checking type compatibility. So we use
	// globals for them. For Message types, "stable" addresses are guaranteed by
	// the loader, which caches message types.
	switch fd.Kind() {
	case protoreflect.BoolKind:
		return &boolConverter

	case protoreflect.EnumKind, protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return &int32Converter

	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return &uint32Converter

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return &int64Converter

	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return &uint64Converter

	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return &floatConverter

	case protoreflect.StringKind, protoreflect.BytesKind:
		return &stringConverter

	case protoreflect.MessageKind, protoreflect.GroupKind:
		return l.MessageType(fd.Message())

	default:
		panic(fmt.Errorf("unexpected field kind %s", fd.Kind()))
	}
}

var (
	boolConverter = simpleConverter{
		tp: "bool",
		cb: func(x starlark.Value) (starlark.Value, error) {
			if _, ok := x.(starlark.Bool); ok {
				return x, nil
			}
			return nil, fmt.Errorf("got %s, want bool", x.Type())
		},
	}

	floatConverter = simpleConverter{
		tp: "float",
		cb: func(x starlark.Value) (starlark.Value, error) {
			if _, ok := x.(starlark.Float); ok {
				return x, nil
			}
			if i, ok := x.(starlark.Int); ok {
				return i.Float(), nil
			}
			return nil, fmt.Errorf("got %s, want float", x.Type())
		},
	}

	stringConverter = simpleConverter{
		tp: "string",
		cb: func(x starlark.Value) (starlark.Value, error) {
			if _, ok := x.(starlark.String); ok {
				return x, nil
			}
			return nil, fmt.Errorf("got %s, want string", x.Type())
		},
	}

	int32Converter = intConverter{
		tp:  "int32",
		min: math.MinInt32,
		max: math.MaxInt32,
	}

	uint32Converter = uintConverter{
		tp:  "uint32",
		max: math.MaxUint32,
	}

	int64Converter = intConverter{
		tp:  "int64",
		min: math.MinInt64,
		max: math.MaxInt64,
	}

	uint64Converter = uintConverter{
		tp:  "uint64",
		max: math.MaxUint64,
	}
)

type simpleConverter struct {
	tp string
	cb func(x starlark.Value) (starlark.Value, error)
}

func (c *simpleConverter) Type() string                                     { return c.tp }
func (c *simpleConverter) Convert(x starlark.Value) (starlark.Value, error) { return c.cb(x) }

type intConverter struct {
	tp  string
	min int64
	max int64
}

func (c *intConverter) Type() string { return c.tp }

func (c *intConverter) Convert(x starlark.Value) (starlark.Value, error) {
	si, ok := x.(starlark.Int)
	if !ok {
		return nil, fmt.Errorf("got %s, want int", x.Type())
	}
	if i, ok := si.Int64(); !ok || i < c.min || i > c.max {
		return nil, fmt.Errorf("value %s doesn't fit into %q", x, c.tp)
	}
	return si, nil
}

type uintConverter struct {
	tp  string
	max uint64
}

func (c *uintConverter) Type() string { return c.tp }

func (c *uintConverter) Convert(x starlark.Value) (starlark.Value, error) {
	si, ok := x.(starlark.Int)
	if !ok {
		return nil, fmt.Errorf("got %s, want int", x.Type())
	}
	if i, ok := si.Uint64(); !ok || i > c.max {
		return nil, fmt.Errorf("value %s doesn't fit into %q", x, c.tp)
	}
	return si, nil
}
