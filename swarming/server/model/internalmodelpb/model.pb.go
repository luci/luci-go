// Copyright 2024 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.33.0
// 	protoc        v5.26.1
// source: go.chromium.org/luci/swarming/server/model/internalmodelpb/model.proto

package internalmodelpb

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// AggregatedDimensions is stored in the datastore in a compressed form.
//
// See BotsDimensionsAggregation entity.
//
// It is a map (pool, dimension key) => [set of dimension values]. It is used
// to serve GetBotDimensions RPC. Updated by scan.BotsDimensionsAggregator.
//
// All entries are sorted.
type AggregatedDimensions struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Pools []*AggregatedDimensions_Pool `protobuf:"bytes,1,rep,name=pools,proto3" json:"pools,omitempty"`
}

func (x *AggregatedDimensions) Reset() {
	*x = AggregatedDimensions{}
	if protoimpl.UnsafeEnabled {
		mi := &file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *AggregatedDimensions) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AggregatedDimensions) ProtoMessage() {}

func (x *AggregatedDimensions) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AggregatedDimensions.ProtoReflect.Descriptor instead.
func (*AggregatedDimensions) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_rawDescGZIP(), []int{0}
}

func (x *AggregatedDimensions) GetPools() []*AggregatedDimensions_Pool {
	if x != nil {
		return x.Pools
	}
	return nil
}

type AggregatedDimensions_Pool struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Pool       string                                 `protobuf:"bytes,1,opt,name=pool,proto3" json:"pool,omitempty"`
	Dimensions []*AggregatedDimensions_Pool_Dimension `protobuf:"bytes,2,rep,name=dimensions,proto3" json:"dimensions,omitempty"`
}

func (x *AggregatedDimensions_Pool) Reset() {
	*x = AggregatedDimensions_Pool{}
	if protoimpl.UnsafeEnabled {
		mi := &file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *AggregatedDimensions_Pool) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AggregatedDimensions_Pool) ProtoMessage() {}

func (x *AggregatedDimensions_Pool) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AggregatedDimensions_Pool.ProtoReflect.Descriptor instead.
func (*AggregatedDimensions_Pool) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_rawDescGZIP(), []int{0, 0}
}

func (x *AggregatedDimensions_Pool) GetPool() string {
	if x != nil {
		return x.Pool
	}
	return ""
}

func (x *AggregatedDimensions_Pool) GetDimensions() []*AggregatedDimensions_Pool_Dimension {
	if x != nil {
		return x.Dimensions
	}
	return nil
}

