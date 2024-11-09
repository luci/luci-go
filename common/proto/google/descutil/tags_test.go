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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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

	ftt.Run(`Hard-coded tag constants match protobuf.`, t, func(t *ftt.Test) {
		t.Run(`For FileDescriptorProto`, func(t *ftt.Test) {
			props := proto.GetProperties(reflect.TypeOf(pb.FileDescriptorProto{}))

			assert.Loosely(t, propertiesForField(props, "Package").Tag, should.Equal(FileDescriptorProtoPackageTag))
			assert.Loosely(t, propertiesForField(props, "MessageType").Tag, should.Equal(FileDescriptorProtoMessageTag))
			assert.Loosely(t, propertiesForField(props, "EnumType").Tag, should.Equal(FileDescriptorProtoEnumTag))
			assert.Loosely(t, propertiesForField(props, "Service").Tag, should.Equal(FileDescriptorProtoServiceTag))
		})

		t.Run(`For ServiceDescriptorProto`, func(t *ftt.Test) {
			props := proto.GetProperties(reflect.TypeOf(pb.ServiceDescriptorProto{}))

			assert.Loosely(t, propertiesForField(props, "Method").Tag, should.Equal(ServiceDescriptorProtoMethodTag))
		})

		t.Run(`For DescriptorProto`, func(t *ftt.Test) {
			props := proto.GetProperties(reflect.TypeOf(pb.DescriptorProto{}))

			assert.Loosely(t, propertiesForField(props, "Field").Tag, should.Equal(DescriptorProtoFieldTag))
			assert.Loosely(t, propertiesForField(props, "NestedType").Tag, should.Equal(DescriptorProtoNestedTypeTag))
			assert.Loosely(t, propertiesForField(props, "EnumType").Tag, should.Equal(DescriptorProtoEnumTypeTag))
			assert.Loosely(t, propertiesForField(props, "OneofDecl").Tag, should.Equal(DescriptorProtoOneOfTag))
		})

		t.Run(`For EnumDescriptorProto`, func(t *ftt.Test) {
			props := proto.GetProperties(reflect.TypeOf(pb.EnumDescriptorProto{}))

			assert.Loosely(t, propertiesForField(props, "Value").Tag, should.Equal(EnumDescriptorProtoValueTag))
		})
	})
}
