// Copyright 2019 The LUCI Authors.
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

package main

import (
	"fmt"

	"go.chromium.org/luci/common/proto/playground"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/runtime/protoimpl"
)

func main() {
	innerMsg1 := &playground.InnerMessage{
		IntField:  123,
		BoolField: true,
	}
	innerMsg2 := &playground.InnerMessage{
		IntField:  456,
		BoolField: true,
	}
	msg := &playground.TestMessage{
		MapField: map[string]*playground.InnerMessage{
			"a": innerMsg1,
			"b": innerMsg2,
		},
	}
	msgDescriptor := protoimpl.X.MessageDescriptorOf(msg)
	fmt.Printf("Message name is %s\n", msgDescriptor.Name())
	fieldDesc := msgDescriptor.Fields().ByName(protoreflect.Name("map_field"))

	// for i := 0; i < fieldDescriptors.Len(); i++ {
	// 	fmt.Printf("Field %d; Name %s\n", i, fieldDescriptors.Get(i).Name())
	// }

	fmt.Printf("%s", fieldDesc.Cardinality())
	// reflectMsg := protoimpl.X.MessageOf(msg)

	// fmt.Printf("%+v", reflectMsg.Get(fieldDesc).Map().)

	// for _, prop := range proto.GetProperties(reflect.ValueOf(*msg).Type()).Prop {
	//      fmt.Printf("Property: %+v\n", *prop)
	// }

	// for _, oneOfType := range proto.GetProperties(reflect.ValueOf(*msg).Type()).OneofTypes {
	//      fmt.Printf("One of Type: %+v\n", *oneOfType)
	// }

	// marshaler := &jsonpb.Marshaler{
	// 	Indent: "  ",
	// }
	// fmt.Println(marshaler.MarshalToString(descriptor))

}
