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
// 	protoc-gen-go v1.28.1
// 	protoc        v3.21.7
// source: go.chromium.org/luci/analysis/proto/v1/buganizer_tester.proto

package analysispb

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
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// Request used to create the sample issue.
type CreateSampleIssueRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *CreateSampleIssueRequest) Reset() {
	*x = CreateSampleIssueRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *CreateSampleIssueRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CreateSampleIssueRequest) ProtoMessage() {}

func (x *CreateSampleIssueRequest) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CreateSampleIssueRequest.ProtoReflect.Descriptor instead.
func (*CreateSampleIssueRequest) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_rawDescGZIP(), []int{0}
}

// Response of creating a sample issue.
type CreateSampleIssueResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// The id of the created issue.
	IssueId int64 `protobuf:"varint,1,opt,name=issue_id,json=issueId,proto3" json:"issue_id,omitempty"`
}

func (x *CreateSampleIssueResponse) Reset() {
	*x = CreateSampleIssueResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *CreateSampleIssueResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CreateSampleIssueResponse) ProtoMessage() {}

func (x *CreateSampleIssueResponse) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CreateSampleIssueResponse.ProtoReflect.Descriptor instead.
func (*CreateSampleIssueResponse) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_rawDescGZIP(), []int{1}
}

func (x *CreateSampleIssueResponse) GetIssueId() int64 {
	if x != nil {
		return x.IssueId
	}
	return 0
}

var File_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_rawDesc = []byte{
	0x0a, 0x3d, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2f,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x76, 0x31, 0x2f, 0x62, 0x75, 0x67, 0x61, 0x6e, 0x69, 0x7a,
	0x65, 0x72, 0x5f, 0x74, 0x65, 0x73, 0x74, 0x65, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12,
	0x10, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2e, 0x76,
	0x31, 0x22, 0x1a, 0x0a, 0x18, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x53, 0x61, 0x6d, 0x70, 0x6c,
	0x65, 0x49, 0x73, 0x73, 0x75, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x22, 0x36, 0x0a,
	0x19, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x53, 0x61, 0x6d, 0x70, 0x6c, 0x65, 0x49, 0x73, 0x73,
	0x75, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x19, 0x0a, 0x08, 0x69, 0x73,
	0x73, 0x75, 0x65, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x03, 0x52, 0x07, 0x69, 0x73,
	0x73, 0x75, 0x65, 0x49, 0x64, 0x32, 0x81, 0x01, 0x0a, 0x0f, 0x42, 0x75, 0x67, 0x61, 0x6e, 0x69,
	0x7a, 0x65, 0x72, 0x54, 0x65, 0x73, 0x74, 0x65, 0x72, 0x12, 0x6e, 0x0a, 0x11, 0x43, 0x72, 0x65,
	0x61, 0x74, 0x65, 0x53, 0x61, 0x6d, 0x70, 0x6c, 0x65, 0x49, 0x73, 0x73, 0x75, 0x65, 0x12, 0x2a,
	0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2e, 0x76,
	0x31, 0x2e, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x53, 0x61, 0x6d, 0x70, 0x6c, 0x65, 0x49, 0x73,
	0x73, 0x75, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x2b, 0x2e, 0x6c, 0x75, 0x63,
	0x69, 0x2e, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x43, 0x72,
	0x65, 0x61, 0x74, 0x65, 0x53, 0x61, 0x6d, 0x70, 0x6c, 0x65, 0x49, 0x73, 0x73, 0x75, 0x65, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x42, 0x33, 0x5a, 0x31, 0x67, 0x6f, 0x2e,
	0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63,
	0x69, 0x2f, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x2f, 0x76, 0x31, 0x3b, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x70, 0x62, 0x62, 0x06,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_rawDescData = file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_rawDesc
)

func file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_rawDescData = protoimpl.X.CompressGZIP(file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_rawDescData)
	})
	return file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_rawDescData
}