type AggregatedDimensions_Pool_Dimension struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name   string   `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Values []string `protobuf:"bytes,2,rep,name=values,proto3" json:"values,omitempty"`
}

func (x *AggregatedDimensions_Pool_Dimension) Reset() {
	*x = AggregatedDimensions_Pool_Dimension{}
	if protoimpl.UnsafeEnabled {
		mi := &file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *AggregatedDimensions_Pool_Dimension) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AggregatedDimensions_Pool_Dimension) ProtoMessage() {}

func (x *AggregatedDimensions_Pool_Dimension) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AggregatedDimensions_Pool_Dimension.ProtoReflect.Descriptor instead.
func (*AggregatedDimensions_Pool_Dimension) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_rawDescGZIP(), []int{0, 0, 0}
}

func (x *AggregatedDimensions_Pool_Dimension) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *AggregatedDimensions_Pool_Dimension) GetValues() []string {
	if x != nil {
		return x.Values
	}
	return nil
}

var File_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_rawDesc = []byte{
	0x0a, 0x46, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x73, 0x77, 0x61, 0x72, 0x6d, 0x69, 0x6e, 0x67, 0x2f,
	0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2f, 0x6d, 0x6f, 0x64, 0x65, 0x6c, 0x2f, 0x69, 0x6e, 0x74,
	0x65, 0x72, 0x6e, 0x61, 0x6c, 0x6d, 0x6f, 0x64, 0x65, 0x6c, 0x70, 0x62, 0x2f, 0x6d, 0x6f, 0x64,
	0x65, 0x6c, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x18, 0x73, 0x77, 0x61, 0x72, 0x6d, 0x69,
	0x6e, 0x67, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x73, 0x2e, 0x6d, 0x6f, 0x64,
	0x65, 0x6c, 0x22, 0x96, 0x02, 0x0a, 0x14, 0x41, 0x67, 0x67, 0x72, 0x65, 0x67, 0x61, 0x74, 0x65,
	0x64, 0x44, 0x69, 0x6d, 0x65, 0x6e, 0x73, 0x69, 0x6f, 0x6e, 0x73, 0x12, 0x49, 0x0a, 0x05, 0x70,
	0x6f, 0x6f, 0x6c, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x33, 0x2e, 0x73, 0x77, 0x61,
	0x72, 0x6d, 0x69, 0x6e, 0x67, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x73, 0x2e,
	0x6d, 0x6f, 0x64, 0x65, 0x6c, 0x2e, 0x41, 0x67, 0x67, 0x72, 0x65, 0x67, 0x61, 0x74, 0x65, 0x64,
	0x44, 0x69, 0x6d, 0x65, 0x6e, 0x73, 0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x50, 0x6f, 0x6f, 0x6c, 0x52,
	0x05, 0x70, 0x6f, 0x6f, 0x6c, 0x73, 0x1a, 0xb2, 0x01, 0x0a, 0x04, 0x50, 0x6f, 0x6f, 0x6c, 0x12,
	0x12, 0x0a, 0x04, 0x70, 0x6f, 0x6f, 0x6c, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x70,
	0x6f, 0x6f, 0x6c, 0x12, 0x5d, 0x0a, 0x0a, 0x64, 0x69, 0x6d, 0x65, 0x6e, 0x73, 0x69, 0x6f, 0x6e,
	0x73, 0x18, 0x02, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x3d, 0x2e, 0x73, 0x77, 0x61, 0x72, 0x6d, 0x69,
	0x6e, 0x67, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x73, 0x2e, 0x6d, 0x6f, 0x64,
	0x65, 0x6c, 0x2e, 0x41, 0x67, 0x67, 0x72, 0x65, 0x67, 0x61, 0x74, 0x65, 0x64, 0x44, 0x69, 0x6d,
	0x65, 0x6e, 0x73, 0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x50, 0x6f, 0x6f, 0x6c, 0x2e, 0x44, 0x69, 0x6d,
	0x65, 0x6e, 0x73, 0x69, 0x6f, 0x6e, 0x52, 0x0a, 0x64, 0x69, 0x6d, 0x65, 0x6e, 0x73, 0x69, 0x6f,
	0x6e, 0x73, 0x1a, 0x37, 0x0a, 0x09, 0x44, 0x69, 0x6d, 0x65, 0x6e, 0x73, 0x69, 0x6f, 0x6e, 0x12,
	0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e,
	0x61, 0x6d, 0x65, 0x12, 0x16, 0x0a, 0x06, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x73, 0x18, 0x02, 0x20,
	0x03, 0x28, 0x09, 0x52, 0x06, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x73, 0x42, 0x3c, 0x5a, 0x3a, 0x67,
	0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c,
	0x75, 0x63, 0x69, 0x2f, 0x73, 0x77, 0x61, 0x72, 0x6d, 0x69, 0x6e, 0x67, 0x2f, 0x73, 0x65, 0x72,
	0x76, 0x65, 0x72, 0x2f, 0x6d, 0x6f, 0x64, 0x65, 0x6c, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e,
	0x61, 0x6c, 0x6d, 0x6f, 0x64, 0x65, 0x6c, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x33,
}

var (
	file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_rawDescData = file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_rawDesc
)

func file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_rawDescData = protoimpl.X.CompressGZIP(file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_rawDescData)
	})
	return file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_rawDescData
}

var file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_goTypes = []interface{}{
	(*AggregatedDimensions)(nil),                // 0: swarming.internals.model.AggregatedDimensions
	(*AggregatedDimensions_Pool)(nil),           // 1: swarming.internals.model.AggregatedDimensions.Pool
	(*AggregatedDimensions_Pool_Dimension)(nil), // 2: swarming.internals.model.AggregatedDimensions.Pool.Dimension
}
var file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_depIdxs = []int32{
	1, // 0: swarming.internals.model.AggregatedDimensions.pools:type_name -> swarming.internals.model.AggregatedDimensions.Pool
	2, // 1: swarming.internals.model.AggregatedDimensions.Pool.dimensions:type_name -> swarming.internals.model.AggregatedDimensions.Pool.Dimension
	2, // [2:2] is the sub-list for method output_type
	2, // [2:2] is the sub-list for method input_type
	2, // [2:2] is the sub-list for extension type_name
	2, // [2:2] is the sub-list for extension extendee
	0, // [0:2] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_init() }
func file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_init() {
	if File_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*AggregatedDimensions); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*AggregatedDimensions_Pool); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*AggregatedDimensions_Pool_Dimension); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto = out.File
	file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_rawDesc = nil
	file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_goTypes = nil
	file_go_chromium_org_luci_swarming_server_model_internalmodelpb_model_proto_depIdxs = nil
}