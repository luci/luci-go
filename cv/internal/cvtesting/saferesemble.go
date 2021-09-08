// Copyright 2021 The LUCI Authors.
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

package cvtesting

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unsafe"

	"github.com/smartystreets/goconvey/convey"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/assertions"
)

// SafeShouldResemble compares 2 structs, which may include top-evel proto fields,
// and should not include any embedded structs except well known standard types.
//
// This should work in place of GoConvey's ShouldResemble on most of CV's
// structs which may contain protos.
func SafeShouldResemble(actual interface{}, expected ...interface{}) string {
	if len(expected) != 1 {
		return fmt.Sprintf("expected 1 value, got %d", len(expected))
	}
	if diff := convey.ShouldHaveSameTypeAs(actual, expected[0]); diff != "" {
		return diff
	}

	rA, rE := reflect.ValueOf(actual), reflect.ValueOf(expected[0])
	switch rA.Kind() {
	case reflect.Struct:
	case reflect.Ptr:
		switch {
		case rA.IsNil() && rE.IsNil():
			return ""
		case !rA.IsNil() && rE.IsNil():
			return "actual is not nil, but nil is expected"
		case rA.IsNil() && !rE.IsNil():
			return "actual is nil, but not nil is expected"
		}

		if rA.Elem().Kind() == reflect.Struct {
			// Neither is nil at this point, so can dereference both.
			rA, rE = rA.Elem(), rE.Elem()
			break
		}
		fallthrough
	default:
		return fmt.Sprintf("Wrong type %T, must be a pointer to struct or a struct", actual)
	}

	// rA, rE are now structs.

	// Because GoConvey's ShouldResemble may hang when comparing protos,
	// first compare proto fields with ShouldResembleProto,
	// then compare the remaining fields with ShouldResemble.

	// Copy the *values* s.t. resetting proto fields to `nil` doesn't modify the
	// passed arguments.
	typ := rA.Type()
	vA, vE := reflect.New(typ), reflect.New(typ)
	vA.Elem().Set(rA) // shallow-copy
	vE.Elem().Set(rE) // shallow-copy

	buf := strings.Builder{}
	protoMessageType := reflect.TypeOf((*proto.Message)(nil)).Elem()
fieldsLoop:
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		switch {
		case field.Type.Implements(protoMessageType):
			// ShouldResembleProto can handle this.
		case field.Type.Kind() == reflect.Slice && field.Type.Elem().Implements(protoMessageType):
			// ShouldResembleProto can also handle this.
		default:
			// Assume not a proto. In practice, this can be a struct or ptr to a
			// struct with a proto inside. Detecting and bailing in such a case is
			// left as future work if it becomes really necessary.
			continue fieldsLoop
		}

		fA, fE := vA.Elem().Field(i), vE.Elem().Field(i)
		switch {
		case fA.CanInterface() != fE.CanInterface():
			panic(fmt.Errorf("type %s field %s CanInterface behaves differently in actual (%t) and expected (%t)",
				typ.Name(), field.Name, fA.CanInterface(), fE.CanInterface()))
		case fA.CanSet() != fE.CanSet():
			panic(fmt.Errorf("type %s field %s CanSet behaves differently in actual (%t) and expected (%t)",
				typ.Name(), field.Name, fA.CanSet(), fE.CanSet()))
		case !fA.CanInterface() || !fA.CanSet():
			// HACK to make private fields interface-able & settable.
			fA = reflect.NewAt(field.Type, unsafe.Pointer(fA.UnsafeAddr())).Elem()
			fE = reflect.NewAt(field.Type, unsafe.Pointer(fE.UnsafeAddr())).Elem()
		}
		if diff := assertions.ShouldResembleProto(fA.Interface(), fE.Interface()); diff != "" {
			addWithIndent(&buf, "field ."+field.Name+" differs:\n", poorifyIfConveyJSON(diff))
		}
		// Reset proto field to nil.
		fA.Set(reflect.New(field.Type).Elem())
		fE.Set(reflect.New(field.Type).Elem())
	}

	// OK, now compare all non-proto fields.
	if diff := convey.ShouldResemble(vA.Elem().Interface(), vE.Elem().Interface()); diff != "" {
		addWithIndent(&buf, "non-proto fields differ:\n", poorifyIfConveyJSON(diff))
	}
	return strings.TrimSpace(buf.String())
}

func addWithIndent(buf *strings.Builder, section, text string) {
	buf.WriteString(section)
	for _, line := range strings.Split(text, "\n") {
		buf.WriteString("  ")
		buf.WriteString(line)
		buf.WriteRune('\n')
	}
	buf.WriteRune('\n')
}

// poorifyIfConveyJSON detects rich Convey JSON mascarading as string and
// returns only its "poor" component.
//
// Depending on Convey's config, ShouldResemble-like response may be JSON
// in the following format:
//	 {
//		 "Message": ...
//		 "Actual": ...
//		 "Expected": ...
//	 }
// If so, we want just the value of the "Message" part.
func poorifyIfConveyJSON(msg string) string {
	out := map[string]interface{}{}
	if err := json.Unmarshal([]byte(msg), &out); err == nil {
		return out["Message"].(string)
	}
	return msg
}
