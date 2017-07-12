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

package descutil

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/golang/protobuf/proto"
	pb "github.com/golang/protobuf/protoc-gen-go/descriptor"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTagsMatchProto(t *testing.T) {
	t.Parallel()

	propertiesForField := func(props *proto.StructProperties, name string) *proto.Properties {
		for _, p := range props.Prop {
			if p.Name == name {
				return p
			}
		}
		panic(fmt.Errorf("no property for %q", name))
	}

	Convey(`Hard-coded tag constants match protobuf.`, t, func() {
		Convey(`For FileDescriptorProto`, func() {
			props := proto.GetProperties(reflect.TypeOf(pb.FileDescriptorProto{}))

			So(propertiesForField(props, "Package").Tag, ShouldEqual, FileDescriptorProtoPackageTag)
			So(propertiesForField(props, "MessageType").Tag, ShouldEqual, FileDescriptorProtoMessageTag)
			So(propertiesForField(props, "EnumType").Tag, ShouldEqual, FileDescriptorProtoEnumTag)
			So(propertiesForField(props, "Service").Tag, ShouldEqual, FileDescriptorProtoServiceTag)
		})

		Convey(`For ServiceDescriptorProto`, func() {
			props := proto.GetProperties(reflect.TypeOf(pb.ServiceDescriptorProto{}))

			So(propertiesForField(props, "Method").Tag, ShouldEqual, ServiceDescriptorProtoMethodTag)
		})

		Convey(`For DescriptorProto`, func() {
			props := proto.GetProperties(reflect.TypeOf(pb.DescriptorProto{}))

			So(propertiesForField(props, "Field").Tag, ShouldEqual, DescriptorProtoFieldTag)
			So(propertiesForField(props, "NestedType").Tag, ShouldEqual, DescriptorProtoNestedTypeTag)
			So(propertiesForField(props, "EnumType").Tag, ShouldEqual, DescriptorProtoEnumTypeTag)
			So(propertiesForField(props, "OneofDecl").Tag, ShouldEqual, DescriptorProtoOneOfTag)
		})

		Convey(`For EnumDescriptorProto`, func() {
			props := proto.GetProperties(reflect.TypeOf(pb.EnumDescriptorProto{}))

			So(propertiesForField(props, "Value").Tag, ShouldEqual, EnumDescriptorProtoValueTag)
		})
	})
}
