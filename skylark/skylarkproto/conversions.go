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

package skylarkproto

import (
	"fmt"
	"reflect"

	"github.com/google/skylark"
)

// getAssigner returns a callback that can assign the given skylark value to
// a go value of the given type, or an error if such assignment is not allowed
// due to incompatible types.
func getAssigner(typ reflect.Type, sv skylark.Value) (func(reflect.Value) error, error) {
	switch val := sv.(type) {
	case skylark.NoneType:
		return nil, fmt.Errorf("can't assign nil to a value of kind %q", typ.Kind())

	case skylark.Int:
		// TODO: add more simple types
		asInt64, ok := val.Int64()
		if !ok {
			return nil, fmt.Errorf("the integer %s doesn't fit into int64", val)
		}
		if typ.Kind() != reflect.Int64 { // TODO: cast to other ints
			return nil, fmt.Errorf("can't assign integer to a value of kind %q", typ.Kind())
		}
		return func(gv reflect.Value) error {
			gv.SetInt(asInt64)
			return nil
		}, nil

	case *skylark.List, skylark.Tuple:
		if typ.Kind() != reflect.Slice {
			return nil, fmt.Errorf("can't assign list to a value of kind %q", typ.Kind())
		}
		return func(gv reflect.Value) error {
			slice := reflect.MakeSlice(gv.Type(), 0, 0) // ~ slice := []T{}

			iter := skylark.Iterate(val)
			defer iter.Done()

			var skyitm skylark.Value
			var idx int

			for iter.Next(&skyitm) {
				goitm := reflect.New(gv.Type().Elem()).Elem() // ~ goitm := T{}
				if err := assign(goitm, skyitm); err != nil {
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

	return nil, fmt.Errorf("don't know how to handle skylark value of type %q", sv.Type())
}

// checkAssignable returns no errors if the given skylark value can be assigned
// to a go value of the given type.
func checkAssignable(typ reflect.Type, val skylark.Value) error {
	_, err := getAssigner(typ, val)
	return err
}

// assign assigns the given skylark value to the given go value, if types allow.
func assign(gv reflect.Value, sv skylark.Value) error {
	assigner, err := getAssigner(gv.Type(), sv)
	if err != nil {
		return err
	}
	return assigner(gv)
}
