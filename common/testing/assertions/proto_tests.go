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

package assertions

import (
	"fmt"
	"reflect"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	protoLegacy "github.com/golang/protobuf/proto"

	"github.com/smarty/assertions"
)

// ShouldResembleProto asserts that given two values that contain proto messages
// are equal by comparing their types and ensuring they serialize to the same
// text representation.
//
// Values can either each be a proto.Message or a slice of values that each
// implement proto.Message interface.
func ShouldResembleProto(actual any, expected ...any) string {
	if len(expected) != 1 {
		return fmt.Sprintf("ShouldResembleProto expects 1 value, got %d", len(expected))
	}
	exp := expected[0]

	// This is very crude... We want to be able to see a diff between expected
	// and actual protos, so we just serialize them into a string and compare
	// strings. This is much simpler than trying to achieve the same using
	// reflection, clearing of XXX_*** fields and ShouldResemble.

	if am, ok := protoMessage(actual); ok {
		if err := assertions.ShouldHaveSameTypeAs(actual, exp); err != "" {
			return err
		}
		em, _ := protoMessage(exp)
		return assertions.ShouldEqual(textPBMultiline.Format(am), textPBMultiline.Format(em))
	}

	lVal := reflect.ValueOf(actual)
	rVal := reflect.ValueOf(exp)
	if lVal.Kind() == reflect.Slice {
		if rVal.Kind() != reflect.Slice {
			return "ShouldResembleProto is expecting both arguments to be a slice if first one is a slice"
		}
		if err := assertions.ShouldHaveLength(actual, rVal.Len()); err != "" {
			return err
		}

		var left, right strings.Builder

		for i := 0; i < lVal.Len(); i++ {
			l := lVal.Index(i).Interface()
			r := rVal.Index(i).Interface()
			if err := assertions.ShouldHaveSameTypeAs(l, r); err != "" {
				return err
			}
			if i != 0 {
				left.WriteString("---\n")
				right.WriteString("---\n")
			}
			lm, _ := protoMessage(l)
			rm, _ := protoMessage(r)
			left.WriteString(textPBMultiline.Format(lm))
			right.WriteString(textPBMultiline.Format(rm))
		}
		return assertions.ShouldEqual(left.String(), right.String())
	}

	return fmt.Sprintf(
		"ShouldResembleProto doesn't know how to handle values of type %T, "+
			"expecting a proto.Message or a slice of thereof", actual)
}

// ShouldResembleProtoText is like ShouldResembleProto, but expected
// is protobuf text.
// actual must be a message. A slice of messages is not supported.
func ShouldResembleProtoText(actual any, expected ...any) string {
	return shouldResembleProtoUnmarshal(
		func(s string, m proto.Message) error {
			return prototext.Unmarshal([]byte(s), m)
		},
		actual,
		expected...)
}

// ShouldResembleProtoJSON is like ShouldResembleProto, but expected
// is protobuf text.
// actual must be a message. A slice of messages is not supported.
func ShouldResembleProtoJSON(actual any, expected ...any) string {
	return shouldResembleProtoUnmarshal(
		func(s string, m proto.Message) error {
			return protojson.Unmarshal([]byte(s), m)
		},
		actual,
		expected...)
}

func shouldResembleProtoUnmarshal(unmarshal func(string, proto.Message) error, actual any, expected ...any) string {
	if _, ok := protoMessage(actual); !ok {
		return fmt.Sprintf("ShouldResembleProtoText expects a proto message, got %T", actual)
	}

	if len(expected) != 1 {
		return fmt.Sprintf("ShouldResembleProtoText expects 1 value, got %d", len(expected))
	}
	expText, ok := expected[0].(string)
	if !ok {
		return fmt.Sprintf("ShouldResembleProtoText expects a string value, got %T", expected[0])
	}

	expMsg := reflect.New(reflect.TypeOf(actual).Elem()).Interface().(proto.Message)

	if err := unmarshal(expText, expMsg); err != nil {
		return err.Error()
	}
	return ShouldResembleProto(actual, expMsg)
}

var textPBMultiline = prototext.MarshalOptions{
	Multiline: true,
}

// protoMessage returns V2 proto message, converting v1 on the fly.
func protoMessage(a any) (proto.Message, bool) {
	if m, ok := a.(proto.Message); ok {
		return m, true
	}
	if m, ok := a.(protoLegacy.Message); ok {
		return protoLegacy.MessageV2(m), true
	}
	return nil, false
}
