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

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/assertions"

	. "github.com/smartystreets/goconvey/convey"
)

// SafeShouldResemble compares 2 structs recursively, which may include proto
// fields.
//
// Inner struct or slice of struct property is okay, but should not include
// any pointer to struct or slice of pointer to struct property.
//
// This should work in place of GoConvey's ShouldResemble on most of CV's
// structs which may contain protos.
func SafeShouldResemble(actual any, expected ...any) string {
	if len(expected) != 1 {
		return fmt.Sprintf("expected 1 value, got %d", len(expected))
	}
	if diff := ShouldHaveSameTypeAs(actual, expected[0]); diff != "" {
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

	// Copy the *values* before passing to `compareStructRecursive` because
	// it will reset proto fields to `nil`.
	typ := rA.Type()
	copyA, copyE := reflect.New(typ).Elem(), reflect.New(typ).Elem()
	copyA.Set(rA) // shallow-copy
	copyE.Set(rE) // shallow-copy
	// Because GoConvey's ShouldResemble may hang when comparing protos,
	// first compare proto fields with ShouldResembleProto,
	// then compare the remaining fields with ShouldResemble.
	buf := &strings.Builder{}
	p := &protoFieldsComparator{
		actual:   copyA,
		expected: copyE,
		diffBuf:  buf,
	}
	p.compareRecursiveAndNilify()
	// OK, now compare all non-proto fields.
	if diff := ShouldResemble(copyA.Interface(), copyE.Interface()); diff != "" {
		buf.WriteRune('\n')
		addWithIndent(buf, "non-proto fields differ:\n", poorifyIfConveyJSON(diff))
	}
	return strings.TrimSpace(buf.String())
}

type protoFieldsComparator struct {
	actual, expected reflect.Value

	parentFields []string
	diffBuf      *strings.Builder
}

var protoMessageType = reflect.TypeOf((*proto.Message)(nil)).Elem()

func (p *protoFieldsComparator) compareRecursiveAndNilify() {
	structType := p.actual.Type()
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		fieldKind := field.Type.Kind()
		fA, fE := p.actual.Field(i), p.expected.Field(i)
		fullPath := strings.Join(append(p.parentFields, field.Name), ".")
		switch {
		case field.Type.Implements(protoMessageType):
			fallthrough
		case fieldKind == reflect.Slice && field.Type.Elem().Implements(protoMessageType):
			switch {
			case fA.CanInterface() != fE.CanInterface():
				panic(fmt.Errorf("type %s field %s CanInterface behaves differently in actual (%t) and expected (%t)",
					structType.Name(), fullPath, fA.CanInterface(), fE.CanInterface()))
			case fA.CanSet() != fE.CanSet():
				panic(fmt.Errorf("type %s field %s CanSet behaves differently in actual (%t) and expected (%t)",
					structType.Name(), fullPath, fA.CanSet(), fE.CanSet()))
			case !fA.CanInterface():
				// HACK to make private fields interface-able.
				fA = reflect.NewAt(field.Type, unsafe.Pointer(fA.UnsafeAddr())).Elem()
				fE = reflect.NewAt(field.Type, unsafe.Pointer(fE.UnsafeAddr())).Elem()
			}
			if diff := assertions.ShouldResembleProto(fA.Interface(), fE.Interface()); diff != "" {
				addWithIndent(p.diffBuf, "field ."+fullPath+" differs:\n", poorifyIfConveyJSON(diff))
			}
			// Reset proto field to nil.
			zeroOutValue(fA)
			zeroOutValue(fE)
		case fieldKind == reflect.Struct:
			p := &protoFieldsComparator{
				actual:       fA,
				expected:     fE,
				diffBuf:      p.diffBuf,
				parentFields: append(p.parentFields, field.Name),
			}
			p.compareRecursiveAndNilify()
		case fieldKind == reflect.Slice && field.Type.Elem().Kind() == reflect.Struct:
			if fA.Len() != fE.Len() {
				addWithIndent(p.diffBuf, "field ."+fullPath+" differs in length:\n", fmt.Sprintf("expected %d, got %d", fE.Len(), fA.Len()))
				// the element may contain proto fields inside.
				zeroOutValue(fA)
				zeroOutValue(fE)
			} else {
				for i := 0; i < fA.Len(); i++ {
					p := &protoFieldsComparator{
						actual:       fA.Index(i),
						expected:     fE.Index(i),
						diffBuf:      p.diffBuf,
						parentFields: append(p.parentFields, fmt.Sprintf("%s[%d]", field.Name, i)),
					}
					p.compareRecursiveAndNilify()
				}
			}
		default:
			// In practice, this can be a ptr to a struct with a proto inside.
			// Detecting and bailing in such a case is left as future work if it
			// becomes really necessary.
		}
	}
}

func zeroOutValue(val reflect.Value) {
	valType := val.Type()
	if !val.CanSet() {
		// HACK to workaround setting private fields.
		val = reflect.NewAt(val.Type(), unsafe.Pointer(val.UnsafeAddr())).Elem()
	}
	val.Set(reflect.New(valType).Elem())
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
//
//	 {
//		 "Message": ...
//		 "Actual": ...
//		 "Expected": ...
//	 }
//
// If so, we want just the value of the "Message" part.
func poorifyIfConveyJSON(msg string) string {
	out := map[string]any{}
	if err := json.Unmarshal([]byte(msg), &out); err == nil {
		return out["Message"].(string)
	}
	return msg
}