var file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_goTypes = []interface{}{
	(*CreateSampleIssueRequest)(nil),  // 0: luci.analysis.v1.CreateSampleIssueRequest
	(*CreateSampleIssueResponse)(nil), // 1: luci.analysis.v1.CreateSampleIssueResponse
}
var file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_depIdxs = []int32{
	0, // 0: luci.analysis.v1.BuganizerTester.CreateSampleIssue:input_type -> luci.analysis.v1.CreateSampleIssueRequest
	1, // 1: luci.analysis.v1.BuganizerTester.CreateSampleIssue:output_type -> luci.analysis.v1.CreateSampleIssueResponse
	1, // [1:2] is the sub-list for method output_type
	0, // [0:1] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_init() }
func file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_init() {
	if File_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*CreateSampleIssueRequest); i {
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
		file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*CreateSampleIssueResponse); i {
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
			RawDescriptor: file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto = out.File
	file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_rawDesc = nil
	file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_goTypes = nil
	file_go_chromium_org_luci_analysis_proto_v1_buganizer_tester_proto_depIdxs = nil
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConnInterface

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion6

// BuganizerTesterClient is the client API for BuganizerTester service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type BuganizerTesterClient interface {
	// Creates a sample issue and returns it's id.
	CreateSampleIssue(ctx context.Context, in *CreateSampleIssueRequest, opts ...grpc.CallOption) (*CreateSampleIssueResponse, error)
}
type buganizerTesterPRPCClient struct {
	client *prpc.Client
}

func NewBuganizerTesterPRPCClient(client *prpc.Client) BuganizerTesterClient {
	return &buganizerTesterPRPCClient{client}
}

func (c *buganizerTesterPRPCClient) CreateSampleIssue(ctx context.Context, in *CreateSampleIssueRequest, opts ...grpc.CallOption) (*CreateSampleIssueResponse, error) {
	out := new(CreateSampleIssueResponse)
	err := c.client.Call(ctx, "luci.analysis.v1.BuganizerTester", "CreateSampleIssue", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type buganizerTesterClient struct {
	cc grpc.ClientConnInterface
}

func NewBuganizerTesterClient(cc grpc.ClientConnInterface) BuganizerTesterClient {
	return &buganizerTesterClient{cc}
}

func (c *buganizerTesterClient) CreateSampleIssue(ctx context.Context, in *CreateSampleIssueRequest, opts ...grpc.CallOption) (*CreateSampleIssueResponse, error) {
	out := new(CreateSampleIssueResponse)
	err := c.cc.Invoke(ctx, "/luci.analysis.v1.BuganizerTester/CreateSampleIssue", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// BuganizerTesterServer is the server API for BuganizerTester service.
type BuganizerTesterServer interface {
	// Creates a sample issue and returns it's id.
	CreateSampleIssue(context.Context, *CreateSampleIssueRequest) (*CreateSampleIssueResponse, error)
}

// UnimplementedBuganizerTesterServer can be embedded to have forward compatible implementations.
type UnimplementedBuganizerTesterServer struct {
}

func (*UnimplementedBuganizerTesterServer) CreateSampleIssue(context.Context, *CreateSampleIssueRequest) (*CreateSampleIssueResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateSampleIssue not implemented")
}

func RegisterBuganizerTesterServer(s prpc.Registrar, srv BuganizerTesterServer) {
	s.RegisterService(&_BuganizerTester_serviceDesc, srv)
}

func _BuganizerTester_CreateSampleIssue_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateSampleIssueRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(BuganizerTesterServer).CreateSampleIssue(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/luci.analysis.v1.BuganizerTester/CreateSampleIssue",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(BuganizerTesterServer).CreateSampleIssue(ctx, req.(*CreateSampleIssueRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _BuganizerTester_serviceDesc = grpc.ServiceDesc{
	ServiceName: "luci.analysis.v1.BuganizerTester",
	HandlerType: (*BuganizerTesterServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "CreateSampleIssue",
			Handler:    _BuganizerTester_CreateSampleIssue_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "go.chromium.org/luci/analysis/proto/v1/buganizer_tester.proto",
}