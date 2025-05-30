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

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.5
// 	protoc        v6.30.2
// source: go.chromium.org/luci/lucictx/lucictx_test.proto

package lucictx

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type TestStructure struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	HelloThere    int32                  `protobuf:"varint,1,opt,name=hello_there,json=helloThere,proto3" json:"hello_there,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TestStructure) Reset() {
	*x = TestStructure{}
	mi := &file_go_chromium_org_luci_lucictx_lucictx_test_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TestStructure) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TestStructure) ProtoMessage() {}

func (x *TestStructure) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_lucictx_lucictx_test_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TestStructure.ProtoReflect.Descriptor instead.
func (*TestStructure) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_lucictx_lucictx_test_proto_rawDescGZIP(), []int{0}
}

func (x *TestStructure) GetHelloThere() int32 {
	if x != nil {
		return x.HelloThere
	}
	return 0
}

var File_go_chromium_org_luci_lucictx_lucictx_test_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_lucictx_lucictx_test_proto_rawDesc = string([]byte{
	0x0a, 0x2f, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x63, 0x74, 0x78, 0x2f, 0x6c,
	0x75, 0x63, 0x69, 0x63, 0x74, 0x78, 0x5f, 0x74, 0x65, 0x73, 0x74, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x12, 0x07, 0x6c, 0x75, 0x63, 0x69, 0x63, 0x74, 0x78, 0x22, 0x30, 0x0a, 0x0d, 0x54, 0x65,
	0x73, 0x74, 0x53, 0x74, 0x72, 0x75, 0x63, 0x74, 0x75, 0x72, 0x65, 0x12, 0x1f, 0x0a, 0x0b, 0x68,
	0x65, 0x6c, 0x6c, 0x6f, 0x5f, 0x74, 0x68, 0x65, 0x72, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x05,
	0x52, 0x0a, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x54, 0x68, 0x65, 0x72, 0x65, 0x42, 0x26, 0x5a, 0x24,
	0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f,
	0x6c, 0x75, 0x63, 0x69, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x63, 0x74, 0x78, 0x3b, 0x6c, 0x75, 0x63,
	0x69, 0x63, 0x74, 0x78, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_lucictx_lucictx_test_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_lucictx_lucictx_test_proto_rawDescData []byte
)

func file_go_chromium_org_luci_lucictx_lucictx_test_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_lucictx_lucictx_test_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_lucictx_lucictx_test_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_lucictx_lucictx_test_proto_rawDesc), len(file_go_chromium_org_luci_lucictx_lucictx_test_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_lucictx_lucictx_test_proto_rawDescData
}

var file_go_chromium_org_luci_lucictx_lucictx_test_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_go_chromium_org_luci_lucictx_lucictx_test_proto_goTypes = []any{
	(*TestStructure)(nil), // 0: lucictx.TestStructure
}
var file_go_chromium_org_luci_lucictx_lucictx_test_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_lucictx_lucictx_test_proto_init() }
func file_go_chromium_org_luci_lucictx_lucictx_test_proto_init() {
	if File_go_chromium_org_luci_lucictx_lucictx_test_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_lucictx_lucictx_test_proto_rawDesc), len(file_go_chromium_org_luci_lucictx_lucictx_test_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_lucictx_lucictx_test_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_lucictx_lucictx_test_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_lucictx_lucictx_test_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_lucictx_lucictx_test_proto = out.File
	file_go_chromium_org_luci_lucictx_lucictx_test_proto_goTypes = nil
	file_go_chromium_org_luci_lucictx_lucictx_test_proto_depIdxs = nil
}
