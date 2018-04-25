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

// newDefaultValue returns a new "zero" skylark value for a field described by
// the given Go type.
func newDefaultValue(typ reflect.Type) (skylark.Value, error) {
	switch typ.Kind() {
	case reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64:
		return skylark.MakeInt(0), nil

	case reflect.Slice:
		// TODO: detect []byte as special
		return skylark.NewList(nil), nil

	case reflect.Ptr: // a message field
		t, err := GetMessageType(typ)
		if err != nil {
			return nil, fmt.Errorf("can't instantiate value for go type %q - %s", typ, err)
		}
		return NewMessage(t), nil
	}

	return nil, fmt.Errorf("do not know how to instantiate skylark value for go type %q", typ)
}
