// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.5
// 	protoc        v6.30.2
// source: go.chromium.org/luci/tokenserver/api/token_file.proto

package tokenserver

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

// TokenFile is representation of a token file on disk (serialized as JSON).
//
// The token file is consumed by whoever wishes to use machine tokens. It is
// intentionally made as simple as possible (e.g. uses unix timestamps instead
// of fancy protobuf ones).
type TokenFile struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Google OAuth2 access token of a machine service account.
	AccessToken string `protobuf:"bytes,1,opt,name=access_token,proto3" json:"access_token,omitempty"`
	// OAuth2 access token type, usually "Bearer".
	TokenType string `protobuf:"bytes,2,opt,name=token_type,proto3" json:"token_type,omitempty"`
	// Machine token understood by LUCI backends (alternative to access_token).
	LuciMachineToken string `protobuf:"bytes,3,opt,name=luci_machine_token,proto3" json:"luci_machine_token,omitempty"`
	// Unix timestamp (in seconds) when this token expires.
	//
	// The token file is expected to be updated before the token expires, see
	// 'next_update' for next expected update time.
	Expiry int64 `protobuf:"varint,4,opt,name=expiry,proto3" json:"expiry,omitempty"`
	// Unix timestamp of when this file was updated the last time.
	LastUpdate int64 `protobuf:"varint,5,opt,name=last_update,proto3" json:"last_update,omitempty"`
	// Unix timestamp of when this file is expected to be updated next time.
	NextUpdate int64 `protobuf:"varint,6,opt,name=next_update,proto3" json:"next_update,omitempty"`
	// Email of the associated service account.
	ServiceAccountEmail string `protobuf:"bytes,7,opt,name=service_account_email,proto3" json:"service_account_email,omitempty"`
	// Unique stable ID of the associated service account.
	ServiceAccountUniqueId string `protobuf:"bytes,8,opt,name=service_account_unique_id,proto3" json:"service_account_unique_id,omitempty"`
	// Any information tokend daemon wishes to associate with the token.
	//
	// Consumers of the token file should ignore this field. It is used
	// exclusively by tokend daemon.
	TokendState   []byte `protobuf:"bytes,50,opt,name=tokend_state,proto3" json:"tokend_state,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TokenFile) Reset() {
	*x = TokenFile{}
	mi := &file_go_chromium_org_luci_tokenserver_api_token_file_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TokenFile) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TokenFile) ProtoMessage() {}

func (x *TokenFile) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_tokenserver_api_token_file_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TokenFile.ProtoReflect.Descriptor instead.
func (*TokenFile) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_tokenserver_api_token_file_proto_rawDescGZIP(), []int{0}
}

func (x *TokenFile) GetAccessToken() string {
	if x != nil {
		return x.AccessToken
	}
	return ""
}

func (x *TokenFile) GetTokenType() string {
	if x != nil {
		return x.TokenType
	}
	return ""
}

func (x *TokenFile) GetLuciMachineToken() string {
	if x != nil {
		return x.LuciMachineToken
	}
	return ""
}

func (x *TokenFile) GetExpiry() int64 {
	if x != nil {
		return x.Expiry
	}
	return 0
}

func (x *TokenFile) GetLastUpdate() int64 {
	if x != nil {
		return x.LastUpdate
	}
	return 0
}

func (x *TokenFile) GetNextUpdate() int64 {
	if x != nil {
		return x.NextUpdate
	}
	return 0
}

func (x *TokenFile) GetServiceAccountEmail() string {
	if x != nil {
		return x.ServiceAccountEmail
	}
	return ""
}

func (x *TokenFile) GetServiceAccountUniqueId() string {
	if x != nil {
		return x.ServiceAccountUniqueId
	}
	return ""
}

func (x *TokenFile) GetTokendState() []byte {
	if x != nil {
		return x.TokendState
	}
	return nil
}

var File_go_chromium_org_luci_tokenserver_api_token_file_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_tokenserver_api_token_file_proto_rawDesc = string([]byte{
	0x0a, 0x35, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x73, 0x65, 0x72, 0x76,
	0x65, 0x72, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x5f, 0x66, 0x69, 0x6c,
	0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0b, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x73, 0x65,
	0x72, 0x76, 0x65, 0x72, 0x22, 0xf3, 0x02, 0x0a, 0x09, 0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x46, 0x69,
	0x6c, 0x65, 0x12, 0x22, 0x0a, 0x0c, 0x61, 0x63, 0x63, 0x65, 0x73, 0x73, 0x5f, 0x74, 0x6f, 0x6b,
	0x65, 0x6e, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0c, 0x61, 0x63, 0x63, 0x65, 0x73, 0x73,
	0x5f, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x12, 0x1e, 0x0a, 0x0a, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x5f,
	0x74, 0x79, 0x70, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x74, 0x6f, 0x6b, 0x65,
	0x6e, 0x5f, 0x74, 0x79, 0x70, 0x65, 0x12, 0x2e, 0x0a, 0x12, 0x6c, 0x75, 0x63, 0x69, 0x5f, 0x6d,
	0x61, 0x63, 0x68, 0x69, 0x6e, 0x65, 0x5f, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x18, 0x03, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x12, 0x6c, 0x75, 0x63, 0x69, 0x5f, 0x6d, 0x61, 0x63, 0x68, 0x69, 0x6e, 0x65,
	0x5f, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x12, 0x16, 0x0a, 0x06, 0x65, 0x78, 0x70, 0x69, 0x72, 0x79,
	0x18, 0x04, 0x20, 0x01, 0x28, 0x03, 0x52, 0x06, 0x65, 0x78, 0x70, 0x69, 0x72, 0x79, 0x12, 0x20,
	0x0a, 0x0b, 0x6c, 0x61, 0x73, 0x74, 0x5f, 0x75, 0x70, 0x64, 0x61, 0x74, 0x65, 0x18, 0x05, 0x20,
	0x01, 0x28, 0x03, 0x52, 0x0b, 0x6c, 0x61, 0x73, 0x74, 0x5f, 0x75, 0x70, 0x64, 0x61, 0x74, 0x65,
	0x12, 0x20, 0x0a, 0x0b, 0x6e, 0x65, 0x78, 0x74, 0x5f, 0x75, 0x70, 0x64, 0x61, 0x74, 0x65, 0x18,
	0x06, 0x20, 0x01, 0x28, 0x03, 0x52, 0x0b, 0x6e, 0x65, 0x78, 0x74, 0x5f, 0x75, 0x70, 0x64, 0x61,
	0x74, 0x65, 0x12, 0x34, 0x0a, 0x15, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x5f, 0x61, 0x63,
	0x63, 0x6f, 0x75, 0x6e, 0x74, 0x5f, 0x65, 0x6d, 0x61, 0x69, 0x6c, 0x18, 0x07, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x15, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x5f, 0x61, 0x63, 0x63, 0x6f, 0x75,
	0x6e, 0x74, 0x5f, 0x65, 0x6d, 0x61, 0x69, 0x6c, 0x12, 0x3c, 0x0a, 0x19, 0x73, 0x65, 0x72, 0x76,
	0x69, 0x63, 0x65, 0x5f, 0x61, 0x63, 0x63, 0x6f, 0x75, 0x6e, 0x74, 0x5f, 0x75, 0x6e, 0x69, 0x71,
	0x75, 0x65, 0x5f, 0x69, 0x64, 0x18, 0x08, 0x20, 0x01, 0x28, 0x09, 0x52, 0x19, 0x73, 0x65, 0x72,
	0x76, 0x69, 0x63, 0x65, 0x5f, 0x61, 0x63, 0x63, 0x6f, 0x75, 0x6e, 0x74, 0x5f, 0x75, 0x6e, 0x69,
	0x71, 0x75, 0x65, 0x5f, 0x69, 0x64, 0x12, 0x22, 0x0a, 0x0c, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x64,
	0x5f, 0x73, 0x74, 0x61, 0x74, 0x65, 0x18, 0x32, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x0c, 0x74, 0x6f,
	0x6b, 0x65, 0x6e, 0x64, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x65, 0x42, 0x32, 0x5a, 0x30, 0x67, 0x6f,
	0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75,
	0x63, 0x69, 0x2f, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2f, 0x61,
	0x70, 0x69, 0x3b, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x62, 0x06,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_tokenserver_api_token_file_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_tokenserver_api_token_file_proto_rawDescData []byte
)

func file_go_chromium_org_luci_tokenserver_api_token_file_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_tokenserver_api_token_file_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_tokenserver_api_token_file_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_tokenserver_api_token_file_proto_rawDesc), len(file_go_chromium_org_luci_tokenserver_api_token_file_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_tokenserver_api_token_file_proto_rawDescData
}

var file_go_chromium_org_luci_tokenserver_api_token_file_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_go_chromium_org_luci_tokenserver_api_token_file_proto_goTypes = []any{
	(*TokenFile)(nil), // 0: tokenserver.TokenFile
}
var file_go_chromium_org_luci_tokenserver_api_token_file_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_tokenserver_api_token_file_proto_init() }
func file_go_chromium_org_luci_tokenserver_api_token_file_proto_init() {
	if File_go_chromium_org_luci_tokenserver_api_token_file_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_tokenserver_api_token_file_proto_rawDesc), len(file_go_chromium_org_luci_tokenserver_api_token_file_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_tokenserver_api_token_file_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_tokenserver_api_token_file_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_tokenserver_api_token_file_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_tokenserver_api_token_file_proto = out.File
	file_go_chromium_org_luci_tokenserver_api_token_file_proto_goTypes = nil
	file_go_chromium_org_luci_tokenserver_api_token_file_proto_depIdxs = nil
}
