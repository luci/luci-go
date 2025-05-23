// Copyright 2021 The LUCI Authors.
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
// source: go.chromium.org/luci/luci_notify/api/service/v1/rpc.proto

package lucinotifypb

import prpc "go.chromium.org/luci/grpc/prpc"

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
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

type CheckTreeCloserRequest struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Project of the builder
	Project string `protobuf:"bytes,1,opt,name=project,proto3" json:"project,omitempty"`
	// Bucket of the builder
	Bucket string `protobuf:"bytes,2,opt,name=bucket,proto3" json:"bucket,omitempty"`
	// Name of the builder
	Builder string `protobuf:"bytes,3,opt,name=builder,proto3" json:"builder,omitempty"`
	// Some tree closers are only close if some particular steps failed.
	Step          string `protobuf:"bytes,4,opt,name=step,proto3" json:"step,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *CheckTreeCloserRequest) Reset() {
	*x = CheckTreeCloserRequest{}
	mi := &file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *CheckTreeCloserRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CheckTreeCloserRequest) ProtoMessage() {}

func (x *CheckTreeCloserRequest) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CheckTreeCloserRequest.ProtoReflect.Descriptor instead.
func (*CheckTreeCloserRequest) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_rawDescGZIP(), []int{0}
}

func (x *CheckTreeCloserRequest) GetProject() string {
	if x != nil {
		return x.Project
	}
	return ""
}

func (x *CheckTreeCloserRequest) GetBucket() string {
	if x != nil {
		return x.Bucket
	}
	return ""
}

func (x *CheckTreeCloserRequest) GetBuilder() string {
	if x != nil {
		return x.Builder
	}
	return ""
}

func (x *CheckTreeCloserRequest) GetStep() string {
	if x != nil {
		return x.Step
	}
	return ""
}

type CheckTreeCloserResponse struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Whether this is a tree closer
	IsTreeCloser  bool `protobuf:"varint,1,opt,name=is_tree_closer,json=isTreeCloser,proto3" json:"is_tree_closer,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *CheckTreeCloserResponse) Reset() {
	*x = CheckTreeCloserResponse{}
	mi := &file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *CheckTreeCloserResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CheckTreeCloserResponse) ProtoMessage() {}

func (x *CheckTreeCloserResponse) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CheckTreeCloserResponse.ProtoReflect.Descriptor instead.
func (*CheckTreeCloserResponse) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_rawDescGZIP(), []int{1}
}

func (x *CheckTreeCloserResponse) GetIsTreeCloser() bool {
	if x != nil {
		return x.IsTreeCloser
	}
	return false
}

var File_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_rawDesc = string([]byte{
	0x0a, 0x39, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x5f, 0x6e, 0x6f, 0x74, 0x69,
	0x66, 0x79, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x2f, 0x76,
	0x31, 0x2f, 0x72, 0x70, 0x63, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0e, 0x6c, 0x75, 0x63,
	0x69, 0x2e, 0x6e, 0x6f, 0x74, 0x69, 0x66, 0x79, 0x2e, 0x76, 0x31, 0x22, 0x78, 0x0a, 0x16, 0x43,
	0x68, 0x65, 0x63, 0x6b, 0x54, 0x72, 0x65, 0x65, 0x43, 0x6c, 0x6f, 0x73, 0x65, 0x72, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x18, 0x0a, 0x07, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x12,
	0x16, 0x0a, 0x06, 0x62, 0x75, 0x63, 0x6b, 0x65, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x06, 0x62, 0x75, 0x63, 0x6b, 0x65, 0x74, 0x12, 0x18, 0x0a, 0x07, 0x62, 0x75, 0x69, 0x6c, 0x64,
	0x65, 0x72, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x62, 0x75, 0x69, 0x6c, 0x64, 0x65,
	0x72, 0x12, 0x12, 0x0a, 0x04, 0x73, 0x74, 0x65, 0x70, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x04, 0x73, 0x74, 0x65, 0x70, 0x22, 0x3f, 0x0a, 0x17, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x54, 0x72,
	0x65, 0x65, 0x43, 0x6c, 0x6f, 0x73, 0x65, 0x72, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x12, 0x24, 0x0a, 0x0e, 0x69, 0x73, 0x5f, 0x74, 0x72, 0x65, 0x65, 0x5f, 0x63, 0x6c, 0x6f, 0x73,
	0x65, 0x72, 0x18, 0x01, 0x20, 0x01, 0x28, 0x08, 0x52, 0x0c, 0x69, 0x73, 0x54, 0x72, 0x65, 0x65,
	0x43, 0x6c, 0x6f, 0x73, 0x65, 0x72, 0x32, 0x72, 0x0a, 0x0a, 0x54, 0x72, 0x65, 0x65, 0x43, 0x6c,
	0x6f, 0x73, 0x65, 0x72, 0x12, 0x64, 0x0a, 0x0f, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x54, 0x72, 0x65,
	0x65, 0x43, 0x6c, 0x6f, 0x73, 0x65, 0x72, 0x12, 0x26, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x6e,
	0x6f, 0x74, 0x69, 0x66, 0x79, 0x2e, 0x76, 0x31, 0x2e, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x54, 0x72,
	0x65, 0x65, 0x43, 0x6c, 0x6f, 0x73, 0x65, 0x72, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a,
	0x27, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x6e, 0x6f, 0x74, 0x69, 0x66, 0x79, 0x2e, 0x76, 0x31,
	0x2e, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x54, 0x72, 0x65, 0x65, 0x43, 0x6c, 0x6f, 0x73, 0x65, 0x72,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x42, 0x3e, 0x5a, 0x3c, 0x67, 0x6f,
	0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75,
	0x63, 0x69, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x5f, 0x6e, 0x6f, 0x74, 0x69, 0x66, 0x79, 0x2f, 0x61,
	0x70, 0x69, 0x2f, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x2f, 0x76, 0x31, 0x3b, 0x6c, 0x75,
	0x63, 0x69, 0x6e, 0x6f, 0x74, 0x69, 0x66, 0x79, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_rawDescData []byte
)

func file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_rawDesc), len(file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_rawDescData
}

