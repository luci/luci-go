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
	"math"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/starlark/typed"
)

// toStarlarkSingular converts 'v' to starlark, based on type in 'fd'.
//
// This is Proto => Starlark converter. Ignores 'repeated' qualifier.
//
// Panics if type of 'v' doesn't match 'fd'.
func toStarlarkSingular(l *Loader, fd protoreflect.FieldDescriptor, v protoreflect.Value) starlark.Value {
	// See https://godoc.org/google.golang.org/protobuf/reflect/protoreflect#Kind
	// Also https://developers.google.com/protocol-buffers/docs/proto#scalar

	switch fd.Kind() {
	case protoreflect.BoolKind:
		return starlark.Bool(v.Bool())

	case protoreflect.EnumKind:
		return starlark.MakeInt(int(v.Enum()))

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return starlark.MakeInt64(v.Int())

	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return starlark.MakeUint64(v.Uint())

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return starlark.MakeInt64(v.Int())

	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return starlark.MakeUint64(v.Uint())

	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return starlark.Float(v.Float())

	case protoreflect.StringKind:
		return starlark.String(v.String())

	case protoreflect.BytesKind:
		return starlark.String(v.Bytes())

	case protoreflect.MessageKind, protoreflect.GroupKind:
		typ := l.MessageType(fd.Message())
		if v.IsValid() {
			return typ.MessageFromProto(v.Message().Interface())
		}
		return typ.Message()

	default:
		panic(fmt.Errorf("internal error: unexpected field kind %s", fd.Kind()))
	}
}

// toProtoSingular converts 'v' to a protobuf value described by 'fd'.
//
// This is Starlark => Proto converter.
//
// Assumes type checks have been done already (this is responsibility of
// 'converter' function, see below).
//
// Panics on type mismatch.
func toProtoSingular(fd protoreflect.FieldDescriptor, v starlark.Value) protoreflect.Value {
	// See https://godoc.org/google.golang.org/protobuf/reflect/protoreflect#Kind
	// Also https://developers.google.com/protocol-buffers/docs/proto#scalar
	switch kind := fd.Kind(); kind {
	case protoreflect.BoolKind:
		return protoreflect.ValueOf(bool(v.(starlark.Bool)))

	case protoreflect.EnumKind:
		return protoreflect.ValueOf(protoreflect.EnumNumber(starlarkToInt(v, kind, math.MinInt32, math.MaxInt32)))

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return protoreflect.ValueOf(int32(starlarkToInt(v, kind, math.MinInt32, math.MaxInt32)))

	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return protoreflect.ValueOf(uint32(starlarkToUint(v, kind, math.MaxUint32)))

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return protoreflect.ValueOf(int64(starlarkToInt(v, kind, math.MinInt64, math.MaxInt64)))

	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return protoreflect.ValueOf(uint64(starlarkToUint(v, kind, math.MaxUint64)))

	case protoreflect.FloatKind:
		return protoreflect.ValueOf(float32(v.(starlark.Float)))

	case protoreflect.DoubleKind:
		return protoreflect.ValueOf(float64(v.(starlark.Float)))

	case protoreflect.StringKind:
		return protoreflect.ValueOf(v.(starlark.String).GoString())

	case protoreflect.BytesKind:
		return protoreflect.ValueOf([]byte(v.(starlark.String).GoString()))

	case protoreflect.MessageKind, protoreflect.GroupKind:
		return protoreflect.ValueOf(v.(*Message).ToProto())

	default:
		panic(fmt.Errorf("internal error: unexpected field kind %s", fd.Kind()))
	}
}

// Extracts int64 from 'v' checking its range. Panics on mismatches.
func starlarkToInt(v starlark.Value, k protoreflect.Kind, min, max int64) int64 {
	si, ok := v.(starlark.Int)
	if !ok {
		panic(fmt.Errorf("internal error: got %s, expect %s", v.Type(), k))
	}
	i, ok := si.Int64()
	if !ok || i < min || i > max {
		panic(fmt.Errorf("internal error: %s doesn't fit into %s field", v, k))
	}
	return i
}

// Extract uint64 from 'v' checking its range. Panics on mismatches.
func starlarkToUint(v starlark.Value, k protoreflect.Kind, max uint64) uint64 {
	si, ok := v.(starlark.Int)
	if !ok {
		panic(fmt.Errorf("internal error: got %s, expect %s", v.Type(), k))
	}
	i, ok := si.Uint64()
	if !ok || i > max {
		panic(fmt.Errorf("internal error: %s doesn't fit into %s field", v, k))
	}
	return i
}

// converter returns typed.Converter that converts arbitrary Starlark values to
// another Starlark values of a scalar (i.e. non-repeated, non-dict) type
// described by the descriptor, if such conversion is allowed.
//
// This is Starlark => Starlark converter. Such converter is involved when
// executing following Starlark statements:
//
//	msg.scalar = <some starlark value>
//	msg.repeated[123] = <some starlark value>
//	msg.dict[<some starlark value>] = <some starlark value>
//
// It ensures that *Message fields at all times conform to the proto message
// schema.
//
// Some notable conversion rules:
//   - converter([u]int(32|64)) checks int fits within the corresponding range.
//   - converter(float[32|64]) implicitly converts ints to floats.
//   - converter(Message) implicitly converts dicts and Nones to messages.
//
// The following invariant holds (and relied upon by 'assign'): for all possible
// 'fd' and all possible 'x' the following doesn't panic:
//
//	v, err := converter(l, fd).Convert(x)
//	if err != nil {
//	  return err // type of 'x' is incompatible with 'fd'
//	}
//	p := toProtoSingular(fd, v) // compatible values can be converted to proto
func converter(l *Loader, fd protoreflect.FieldDescriptor) typed.Converter {
	// Note: primitive type converters need "stable" addresses, since converters
	// are compared by identity when checking type compatibility. So we use
	// globals for them. For Message types, "stable" addresses are guaranteed by
	// the loader, which caches types internally.
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
		return l.MessageType(fd.Message()).Converter()

	default:
		panic(fmt.Errorf("internal error: unexpected field kind %s", fd.Kind()))
	}
}

// Converters into built-in types.

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
		return nil, fmt.Errorf("%s doesn't fit into %s", x, c.tp)
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
		return nil, fmt.Errorf("%s doesn't fit into %s", x, c.tp)
	}
	return si, nil
}
