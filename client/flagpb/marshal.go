// Copyright 2016 The LUCI Authors.
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

package flagpb

import (
	"fmt"
	"reflect"
	"strings"
)

// MarshalUntyped marshals a key-value map to flags.
func MarshalUntyped(msg map[string]any) ([]string, error) {
	return appendFlags(nil, nil, reflect.ValueOf(msg))
}

func appendFlags(flags []string, path []string, v reflect.Value) ([]string, error) {
	name := "-" + strings.Join(path, ".")

	v = indirect(v)

	var err error
	switch v.Kind() {
	case reflect.Map:
		if kind := v.Type().Key().Kind(); kind != reflect.String {
			return nil, fmt.Errorf("map key type must be string, got %s", kind)
		}
		for _, k := range v.MapKeys() {
			flags, err = appendFlags(flags, append(path, k.String()), v.MapIndex(k))
			if err != nil {
				return nil, err
			}
		}

	case reflect.Slice, reflect.Array:
		sep := false
		for i, l := 0, v.Len(); i < l; i++ {
			if sep {
				flags = append(flags, name)
			}
			e := indirect(v.Index(i))
			if flags, err = appendFlags(flags, path, e); err != nil {
				return nil, err
			}
			sep = e.Kind() == reflect.Map
		}

	case reflect.Bool:
		if v.Bool() {
			flags = append(flags, name)
		} else {
			flags = append(flags, name+"=false")
		}

	// numbers and strings
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64,
		reflect.String:
		flags = append(flags, name, fmt.Sprintf("%v", v.Interface()))

	case reflect.Invalid:
		return nil, fmt.Errorf("invalid value")

	default:
		return nil, fmt.Errorf("unsupported type: %s", v.Type())
	}
	return flags, nil
}

func indirect(v reflect.Value) reflect.Value {
	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	return v
}