var file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_goTypes = []any{
	(*CheckTreeCloserRequest)(nil),  // 0: luci.notify.v1.CheckTreeCloserRequest
	(*CheckTreeCloserResponse)(nil), // 1: luci.notify.v1.CheckTreeCloserResponse
}
var file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_depIdxs = []int32{
	0, // 0: luci.notify.v1.TreeCloser.CheckTreeCloser:input_type -> luci.notify.v1.CheckTreeCloserRequest
	1, // 1: luci.notify.v1.TreeCloser.CheckTreeCloser:output_type -> luci.notify.v1.CheckTreeCloserResponse
	1, // [1:2] is the sub-list for method output_type
	0, // [0:1] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_init() }
func file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_init() {
	if File_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_rawDesc), len(file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto = out.File
	file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_goTypes = nil
	file_go_chromium_org_luci_luci_notify_api_service_v1_rpc_proto_depIdxs = nil
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConnInterface

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion6

// TreeCloserClient is the client API for TreeCloser service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type TreeCloserClient interface {
	// Checks if the builder in CheckTreeCloserRequest is tree-closer or not
	CheckTreeCloser(ctx context.Context, in *CheckTreeCloserRequest, opts ...grpc.CallOption) (*CheckTreeCloserResponse, error)
}
type treeCloserPRPCClient struct {
	client *prpc.Client
}

func NewTreeCloserPRPCClient(client *prpc.Client) TreeCloserClient {
	return &treeCloserPRPCClient{client}
}

func (c *treeCloserPRPCClient) CheckTreeCloser(ctx context.Context, in *CheckTreeCloserRequest, opts ...grpc.CallOption) (*CheckTreeCloserResponse, error) {
	out := new(CheckTreeCloserResponse)
	err := c.client.Call(ctx, "luci.notify.v1.TreeCloser", "CheckTreeCloser", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type treeCloserClient struct {
	cc grpc.ClientConnInterface
}

func NewTreeCloserClient(cc grpc.ClientConnInterface) TreeCloserClient {
	return &treeCloserClient{cc}
}

func (c *treeCloserClient) CheckTreeCloser(ctx context.Context, in *CheckTreeCloserRequest, opts ...grpc.CallOption) (*CheckTreeCloserResponse, error) {
	out := new(CheckTreeCloserResponse)
	err := c.cc.Invoke(ctx, "/luci.notify.v1.TreeCloser/CheckTreeCloser", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// TreeCloserServer is the server API for TreeCloser service.
type TreeCloserServer interface {
	// Checks if the builder in CheckTreeCloserRequest is tree-closer or not
	CheckTreeCloser(context.Context, *CheckTreeCloserRequest) (*CheckTreeCloserResponse, error)
}

// UnimplementedTreeCloserServer can be embedded to have forward compatible implementations.
type UnimplementedTreeCloserServer struct {
}

func (*UnimplementedTreeCloserServer) CheckTreeCloser(context.Context, *CheckTreeCloserRequest) (*CheckTreeCloserResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CheckTreeCloser not implemented")
}

func RegisterTreeCloserServer(s prpc.Registrar, srv TreeCloserServer) {
	s.RegisterService(&_TreeCloser_serviceDesc, srv)
}

func _TreeCloser_CheckTreeCloser_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CheckTreeCloserRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TreeCloserServer).CheckTreeCloser(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/luci.notify.v1.TreeCloser/CheckTreeCloser",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TreeCloserServer).CheckTreeCloser(ctx, req.(*CheckTreeCloserRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _TreeCloser_serviceDesc = grpc.ServiceDesc{
	ServiceName: "luci.notify.v1.TreeCloser",
	HandlerType: (*TreeCloserServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "CheckTreeCloser",
			Handler:    _TreeCloser_CheckTreeCloser_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "go.chromium.org/luci/luci_notify/api/service/v1/rpc.proto",
}
