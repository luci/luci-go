// Copyright 2015 The LUCI Authors.
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

// These constnats correspond to tag values in the respective "descriptor.proto"
// message types. These constants' tag matches are asserted in the
// "TestTagsMatchProto" unit test.
const (
	// FileDescriptorProtoPackageTag is the number of package field
	// in FileDescriptorProto message.
	FileDescriptorProtoPackageTag = 2
	// FileDescriptorProtoMessageTag is the number of message field
	// in FileDescriptorProto message.
	FileDescriptorProtoMessageTag = 4
	// FileDescriptorProtoEnumTag is the number of enum field
	// in FileDescriptorProto message.
	FileDescriptorProtoEnumTag = 5
	// FileDescriptorProtoServiceTag is the number of service field
	// in FileDescriptorProto message.
	FileDescriptorProtoServiceTag = 6

	// ServiceDescriptorProtoMethodTag is the number of method field
	// in ServiceDescriptorProto message.
	ServiceDescriptorProtoMethodTag = 2

	// DescriptorProtoFieldTag is the number of field field
	// in DescriptorProto message.
	DescriptorProtoFieldTag = 2
	// DescriptorProtoNestedTypeTag is the number of nested_type field
	// in DescriptorProto message.
	DescriptorProtoNestedTypeTag = 3
	// DescriptorProtoEnumTypeTag is the number of enum_type field
	// in DescriptorProto message.
	DescriptorProtoEnumTypeTag = 4
	// DescriptorProtoOneOfTag is the number of oneof_decl field
	// in DescriptorProto message.
	DescriptorProtoOneOfTag = 8

	// EnumDescriptorProtoValueTag is the number of value field
	// in EnumDescriptorProto message.
	EnumDescriptorProtoValueTag = 2
)
