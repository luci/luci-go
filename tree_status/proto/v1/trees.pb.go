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
// source: go.chromium.org/luci/tree_status/proto/v1/trees.proto

package v1

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

type GetTreeRequest struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Format: "trees/{tree_id}"
	Name          string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *GetTreeRequest) Reset() {
	*x = GetTreeRequest{}
	mi := &file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetTreeRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetTreeRequest) ProtoMessage() {}

func (x *GetTreeRequest) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetTreeRequest.ProtoReflect.Descriptor instead.
func (*GetTreeRequest) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_rawDescGZIP(), []int{0}
}

func (x *GetTreeRequest) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

type QueryTreesRequest struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The LUCI project to query tree name.
	Project       string `protobuf:"bytes,1,opt,name=project,proto3" json:"project,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *QueryTreesRequest) Reset() {
	*x = QueryTreesRequest{}
	mi := &file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *QueryTreesRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*QueryTreesRequest) ProtoMessage() {}

func (x *QueryTreesRequest) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use QueryTreesRequest.ProtoReflect.Descriptor instead.
func (*QueryTreesRequest) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_rawDescGZIP(), []int{1}
}

func (x *QueryTreesRequest) GetProject() string {
	if x != nil {
		return x.Project
	}
	return ""
}

type QueryTreesResponse struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// List of trees attached to the project.
	// If there are more than 1 tree attached to a project, the results
	// will be sorted ascendingly based on tree name.
	Trees         []*Tree `protobuf:"bytes,1,rep,name=trees,proto3" json:"trees,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *QueryTreesResponse) Reset() {
	*x = QueryTreesResponse{}
	mi := &file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *QueryTreesResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*QueryTreesResponse) ProtoMessage() {}

func (x *QueryTreesResponse) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use QueryTreesResponse.ProtoReflect.Descriptor instead.
func (*QueryTreesResponse) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_rawDescGZIP(), []int{2}
}

func (x *QueryTreesResponse) GetTrees() []*Tree {
	if x != nil {
		return x.Trees
	}
	return nil
}

