// Copyright 2024 The LUCI Authors.
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
// source: go.chromium.org/luci/source_index/proto/config/config.proto

package configpb

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

// The service-level config for LUCI Source Index.
type Config struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Required. A list of gitiles host to index.
	//
	// See go/luci-source-index-new-repo-setup for all the steps required to set
	// up a new host.
	Hosts         []*Config_Host `protobuf:"bytes,1,rep,name=hosts,proto3" json:"hosts,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Config) Reset() {
	*x = Config{}
	mi := &file_go_chromium_org_luci_source_index_proto_config_config_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Config) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Config) ProtoMessage() {}

func (x *Config) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_source_index_proto_config_config_proto_msgTypes[0]
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
	return file_go_chromium_org_luci_source_index_proto_config_config_proto_rawDescGZIP(), []int{0}
}

func (x *Config) GetHosts() []*Config_Host {
	if x != nil {
		return x.Hosts
	}
	return nil
}

type Config_Host struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Required. The gitiles host. Must be a subdomain of `.googlesource.com`
	// (e.g. chromium.googlesource.com).
	Host string `protobuf:"bytes,1,opt,name=host,proto3" json:"host,omitempty"`
	// Required. A list of repositories to be indexed.
	//
	// See go/luci-source-index-new-repo-setup for all the steps required to
	// set up a new repository.
	Repositories  []*Config_Host_Repository `protobuf:"bytes,2,rep,name=repositories,proto3" json:"repositories,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Config_Host) Reset() {
	*x = Config_Host{}
	mi := &file_go_chromium_org_luci_source_index_proto_config_config_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Config_Host) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Config_Host) ProtoMessage() {}

func (x *Config_Host) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_source_index_proto_config_config_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Config_Host.ProtoReflect.Descriptor instead.
func (*Config_Host) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_source_index_proto_config_config_proto_rawDescGZIP(), []int{0, 0}
}

func (x *Config_Host) GetHost() string {
	if x != nil {
		return x.Host
	}
	return ""
}

func (x *Config_Host) GetRepositories() []*Config_Host_Repository {
	if x != nil {
		return x.Repositories
	}
	return nil
}

type Config_Host_Repository struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Required. The name of the gitiles project, for example "chromium/src"
	// or "v8/v8".
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Required. A list of refs to be indexed, specified as a list of ref
	// regexes. The regexes are automatically wrapped in ^ and $.
	//
	// Additionally, only refs that begin with
	//   - refs/branch-heads/
	//   - refs/heads/
	//
	// will be indexed.
	IncludeRefRegexes []string `protobuf:"bytes,2,rep,name=include_ref_regexes,json=includeRefRegexes,proto3" json:"include_ref_regexes,omitempty"`
	unknownFields     protoimpl.UnknownFields
	sizeCache         protoimpl.SizeCache
}

func (x *Config_Host_Repository) Reset() {
	*x = Config_Host_Repository{}
	mi := &file_go_chromium_org_luci_source_index_proto_config_config_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Config_Host_Repository) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Config_Host_Repository) ProtoMessage() {}

func (x *Config_Host_Repository) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_source_index_proto_config_config_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Config_Host_Repository.ProtoReflect.Descriptor instead.
func (*Config_Host_Repository) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_source_index_proto_config_config_proto_rawDescGZIP(), []int{0, 0, 0}
}

func (x *Config_Host_Repository) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *Config_Host_Repository) GetIncludeRefRegexes() []string {
	if x != nil {
		return x.IncludeRefRegexes
	}
	return nil
}

