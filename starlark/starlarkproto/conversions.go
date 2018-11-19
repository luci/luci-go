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

package starlarkproto

import (
	"fmt"
	"math"
	"reflect"

	"go.starlark.net/starlark"
)

var errNoProto2 = fmt.Errorf("proto2 messages are not fully supported, update to proto3")

// intRanges describes ranges of integer types that can appear in *.pb.go code.
//
// Note that platform-dependent 'int' is not possible there, nor (s|u)int16 or
// sint8. Only (s|u)int(32|64) and uint8 (when using 'bytes' field).
var intRanges = map[reflect.Kind]struct {
	signed      bool
	minSigned   int64
	maxSigned   int64
	maxUnsigned uint64
}{
	reflect.Uint8:  {false, 0, 0, math.MaxUint8},
	reflect.Int32:  {true, math.MinInt32, math.MaxInt32, 0},
	reflect.Uint32: {false, 0, 0, math.MaxUint32},
	reflect.Int64:  {true, math.MinInt64, math.MaxInt64, 0},
	reflect.Uint64: {false, 0, 0, math.MaxUint64},
}

// getAssigner returns a callback that can assign the given starlark value to
// a go value of the given type, or an error if such assignment is not allowed
// due to incompatible types.
func getAssigner(typ reflect.Type, sv starlark.Value) (func(reflect.Value) error, error) {
	// Proto3 use pointers only to represent message-valued fields. Proto2 also
	// uses them to represent scalar-valued fields. We don't support proto2. So
	// check that if typ is a pointer, it points to a struct.
	if typ.Kind() == reflect.Ptr && typ.Elem().Kind() != reflect.Struct {
		return nil, errNoProto2
	}

	switch val := sv.(type) {
	case starlark.NoneType:
		return nil, fmt.Errorf("can't assign nil to a value of kind %q", typ.Kind())

	case starlark.Bool:
		if typ.Kind() != reflect.Bool {
			return nil, fmt.Errorf("can't assign boolean to a value of kind %q", typ.Kind())
		}
		return func(gv reflect.Value) error {
			gv.SetBool(bool(val))
			return nil
		}, nil

	case starlark.Float:
		if typ.Kind() != reflect.Float64 && typ.Kind() != reflect.Float32 {
			return nil, fmt.Errorf("can't assign float to a value of kind %q", typ.Kind())
		}
		return func(gv reflect.Value) error {
			gv.SetFloat(float64(val))
			return nil
		}, nil

	case starlark.Int:
		// Assigning integer to a float field? Cast to float first.
		if typ.Kind() == reflect.Float64 || typ.Kind() == reflect.Float32 {
			return getAssigner(typ, val.Float())
		}
		// Otherwise check that assigning to an int, and the value is in range.
		intRange, ok := intRanges[typ.Kind()]
		if !ok {
			return nil, fmt.Errorf("can't assign integer to a value of kind %q", typ.Kind())
		}
		if intRange.signed {
			asInt64, ok := val.Int64()
			if !ok || asInt64 > intRange.maxSigned || asInt64 < intRange.minSigned {
				return nil, fmt.Errorf("the integer %s doesn't fit into %s", val, typ.Kind())
			}
			return func(gv reflect.Value) error {
				gv.SetInt(asInt64)
				return nil
			}, nil
		} else {
			asUint64, ok := val.Uint64()
			if !ok || asUint64 > intRange.maxUnsigned || asUint64 < 0 {
				return nil, fmt.Errorf("the integer %s doesn't fit into %s", val, typ.Kind())
			}
			return func(gv reflect.Value) error {
				gv.SetUint(asUint64)
				return nil
			}, nil
		}

	case starlark.String:
		if typ.Kind() != reflect.String {
			return nil, fmt.Errorf("can't assign string to a value of kind %q", typ.Kind())
		}
		return func(gv reflect.Value) error {
			gv.SetString(string(val))
			return nil
		}, nil

	case *starlark.List, starlark.Tuple:
		if typ.Kind() != reflect.Slice {
			return nil, fmt.Errorf("can't assign list to a value of kind %q", typ.Kind())
		}
		return func(gv reflect.Value) error {
			slice := reflect.MakeSlice(gv.Type(), 0, 0) // ~ slice := []T{}

			iter := starlark.Iterate(val)
			defer iter.Done()

			var staritm starlark.Value
			var idx int

			for iter.Next(&staritm) {
				goitm := reflect.New(gv.Type().Elem()).Elem() // ~ goitm := T{}
				if err := assign(goitm, staritm); err != nil {
					return fmt.Errorf("list item #%d - %s", idx, err)
				}
				slice = reflect.Append(slice, goitm)
				idx++
			}

			gv.Set(slice)
			return nil
		}, nil

	case *Message:
		// 'typ' is expected to be *Struct{}
		if typ.Kind() != reflect.Ptr || typ.Elem().Kind() != reflect.Struct {
			return nil, fmt.Errorf("can't assign proto struct to a value of type %q", typ)
		}
		rightTyp := val.MessageType().Type() // also should be *Struct{}
		if typ != rightTyp {
			return nil, fmt.Errorf("incompatible types %q and %q", typ.Elem().Name(), rightTyp.Elem().Name())
		}
		return func(gv reflect.Value) error {
			rightMsg, err := val.ToProto()
			if err != nil {
				return err
			}
			gv.Set(reflect.ValueOf(rightMsg))
			return nil
		}, nil
	}

	return nil, fmt.Errorf("don't know how to handle starlark value of type %q", sv.Type())
}

// checkAssignable returns no errors if the given starlark value can be assigned
// to a go value of the given type.
func checkAssignable(typ reflect.Type, val starlark.Value) error {
	_, err := getAssigner(typ, val)
	return err
}

// assign assigns the given starlark value to the given go value, if types
// allow.
func assign(gv reflect.Value, sv starlark.Value) error {
	assigner, err := getAssigner(gv.Type(), sv)
	if err != nil {
		return err
	}
	return assigner(gv)
}
