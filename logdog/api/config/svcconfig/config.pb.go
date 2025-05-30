// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.5
// 	protoc        v6.30.2
// source: go.chromium.org/luci/logdog/api/config/svcconfig/config.proto

package svcconfig

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
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

// Config is the overall instance configuration.
type Config struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Configuration for the Butler's log transport.
	Transport *Transport `protobuf:"bytes,10,opt,name=transport,proto3" json:"transport,omitempty"`
	// Coordinator is the coordinator service configuration.
	Coordinator   *Coordinator `protobuf:"bytes,20,opt,name=coordinator,proto3" json:"coordinator,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Config) Reset() {
	*x = Config{}
	mi := &file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Config) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Config) ProtoMessage() {}

func (x *Config) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Config.ProtoReflect.Descriptor instead.
func (*Config) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_rawDescGZIP(), []int{0}
}

func (x *Config) GetTransport() *Transport {
	if x != nil {
		return x.Transport
	}
	return nil
}

func (x *Config) GetCoordinator() *Coordinator {
	if x != nil {
		return x.Coordinator
	}
	return nil
}

// Coordinator is the Coordinator service configuration.
type Coordinator struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The name of the authentication group for administrators.
	AdminAuthGroup string `protobuf:"bytes,10,opt,name=admin_auth_group,json=adminAuthGroup,proto3" json:"admin_auth_group,omitempty"`
	// The name of the authentication group for backend services.
	ServiceAuthGroup string `protobuf:"bytes,11,opt,name=service_auth_group,json=serviceAuthGroup,proto3" json:"service_auth_group,omitempty"`
	// A list of origin URLs that are allowed to perform CORS RPC calls.
	RpcAllowOrigins []string `protobuf:"bytes,20,rep,name=rpc_allow_origins,json=rpcAllowOrigins,proto3" json:"rpc_allow_origins,omitempty"`
	// The maximum amount of time after a prefix has been registered when log
	// streams may also be registered under that prefix.
	//
	// After the expiration period has passed, new log stream registration will
	// fail.
	//
	// Project configurations or stream prefix regitrations may override this by
	// providing >= 0 values for prefix expiration. The smallest configured
	// expiration will be applied.
	PrefixExpiration *durationpb.Duration `protobuf:"bytes,21,opt,name=prefix_expiration,json=prefixExpiration,proto3" json:"prefix_expiration,omitempty"`
	unknownFields    protoimpl.UnknownFields
	sizeCache        protoimpl.SizeCache
}

func (x *Coordinator) Reset() {
	*x = Coordinator{}
	mi := &file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Coordinator) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Coordinator) ProtoMessage() {}

func (x *Coordinator) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Coordinator.ProtoReflect.Descriptor instead.
func (*Coordinator) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_rawDescGZIP(), []int{1}
}

func (x *Coordinator) GetAdminAuthGroup() string {
	if x != nil {
		return x.AdminAuthGroup
	}
	return ""
}

func (x *Coordinator) GetServiceAuthGroup() string {
	if x != nil {
		return x.ServiceAuthGroup
	}
	return ""
}

func (x *Coordinator) GetRpcAllowOrigins() []string {
	if x != nil {
		return x.RpcAllowOrigins
	}
	return nil
}

func (x *Coordinator) GetPrefixExpiration() *durationpb.Duration {
	if x != nil {
		return x.PrefixExpiration
	}
	return nil
}

var File_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_rawDesc = string([]byte{
	0x0a, 0x3d, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x6c, 0x6f, 0x67, 0x64, 0x6f, 0x67, 0x2f, 0x61, 0x70,
	0x69, 0x2f, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x2f, 0x73, 0x76, 0x63, 0x63, 0x6f, 0x6e, 0x66,
	0x69, 0x67, 0x2f, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12,
	0x09, 0x73, 0x76, 0x63, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x1a, 0x40, 0x67, 0x6f, 0x2e, 0x63,
	0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69,
	0x2f, 0x6c, 0x6f, 0x67, 0x64, 0x6f, 0x67, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x63, 0x6f, 0x6e, 0x66,
	0x69, 0x67, 0x2f, 0x73, 0x76, 0x63, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x2f, 0x74, 0x72, 0x61,
	0x6e, 0x73, 0x70, 0x6f, 0x72, 0x74, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1e, 0x67, 0x6f,
	0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x64, 0x75,
	0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0xa7, 0x01, 0x0a,
	0x06, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x12, 0x32, 0x0a, 0x09, 0x74, 0x72, 0x61, 0x6e, 0x73,
	0x70, 0x6f, 0x72, 0x74, 0x18, 0x0a, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x14, 0x2e, 0x73, 0x76, 0x63,
	0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x2e, 0x54, 0x72, 0x61, 0x6e, 0x73, 0x70, 0x6f, 0x72, 0x74,
	0x52, 0x09, 0x74, 0x72, 0x61, 0x6e, 0x73, 0x70, 0x6f, 0x72, 0x74, 0x12, 0x38, 0x0a, 0x0b, 0x63,
	0x6f, 0x6f, 0x72, 0x64, 0x69, 0x6e, 0x61, 0x74, 0x6f, 0x72, 0x18, 0x14, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x16, 0x2e, 0x73, 0x76, 0x63, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x2e, 0x43, 0x6f, 0x6f,
	0x72, 0x64, 0x69, 0x6e, 0x61, 0x74, 0x6f, 0x72, 0x52, 0x0b, 0x63, 0x6f, 0x6f, 0x72, 0x64, 0x69,
	0x6e, 0x61, 0x74, 0x6f, 0x72, 0x4a, 0x04, 0x08, 0x0b, 0x10, 0x0c, 0x4a, 0x04, 0x08, 0x15, 0x10,
	0x16, 0x4a, 0x04, 0x08, 0x16, 0x10, 0x17, 0x52, 0x07, 0x73, 0x74, 0x6f, 0x72, 0x61, 0x67, 0x65,
	0x52, 0x09, 0x63, 0x6f, 0x6c, 0x6c, 0x65, 0x63, 0x74, 0x6f, 0x72, 0x52, 0x09, 0x61, 0x72, 0x63,
	0x68, 0x69, 0x76, 0x69, 0x73, 0x74, 0x22, 0xa3, 0x02, 0x0a, 0x0b, 0x43, 0x6f, 0x6f, 0x72, 0x64,
	0x69, 0x6e, 0x61, 0x74, 0x6f, 0x72, 0x12, 0x28, 0x0a, 0x10, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x5f,
	0x61, 0x75, 0x74, 0x68, 0x5f, 0x67, 0x72, 0x6f, 0x75, 0x70, 0x18, 0x0a, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x0e, 0x61, 0x64, 0x6d, 0x69, 0x6e, 0x41, 0x75, 0x74, 0x68, 0x47, 0x72, 0x6f, 0x75, 0x70,
	0x12, 0x2c, 0x0a, 0x12, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x5f, 0x61, 0x75, 0x74, 0x68,
	0x5f, 0x67, 0x72, 0x6f, 0x75, 0x70, 0x18, 0x0b, 0x20, 0x01, 0x28, 0x09, 0x52, 0x10, 0x73, 0x65,
	0x72, 0x76, 0x69, 0x63, 0x65, 0x41, 0x75, 0x74, 0x68, 0x47, 0x72, 0x6f, 0x75, 0x70, 0x12, 0x2a,
	0x0a, 0x11, 0x72, 0x70, 0x63, 0x5f, 0x61, 0x6c, 0x6c, 0x6f, 0x77, 0x5f, 0x6f, 0x72, 0x69, 0x67,
	0x69, 0x6e, 0x73, 0x18, 0x14, 0x20, 0x03, 0x28, 0x09, 0x52, 0x0f, 0x72, 0x70, 0x63, 0x41, 0x6c,
	0x6c, 0x6f, 0x77, 0x4f, 0x72, 0x69, 0x67, 0x69, 0x6e, 0x73, 0x12, 0x46, 0x0a, 0x11, 0x70, 0x72,
	0x65, 0x66, 0x69, 0x78, 0x5f, 0x65, 0x78, 0x70, 0x69, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x18,
	0x15, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e,
	0x52, 0x10, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x45, 0x78, 0x70, 0x69, 0x72, 0x61, 0x74, 0x69,
	0x6f, 0x6e, 0x4a, 0x04, 0x08, 0x1e, 0x10, 0x1f, 0x4a, 0x04, 0x08, 0x1f, 0x10, 0x20, 0x4a, 0x04,
	0x08, 0x20, 0x10, 0x21, 0x52, 0x0d, 0x61, 0x72, 0x63, 0x68, 0x69, 0x76, 0x65, 0x5f, 0x74, 0x6f,
	0x70, 0x69, 0x63, 0x52, 0x14, 0x61, 0x72, 0x63, 0x68, 0x69, 0x76, 0x65, 0x5f, 0x73, 0x65, 0x74,
	0x74, 0x6c, 0x65, 0x5f, 0x64, 0x65, 0x6c, 0x61, 0x79, 0x52, 0x11, 0x61, 0x72, 0x63, 0x68, 0x69,
	0x76, 0x65, 0x5f, 0x64, 0x65, 0x6c, 0x61, 0x79, 0x5f, 0x6d, 0x61, 0x78, 0x42, 0x32, 0x5a, 0x30,
	0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f,
	0x6c, 0x75, 0x63, 0x69, 0x2f, 0x6c, 0x6f, 0x67, 0x64, 0x6f, 0x67, 0x2f, 0x61, 0x70, 0x69, 0x2f,
	0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x2f, 0x73, 0x76, 0x63, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67,
	0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_rawDescData []byte
)

func file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_rawDesc), len(file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_rawDescData
}

var file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_goTypes = []any{
	(*Config)(nil),              // 0: svcconfig.Config
	(*Coordinator)(nil),         // 1: svcconfig.Coordinator
	(*Transport)(nil),           // 2: svcconfig.Transport
	(*durationpb.Duration)(nil), // 3: google.protobuf.Duration
}
var file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_depIdxs = []int32{
	2, // 0: svcconfig.Config.transport:type_name -> svcconfig.Transport
	1, // 1: svcconfig.Config.coordinator:type_name -> svcconfig.Coordinator
	3, // 2: svcconfig.Coordinator.prefix_expiration:type_name -> google.protobuf.Duration
	3, // [3:3] is the sub-list for method output_type
	3, // [3:3] is the sub-list for method input_type
	3, // [3:3] is the sub-list for extension type_name
	3, // [3:3] is the sub-list for extension extendee
	0, // [0:3] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_init() }
func file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_init() {
	if File_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto != nil {
		return
	}
	file_go_chromium_org_luci_logdog_api_config_svcconfig_transport_proto_init()
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_rawDesc), len(file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto = out.File
	file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_goTypes = nil
	file_go_chromium_org_luci_logdog_api_config_svcconfig_config_proto_depIdxs = nil
}
