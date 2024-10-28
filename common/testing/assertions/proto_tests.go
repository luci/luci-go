// Copyright 2018 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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

	protoLegacy "github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

// ShouldResembleProto determines if two values are deeply equal, using
// the following rules:
//   - proto equality is defined by proto.Equal,
//   - if a type has an .Equal method, equality is defined by
//     that method,
//   - in the absence of the above, deep equality is defined
//     by recursing over structs, maps and slices in the usual way.
//
// See github.com/google/go-cmp/cmp#Equal for more details.
//
// This method is similar to goconvey's ShouldResemble, except that
// supports proto messages at the top-level or nested inside
// other types.
func ShouldResembleProto(actual any, expected ...any) string {
	if len(expected) != 1 {
		return fmt.Sprintf("ShouldResembleProto expects 1 value, got %d", len(expected))
	}
	exp := expected[0]
	// Compare all unexported fields.
	exportAll := cmp.Exporter(func(t reflect.Type) bool {
		return true
	})
	diff := cmp.Diff(exp, actual, protocmp.Transform(), exportAll)
	if diff != "" {
		return fmt.Sprintf("Unexpected difference (-want +got):\n%s", diff)
	}
	return "" // Success
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
