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
)

// toStarlarkSingular converts 'v' to starlark, based on type in 'fd'.
//
// This is Proto => Starlark converter. Ignores 'repeated' qualifier.
func toStarlarkSingular(l *Loader, fd protoreflect.FieldDescriptor, v protoreflect.Value) (starlark.Value, error) {
	// See https://godoc.org/google.golang.org/protobuf/reflect/protoreflect#Kind
	// Also https://developers.google.com/protocol-buffers/docs/proto#scalar

	switch fd.Kind() {
	case protoreflect.BoolKind:
		return starlark.Bool(v.Bool()), nil

	case protoreflect.EnumKind:
		return starlark.MakeInt(int(v.Enum())), nil

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return starlark.MakeInt64(v.Int()), nil

	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return starlark.MakeUint64(v.Uint()), nil

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return starlark.MakeInt64(v.Int()), nil

	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return starlark.MakeUint64(v.Uint()), nil

	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return starlark.Float(v.Float()), nil

	case protoreflect.StringKind:
		return starlark.String(v.String()), nil

	case protoreflect.BytesKind:
		return starlark.String(v.Bytes()), nil

	case protoreflect.MessageKind, protoreflect.GroupKind:
		msg := l.MessageType(fd.Message()).NewMessage()
		if v.IsValid() {
			if err := msg.FromProto(v.Message().Interface()); err != nil {
				return nil, err
			}
		}
		return msg, nil

	default:
		return nil, fmt.Errorf("unexpected field kind %s", fd.Kind())
	}
}

// toProtoSingular converts 'v' to a protobuf value.
//
// Runs in O(N). If you want to do O(1) type check only, use prepProtoSingular
// instead.
func toProtoSingular(fd protoreflect.FieldDescriptor, v starlark.Value) (protoreflect.Value, error) {
	val, err := prepProtoSingular(fd, v)
	if err != nil {
		return protoreflect.Value{}, err
	}
	return val()
}

// prepProtoSingular verifies 'v' can be assigned to field of type 'fd' and
// returns a closure that instantiates the corresponding protobuf value when
// called.
//
// Runs in O(1) itself. The returned closure runs in O(N).
func prepProtoSingular(fd protoreflect.FieldDescriptor, v starlark.Value) (instantiator, error) {
	// See https://godoc.org/google.golang.org/protobuf/reflect/protoreflect#Kind
	// Also https://developers.google.com/protocol-buffers/docs/proto#scalar
	switch kind := fd.Kind(); kind {
	case protoreflect.BoolKind:
		// We require bool fields to be explicitly assigned True or False, no
		// implicit conversion to bool from other types.
		bv, ok := v.(starlark.Bool)
		if !ok {
			return nil, typeErr(v, kind)
		}
		return valueOf(bool(bv)), nil

	case protoreflect.EnumKind:
		// Enums in proto are signed int32.
		i, err := asInt(v, kind, math.MinInt32, math.MaxInt32)
		if err != nil {
			return nil, err
		}
		return valueOf(protoreflect.EnumNumber(i)), nil

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		i, err := asInt(v, kind, math.MinInt32, math.MaxInt32)
		if err != nil {
			return nil, err
		}
		return valueOf(int32(i)), nil

	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		i, err := asUint(v, kind, math.MaxUint32)
		if err != nil {
			return nil, err
		}
		return valueOf(uint32(i)), nil

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		i, err := asInt(v, kind, math.MinInt64, math.MaxInt64)
		if err != nil {
			return nil, err
		}
		return valueOf(int64(i)), nil

	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		i, err := asUint(v, kind, math.MaxUint64)
		if err != nil {
			return nil, err
		}
		return valueOf(uint64(i)), nil

	case protoreflect.FloatKind:
		f, ok := starlark.AsFloat(v)
		if !ok {
			return nil, typeErr(v, kind)
		}
		return valueOf(float32(f)), nil

	case protoreflect.DoubleKind:
		f, ok := starlark.AsFloat(v)
		if !ok {
			return nil, typeErr(v, kind)
		}
		return valueOf(float64(f)), nil

	case protoreflect.StringKind:
		asStr, ok := v.(starlark.String)
		if !ok {
			return nil, typeErr(v, kind)
		}
		return valueOf(asStr.GoString()), nil

	case protoreflect.BytesKind:
		asStr, ok := v.(starlark.String)
		if !ok {
			return nil, typeErr(v, kind)
		}
		return valueOf([]byte(asStr.GoString())), nil

	case protoreflect.MessageKind, protoreflect.GroupKind:
		msg, ok := v.(*Message)
		if !ok {
			return nil, typeErr(v, kind)
		}
		if got, want := msg.typ.desc, fd.Message(); got != want {
			return nil, fmt.Errorf("can't assign message %q to a message field %q", got.FullName(), want.FullName())
		}
		return func() (protoreflect.Value, error) {
			pm, err := msg.ToProto()
			if err != nil {
				return protoreflect.Value{}, err
			}
			return protoreflect.ValueOf(pm), nil
		}, nil

	default:
		return nil, fmt.Errorf("unexpected field kind %s", kind)
	}
}

func typeErr(v starlark.Value, k protoreflect.Kind) error {
	return fmt.Errorf("can't assign %q to %q field", v.Type(), k)
}

func valueOf(x interface{}) instantiator {
	return func() (protoreflect.Value, error) { return protoreflect.ValueOf(x), nil }
}

func asInt(v starlark.Value, k protoreflect.Kind, min, max int64) (int64, error) {
	si, ok := v.(starlark.Int)
	if !ok {
		return 0, typeErr(v, k)
	}
	i, ok := si.Int64()
	if !ok || i < min || i > max {
		return 0, fmt.Errorf("value %s doesn't fit into %q field", v, k)
	}
	return i, nil
}

func asUint(v starlark.Value, k protoreflect.Kind, max uint64) (uint64, error) {
	si, ok := v.(starlark.Int)
	if !ok {
		return 0, typeErr(v, k)
	}
	i, ok := si.Uint64()
	if !ok || i > max {
		return 0, fmt.Errorf("value %s doesn't fit into %q field", v, k)
	}
	return i, nil
}