var File_go_chromium_org_luci_source_index_proto_config_config_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_source_index_proto_config_config_proto_rawDesc = string([]byte{
	0x0a, 0x3b, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x5f, 0x69, 0x6e,
	0x64, 0x65, 0x78, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67,
	0x2f, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x18, 0x6c,
	0x75, 0x63, 0x69, 0x2e, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x5f, 0x69, 0x6e, 0x64, 0x65, 0x78,
	0x2e, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x22, 0x8a, 0x02, 0x0a, 0x06, 0x43, 0x6f, 0x6e, 0x66,
	0x69, 0x67, 0x12, 0x3b, 0x0a, 0x05, 0x68, 0x6f, 0x73, 0x74, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28,
	0x0b, 0x32, 0x25, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x5f,
	0x69, 0x6e, 0x64, 0x65, 0x78, 0x2e, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x2e, 0x43, 0x6f, 0x6e,
	0x66, 0x69, 0x67, 0x2e, 0x48, 0x6f, 0x73, 0x74, 0x52, 0x05, 0x68, 0x6f, 0x73, 0x74, 0x73, 0x1a,
	0xc2, 0x01, 0x0a, 0x04, 0x48, 0x6f, 0x73, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x68, 0x6f, 0x73, 0x74,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x68, 0x6f, 0x73, 0x74, 0x12, 0x54, 0x0a, 0x0c,
	0x72, 0x65, 0x70, 0x6f, 0x73, 0x69, 0x74, 0x6f, 0x72, 0x69, 0x65, 0x73, 0x18, 0x02, 0x20, 0x03,
	0x28, 0x0b, 0x32, 0x30, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65,
	0x5f, 0x69, 0x6e, 0x64, 0x65, 0x78, 0x2e, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x2e, 0x43, 0x6f,
	0x6e, 0x66, 0x69, 0x67, 0x2e, 0x48, 0x6f, 0x73, 0x74, 0x2e, 0x52, 0x65, 0x70, 0x6f, 0x73, 0x69,
	0x74, 0x6f, 0x72, 0x79, 0x52, 0x0c, 0x72, 0x65, 0x70, 0x6f, 0x73, 0x69, 0x74, 0x6f, 0x72, 0x69,
	0x65, 0x73, 0x1a, 0x50, 0x0a, 0x0a, 0x52, 0x65, 0x70, 0x6f, 0x73, 0x69, 0x74, 0x6f, 0x72, 0x79,
	0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04,
	0x6e, 0x61, 0x6d, 0x65, 0x12, 0x2e, 0x0a, 0x13, 0x69, 0x6e, 0x63, 0x6c, 0x75, 0x64, 0x65, 0x5f,
	0x72, 0x65, 0x66, 0x5f, 0x72, 0x65, 0x67, 0x65, 0x78, 0x65, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28,
	0x09, 0x52, 0x11, 0x69, 0x6e, 0x63, 0x6c, 0x75, 0x64, 0x65, 0x52, 0x65, 0x66, 0x52, 0x65, 0x67,
	0x65, 0x78, 0x65, 0x73, 0x42, 0x39, 0x5a, 0x37, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d,
	0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x73, 0x6f, 0x75,
	0x72, 0x63, 0x65, 0x5f, 0x69, 0x6e, 0x64, 0x65, 0x78, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f,
	0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x3b, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x70, 0x62, 0x62,
	0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_source_index_proto_config_config_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_source_index_proto_config_config_proto_rawDescData []byte
)

func file_go_chromium_org_luci_source_index_proto_config_config_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_source_index_proto_config_config_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_source_index_proto_config_config_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_source_index_proto_config_config_proto_rawDesc), len(file_go_chromium_org_luci_source_index_proto_config_config_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_source_index_proto_config_config_proto_rawDescData
}

var file_go_chromium_org_luci_source_index_proto_config_config_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_go_chromium_org_luci_source_index_proto_config_config_proto_goTypes = []any{
	(*Config)(nil),                 // 0: luci.source_index.config.Config
	(*Config_Host)(nil),            // 1: luci.source_index.config.Config.Host
	(*Config_Host_Repository)(nil), // 2: luci.source_index.config.Config.Host.Repository
}
var file_go_chromium_org_luci_source_index_proto_config_config_proto_depIdxs = []int32{
	1, // 0: luci.source_index.config.Config.hosts:type_name -> luci.source_index.config.Config.Host
	2, // 1: luci.source_index.config.Config.Host.repositories:type_name -> luci.source_index.config.Config.Host.Repository
	2, // [2:2] is the sub-list for method output_type
	2, // [2:2] is the sub-list for method input_type
	2, // [2:2] is the sub-list for extension type_name
	2, // [2:2] is the sub-list for extension extendee
	0, // [0:2] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_source_index_proto_config_config_proto_init() }
func file_go_chromium_org_luci_source_index_proto_config_config_proto_init() {
	if File_go_chromium_org_luci_source_index_proto_config_config_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_source_index_proto_config_config_proto_rawDesc), len(file_go_chromium_org_luci_source_index_proto_config_config_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_source_index_proto_config_config_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_source_index_proto_config_config_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_source_index_proto_config_config_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_source_index_proto_config_config_proto = out.File
	file_go_chromium_org_luci_source_index_proto_config_config_proto_goTypes = nil
	file_go_chromium_org_luci_source_index_proto_config_config_proto_depIdxs = nil
}
