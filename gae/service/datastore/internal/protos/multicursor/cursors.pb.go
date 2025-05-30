// Copyright 2023 The LUCI Authors.
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
// source: go.chromium.org/luci/gae/service/datastore/internal/protos/multicursor/cursors.proto

package multicursor

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

// Cursors is a structure containing zero or more cursors. This is used for
// embedding multiple cursors in a single blob/string.
type Cursors struct {
	state   protoimpl.MessageState `protogen:"open.v1"`
	Version uint64                 `protobuf:"varint,1,opt,name=version,proto3" json:"version,omitempty"` // currently 0
	Cursors []string               `protobuf:"bytes,2,rep,name=cursors,proto3" json:"cursors,omitempty"`  // list of cursors
	// magic_number exists to make it easier to determine if this is a
	// multicursor. There is a small but real chance that a base64 cursor
	// representation does represent a valid multicursor. This should
	// increase our chances in such a situation. The value is always 0xA455
	MagicNumber   int64 `protobuf:"varint,3,opt,name=magic_number,json=magicNumber,proto3" json:"magic_number,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Cursors) Reset() {
	*x = Cursors{}
	mi := &file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Cursors) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Cursors) ProtoMessage() {}

func (x *Cursors) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Cursors.ProtoReflect.Descriptor instead.
func (*Cursors) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_rawDescGZIP(), []int{0}
}

func (x *Cursors) GetVersion() uint64 {
	if x != nil {
		return x.Version
	}
	return 0
}

func (x *Cursors) GetCursors() []string {
	if x != nil {
		return x.Cursors
	}
	return nil
}

func (x *Cursors) GetMagicNumber() int64 {
	if x != nil {
		return x.MagicNumber
	}
	return 0
}

var File_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_rawDesc = string([]byte{
	0x0a, 0x54, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x67, 0x61, 0x65, 0x2f, 0x73, 0x65, 0x72, 0x76, 0x69,
	0x63, 0x65, 0x2f, 0x64, 0x61, 0x74, 0x61, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x2f, 0x69, 0x6e, 0x74,
	0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x73, 0x2f, 0x6d, 0x75, 0x6c,
	0x74, 0x69, 0x63, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x2f, 0x63, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x73,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0b, 0x6d, 0x75, 0x6c, 0x74, 0x69, 0x63, 0x75, 0x72,
	0x73, 0x6f, 0x72, 0x22, 0x60, 0x0a, 0x07, 0x43, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x73, 0x12, 0x18,
	0x0a, 0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x01, 0x20, 0x01, 0x28, 0x04, 0x52,
	0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x12, 0x18, 0x0a, 0x07, 0x63, 0x75, 0x72, 0x73,
	0x6f, 0x72, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28, 0x09, 0x52, 0x07, 0x63, 0x75, 0x72, 0x73, 0x6f,
	0x72, 0x73, 0x12, 0x21, 0x0a, 0x0c, 0x6d, 0x61, 0x67, 0x69, 0x63, 0x5f, 0x6e, 0x75, 0x6d, 0x62,
	0x65, 0x72, 0x18, 0x03, 0x20, 0x01, 0x28, 0x03, 0x52, 0x0b, 0x6d, 0x61, 0x67, 0x69, 0x63, 0x4e,
	0x75, 0x6d, 0x62, 0x65, 0x72, 0x42, 0x48, 0x5a, 0x46, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f,
	0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x67, 0x61,
	0x65, 0x2f, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x2f, 0x64, 0x61, 0x74, 0x61, 0x73, 0x74,
	0x6f, 0x72, 0x65, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x73, 0x2f, 0x6d, 0x75, 0x6c, 0x74, 0x69, 0x63, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x62,
	0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_rawDescData []byte
)

func file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_rawDesc), len(file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_rawDescData
}

var file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_goTypes = []any{
	(*Cursors)(nil), // 0: multicursor.Cursors
}
var file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() {
	file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_init()
}
func file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_init() {
	if File_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_rawDesc), len(file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto = out.File
	file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_goTypes = nil
	file_go_chromium_org_luci_gae_service_datastore_internal_protos_multicursor_cursors_proto_depIdxs = nil
}
