// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package descutil

import (
	"fmt"
	"reflect"

	"github.com/golang/protobuf/proto"
	pb "google.golang.org/genproto/protobuf"
)

var (
	// FileDescriptorProtoPackageTag is the number of package field
	// in FileDescriptorProto message.
	FileDescriptorProtoPackageTag int
	// FileDescriptorProtoMessageTag is the number of message field
	// in FileDescriptorProto message.
	FileDescriptorProtoMessageTag int
	// FileDescriptorProtoEnumTag is the number of enum field
	// in FileDescriptorProto message.
	FileDescriptorProtoEnumTag int
	// FileDescriptorProtoServiceTag is the number of service field
	// in FileDescriptorProto message.
	FileDescriptorProtoServiceTag int

	// ServiceDescriptorProtoMethodTag is the number of method field
	// in ServiceDescriptorProto message.
	ServiceDescriptorProtoMethodTag int

	// DescriptorProtoFieldTag is the number of field field
	// in DescriptorProto message.
	DescriptorProtoFieldTag int
	// DescriptorProtoNestedTypeTag is the number of nested_type field
	// in DescriptorProto message.
	DescriptorProtoNestedTypeTag int
	// DescriptorProtoEnumTypeTag is the number of enum_type field
	// in DescriptorProto message.
	DescriptorProtoEnumTypeTag int
	// DescriptorProtoOneOfTag is the number of oneof_decl field
	// in DescriptorProto message.
	DescriptorProtoOneOfTag int

	// EnumDescriptorProtoValueTag is the number of value field
	// in EnumDescriptorProto message.
	EnumDescriptorProtoValueTag int
)

func init() {
	resolveTagsFor(&pb.FileDescriptorProto{}, func(resolve func(string) int) {
		FileDescriptorProtoPackageTag = resolve("Package")
		FileDescriptorProtoMessageTag = resolve("MessageType")
		FileDescriptorProtoEnumTag = resolve("EnumType")
		FileDescriptorProtoServiceTag = resolve("Service")
	})

	resolveTagsFor(&pb.ServiceDescriptorProto{}, func(resolve func(string) int) {
		ServiceDescriptorProtoMethodTag = resolve("Method")
	})

	resolveTagsFor(&pb.DescriptorProto{}, func(resolve func(string) int) {
		DescriptorProtoFieldTag = resolve("Field")
		DescriptorProtoNestedTypeTag = resolve("NestedType")
		DescriptorProtoEnumTypeTag = resolve("EnumType")
		DescriptorProtoOneOfTag = resolve("OneofDecl")
	})

	resolveTagsFor(&pb.EnumDescriptorProto{}, func(resolve func(string) int) {
		EnumDescriptorProtoValueTag = resolve("Value")
	})
}

func resolveTagsFor(msg proto.Message, fn func(func(string) int)) {
	t := reflect.TypeOf(msg).Elem()
	if t.Kind() != reflect.Struct {
		panic(fmt.Errorf("not a struct: %T", msg))
	}

	fn(func(fieldName string) int {
		f, ok := t.FieldByName(fieldName)
		if !ok {
			panic(fmt.Errorf("struct %T has no field named %q", msg, fieldName))
		}

		var p proto.Properties
		p.Parse(f.Tag.Get("protobuf"))
		return p.Tag
	})
}