type Tree struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Name of the tree, in format "trees/{tree_id}".
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// The LUCI projects that the tree applies to..
	// The first project in this list is the primary project. This means:
	//  1. Its "<project>:<subrealm>" realm will be used to check
	//     for ACL for the tree, if the tree uses realm-based ACL.
	//  2. If the tree is access without a LUCI project context, the primary project
	//     will be displayed at the top left of LUCI UI.
	Projects      []string `protobuf:"bytes,2,rep,name=projects,proto3" json:"projects,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Tree) Reset() {
	*x = Tree{}
	mi := &file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Tree) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Tree) ProtoMessage() {}

func (x *Tree) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Tree.ProtoReflect.Descriptor instead.
func (*Tree) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_rawDescGZIP(), []int{3}
}

func (x *Tree) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *Tree) GetProjects() []string {
	if x != nil {
		return x.Projects
	}
	return nil
}

var File_go_chromium_org_luci_tree_status_proto_v1_trees_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_rawDesc = string([]byte{
	0x0a, 0x35, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x74, 0x72, 0x65, 0x65, 0x5f, 0x73, 0x74, 0x61, 0x74,
	0x75, 0x73, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x76, 0x31, 0x2f, 0x74, 0x72, 0x65, 0x65,
	0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x13, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x74, 0x72,
	0x65, 0x65, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x2e, 0x76, 0x31, 0x22, 0x24, 0x0a, 0x0e,
	0x47, 0x65, 0x74, 0x54, 0x72, 0x65, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x12,
	0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61,
	0x6d, 0x65, 0x22, 0x2d, 0x0a, 0x11, 0x51, 0x75, 0x65, 0x72, 0x79, 0x54, 0x72, 0x65, 0x65, 0x73,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x18, 0x0a, 0x07, 0x70, 0x72, 0x6f, 0x6a, 0x65,
	0x63, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63,
	0x74, 0x22, 0x45, 0x0a, 0x12, 0x51, 0x75, 0x65, 0x72, 0x79, 0x54, 0x72, 0x65, 0x65, 0x73, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x2f, 0x0a, 0x05, 0x74, 0x72, 0x65, 0x65, 0x73,
	0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x74, 0x72,
	0x65, 0x65, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x54, 0x72, 0x65,
	0x65, 0x52, 0x05, 0x74, 0x72, 0x65, 0x65, 0x73, 0x22, 0x36, 0x0a, 0x04, 0x54, 0x72, 0x65, 0x65,
	0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04,
	0x6e, 0x61, 0x6d, 0x65, 0x12, 0x1a, 0x0a, 0x08, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x73,
	0x18, 0x02, 0x20, 0x03, 0x28, 0x09, 0x52, 0x08, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x73,
	0x32, 0xb5, 0x01, 0x0a, 0x05, 0x54, 0x72, 0x65, 0x65, 0x73, 0x12, 0x4b, 0x0a, 0x07, 0x47, 0x65,
	0x74, 0x54, 0x72, 0x65, 0x65, 0x12, 0x23, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x74, 0x72, 0x65,
	0x65, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x47, 0x65, 0x74, 0x54,
	0x72, 0x65, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x19, 0x2e, 0x6c, 0x75, 0x63,
	0x69, 0x2e, 0x74, 0x72, 0x65, 0x65, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x2e, 0x76, 0x31,
	0x2e, 0x54, 0x72, 0x65, 0x65, 0x22, 0x00, 0x12, 0x5f, 0x0a, 0x0a, 0x51, 0x75, 0x65, 0x72, 0x79,
	0x54, 0x72, 0x65, 0x65, 0x73, 0x12, 0x26, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x74, 0x72, 0x65,
	0x65, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x51, 0x75, 0x65, 0x72,
	0x79, 0x54, 0x72, 0x65, 0x65, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x27, 0x2e,
	0x6c, 0x75, 0x63, 0x69, 0x2e, 0x74, 0x72, 0x65, 0x65, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73,
	0x2e, 0x76, 0x31, 0x2e, 0x51, 0x75, 0x65, 0x72, 0x79, 0x54, 0x72, 0x65, 0x65, 0x73, 0x52, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x42, 0x2b, 0x5a, 0x29, 0x67, 0x6f, 0x2e, 0x63,
	0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69,
	0x2f, 0x74, 0x72, 0x65, 0x65, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x2f, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x2f, 0x76, 0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_rawDescData []byte
)

func file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_rawDesc), len(file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_rawDescData
}

var file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_goTypes = []any{
	(*GetTreeRequest)(nil),     // 0: luci.tree_status.v1.GetTreeRequest
	(*QueryTreesRequest)(nil),  // 1: luci.tree_status.v1.QueryTreesRequest
	(*QueryTreesResponse)(nil), // 2: luci.tree_status.v1.QueryTreesResponse
	(*Tree)(nil),               // 3: luci.tree_status.v1.Tree
}
var file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_depIdxs = []int32{
	3, // 0: luci.tree_status.v1.QueryTreesResponse.trees:type_name -> luci.tree_status.v1.Tree
	0, // 1: luci.tree_status.v1.Trees.GetTree:input_type -> luci.tree_status.v1.GetTreeRequest
	1, // 2: luci.tree_status.v1.Trees.QueryTrees:input_type -> luci.tree_status.v1.QueryTreesRequest
	3, // 3: luci.tree_status.v1.Trees.GetTree:output_type -> luci.tree_status.v1.Tree
	2, // 4: luci.tree_status.v1.Trees.QueryTrees:output_type -> luci.tree_status.v1.QueryTreesResponse
	3, // [3:5] is the sub-list for method output_type
	1, // [1:3] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_init() }
func file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_init() {
	if File_go_chromium_org_luci_tree_status_proto_v1_trees_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_rawDesc), len(file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_tree_status_proto_v1_trees_proto = out.File
	file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_goTypes = nil
	file_go_chromium_org_luci_tree_status_proto_v1_trees_proto_depIdxs = nil
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConnInterface

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion6

// TreesClient is the client API for Trees service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type TreesClient interface {
	// Get information of a tree.
	GetTree(ctx context.Context, in *GetTreeRequest, opts ...grpc.CallOption) (*Tree, error)
	// Query tree for a LUCI project.
	QueryTrees(ctx context.Context, in *QueryTreesRequest, opts ...grpc.CallOption) (*QueryTreesResponse, error)
}
type treesPRPCClient struct {
	client *prpc.Client
}

func NewTreesPRPCClient(client *prpc.Client) TreesClient {
	return &treesPRPCClient{client}
}

func (c *treesPRPCClient) GetTree(ctx context.Context, in *GetTreeRequest, opts ...grpc.CallOption) (*Tree, error) {
	out := new(Tree)
	err := c.client.Call(ctx, "luci.tree_status.v1.Trees", "GetTree", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *treesPRPCClient) QueryTrees(ctx context.Context, in *QueryTreesRequest, opts ...grpc.CallOption) (*QueryTreesResponse, error) {
	out := new(QueryTreesResponse)
	err := c.client.Call(ctx, "luci.tree_status.v1.Trees", "QueryTrees", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type treesClient struct {
	cc grpc.ClientConnInterface
}

func NewTreesClient(cc grpc.ClientConnInterface) TreesClient {
	return &treesClient{cc}
}

func (c *treesClient) GetTree(ctx context.Context, in *GetTreeRequest, opts ...grpc.CallOption) (*Tree, error) {
	out := new(Tree)
	err := c.cc.Invoke(ctx, "/luci.tree_status.v1.Trees/GetTree", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *treesClient) QueryTrees(ctx context.Context, in *QueryTreesRequest, opts ...grpc.CallOption) (*QueryTreesResponse, error) {
	out := new(QueryTreesResponse)
	err := c.cc.Invoke(ctx, "/luci.tree_status.v1.Trees/QueryTrees", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// TreesServer is the server API for Trees service.
type TreesServer interface {
	// Get information of a tree.
	GetTree(context.Context, *GetTreeRequest) (*Tree, error)
	// Query tree for a LUCI project.
	QueryTrees(context.Context, *QueryTreesRequest) (*QueryTreesResponse, error)
}

// UnimplementedTreesServer can be embedded to have forward compatible implementations.
type UnimplementedTreesServer struct {
}

func (*UnimplementedTreesServer) GetTree(context.Context, *GetTreeRequest) (*Tree, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetTree not implemented")
}
func (*UnimplementedTreesServer) QueryTrees(context.Context, *QueryTreesRequest) (*QueryTreesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method QueryTrees not implemented")
}

func RegisterTreesServer(s prpc.Registrar, srv TreesServer) {
	s.RegisterService(&_Trees_serviceDesc, srv)
}

func _Trees_GetTree_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetTreeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TreesServer).GetTree(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/luci.tree_status.v1.Trees/GetTree",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TreesServer).GetTree(ctx, req.(*GetTreeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Trees_QueryTrees_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(QueryTreesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TreesServer).QueryTrees(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/luci.tree_status.v1.Trees/QueryTrees",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TreesServer).QueryTrees(ctx, req.(*QueryTreesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _Trees_serviceDesc = grpc.ServiceDesc{
	ServiceName: "luci.tree_status.v1.Trees",
	HandlerType: (*TreesServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetTree",
			Handler:    _Trees_GetTree_Handler,
		},
		{
			MethodName: "QueryTrees",
			Handler:    _Trees_QueryTrees_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "go.chromium.org/luci/tree_status/proto/v1/trees.proto",
}
