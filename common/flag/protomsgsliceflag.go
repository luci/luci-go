// Copyright 2020 The LUCI Authors.
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

package flag

import (
	"flag"
	"fmt"
	"reflect"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var protoMsgBaseType = reflect.TypeOf((*proto.Message)(nil)).Elem()

type messageSliceFlag struct {
	// msgSliceVal is a slice of pointers to concrete proto message structs.
	msgSliceVal reflect.Value
	// refMsg is a zero value concrete message struct instance that is used as
	// reference to create new instance for deserialization purposes.
	refMsg proto.Message
}

// MessageSliceFlag returns a new flag for a slice of pointer to concrete proto
// message struct which implements the proto.Message interface. Expect input to
// be of type *[]*B where B is concrete struct of proto message like pb.Build.
// A flag value must be a JSON string of the provided concrete proto message.
func MessageSliceFlag(msgSlicePtr any) flag.Getter {
	slicePtrVal := reflect.ValueOf(msgSlicePtr)
	assertKind(slicePtrVal.Type(), reflect.Ptr)

	sliceType := slicePtrVal.Elem().Type()
	assertKind(sliceType, reflect.Slice)

	sliceElemType := sliceType.Elem()
	if !sliceElemType.Implements(protoMsgBaseType) {
		panic(fmt.Sprintf("Expect elements of the slice to implement interface: %s, however, got type: %s", protoMsgBaseType, sliceElemType))
	}
	// type that implement proto.message should be of pointer.
	assertKind(sliceElemType, reflect.Ptr)

	protoMsgStructType := sliceElemType.Elem()
	assertKind(protoMsgStructType, reflect.Struct)

	// reflect new return ptr to the message struct which implements the
	// proto.Message interface
	refMsg := reflect.New(protoMsgStructType).Interface().(proto.Message)

	return &messageSliceFlag{
		msgSliceVal: slicePtrVal.Elem(),
		refMsg:      refMsg,
	}
}

// String returns all messages serailzed in JSON and separated by a new line.
// Empty string will be returned if flag is a zero value.
func (m messageSliceFlag) String() string {
	if !m.msgSliceVal.IsValid() {
		return ""
	}
	var sb strings.Builder
	for i := range m.msgSliceVal.Len() {
		msg := m.msgSliceVal.Index(i).Interface().(proto.Message)

		if buf, err := protojson.Marshal(msg); err != nil {
			panic(fmt.Errorf("failed to marshal a message: %s", err))
		} else {
			sb.Write(buf)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// Set deserializes the JSON datagram of proto message and appends it
// to the slice.
func (m *messageSliceFlag) Set(val string) error {
	newMsg := proto.Clone(m.refMsg)
	if err := protojson.Unmarshal([]byte(val), newMsg); err != nil {
		return err
	}
	m.msgSliceVal.Set(reflect.Append(m.msgSliceVal, reflect.ValueOf(newMsg)))
	return nil
}

// Get retrieves the raw flag value.
func (m *messageSliceFlag) Get() any {
	return m.msgSliceVal.Interface()
}

// assertKind panics when the given actual type does not match the expected
// kind.
func assertKind(actual reflect.Type, expected reflect.Kind) {
	if actual.Kind() != expected {
		panic(fmt.Sprintf("Expect kind: %s, however, got type: %s of kind %s",
			expected, actual, actual.Kind()))
	}
}
