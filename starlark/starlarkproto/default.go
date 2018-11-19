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
	"reflect"

	"go.starlark.net/starlark"
)

// newDefaultValue returns a new "zero" starlark value for a field described by
// the given Go type.
func newDefaultValue(typ reflect.Type) (starlark.Value, error) {
	switch typ.Kind() {
	case reflect.Bool:
		return starlark.False, nil

	case reflect.Float32, reflect.Float64:
		return starlark.Float(0), nil

	case reflect.Int32, reflect.Int64, reflect.Uint32, reflect.Uint64:
		return starlark.MakeInt(0), nil

	case reflect.String:
		return starlark.String(""), nil

	case reflect.Slice: // this is either a repeated field or 'bytes'
		return starlark.NewList(nil), nil

	case reflect.Ptr: // a message field (*Struct) or proto2 scalar field (*int64)
		if typ.Elem().Kind() != reflect.Struct {
			return nil, errNoProto2
		}
		t, err := GetMessageType(typ)
		if err != nil {
			return nil, fmt.Errorf("can't instantiate value for go type %q - %s", typ, err)
		}
		return NewMessage(t), nil
	}

	return nil, fmt.Errorf("do not know how to instantiate starlark value for go type %q", typ)
}
