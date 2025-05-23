// Copyright 2022 The LUCI Authors.
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
// source: go.chromium.org/luci/analysis/proto/v1/metrics.proto

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
	unsafe "unsafe"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// A request to list metrics in a given LUCI project.
type ListProjectMetricsRequest struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The parent LUCI Project, which owns the collection of metrics.
	// Format: projects/{project}.
	Parent        string `protobuf:"bytes,1,opt,name=parent,proto3" json:"parent,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ListProjectMetricsRequest) Reset() {
	*x = ListProjectMetricsRequest{}
	mi := &file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ListProjectMetricsRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ListProjectMetricsRequest) ProtoMessage() {}

func (x *ListProjectMetricsRequest) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ListProjectMetricsRequest.ProtoReflect.Descriptor instead.
func (*ListProjectMetricsRequest) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_rawDescGZIP(), []int{0}
}

func (x *ListProjectMetricsRequest) GetParent() string {
	if x != nil {
		return x.Parent
	}
	return ""
}

// Lists the metrics available in a LUCI Project.
// Designed to follow aip.dev/132.
type ListProjectMetricsResponse struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The metrics available in the LUCI Project.
	Metrics       []*ProjectMetric `protobuf:"bytes,1,rep,name=metrics,proto3" json:"metrics,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ListProjectMetricsResponse) Reset() {
	*x = ListProjectMetricsResponse{}
	mi := &file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ListProjectMetricsResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ListProjectMetricsResponse) ProtoMessage() {}

func (x *ListProjectMetricsResponse) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ListProjectMetricsResponse.ProtoReflect.Descriptor instead.
func (*ListProjectMetricsResponse) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_rawDescGZIP(), []int{1}
}

func (x *ListProjectMetricsResponse) GetMetrics() []*ProjectMetric {
	if x != nil {
		return x.Metrics
	}
	return nil
}

// A metric with LUCI project-specific configuration attached.
type ProjectMetric struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The resource name of the metric.
	// Format: projects/{project}/metrics/{metric_id}.
	// See aip.dev/122 for more.
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// The identifier of the metric.
	// Follows the pattern: ^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$.
	MetricId string `protobuf:"bytes,2,opt,name=metric_id,json=metricId,proto3" json:"metric_id,omitempty"`
	// A human readable name for the metric. E.g.
	// "User CLs Failed Presubmit".
	HumanReadableName string `protobuf:"bytes,3,opt,name=human_readable_name,json=humanReadableName,proto3" json:"human_readable_name,omitempty"`
	// A human readable description of the metric. Normally
	// this appears in a help popup near the metric.
	Description string `protobuf:"bytes,4,opt,name=description,proto3" json:"description,omitempty"`
	// Whether the metric should be shown by default in
	// the cluster listing and on cluster pages.
	IsDefault bool `protobuf:"varint,5,opt,name=is_default,json=isDefault,proto3" json:"is_default,omitempty"`
	// SortPriority defines the order by which metrics are sorted by default.
	// The metric with the highest sort priority will define the
	// (default) primary sort order, followed by the metric with the
	// second highest sort priority, and so on.
	// Each metric is guaranteed to have a unique sort priority.
	SortPriority  int32 `protobuf:"varint,6,opt,name=sort_priority,json=sortPriority,proto3" json:"sort_priority,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ProjectMetric) Reset() {
	*x = ProjectMetric{}
	mi := &file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ProjectMetric) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ProjectMetric) ProtoMessage() {}

func (x *ProjectMetric) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ProjectMetric.ProtoReflect.Descriptor instead.
func (*ProjectMetric) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_rawDescGZIP(), []int{2}
}

func (x *ProjectMetric) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *ProjectMetric) GetMetricId() string {
	if x != nil {
		return x.MetricId
	}
	return ""
}

func (x *ProjectMetric) GetHumanReadableName() string {
	if x != nil {
		return x.HumanReadableName
	}
	return ""
}

func (x *ProjectMetric) GetDescription() string {
	if x != nil {
		return x.Description
	}
	return ""
}

func (x *ProjectMetric) GetIsDefault() bool {
	if x != nil {
		return x.IsDefault
	}
	return false
}

func (x *ProjectMetric) GetSortPriority() int32 {
	if x != nil {
		return x.SortPriority
	}
	return 0
}

var File_go_chromium_org_luci_analysis_proto_v1_metrics_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_rawDesc = string([]byte{
	0x0a, 0x34, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2f,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x76, 0x31, 0x2f, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x73,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x10, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x61, 0x6e, 0x61,
	0x6c, 0x79, 0x73, 0x69, 0x73, 0x2e, 0x76, 0x31, 0x22, 0x33, 0x0a, 0x19, 0x4c, 0x69, 0x73, 0x74,
	0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x73, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x16, 0x0a, 0x06, 0x70, 0x61, 0x72, 0x65, 0x6e, 0x74, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x70, 0x61, 0x72, 0x65, 0x6e, 0x74, 0x22, 0x57, 0x0a,
	0x1a, 0x4c, 0x69, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x4d, 0x65, 0x74, 0x72,
	0x69, 0x63, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x39, 0x0a, 0x07, 0x6d,
	0x65, 0x74, 0x72, 0x69, 0x63, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x1f, 0x2e, 0x6c,
	0x75, 0x63, 0x69, 0x2e, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2e, 0x76, 0x31, 0x2e,
	0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x52, 0x07, 0x6d,
	0x65, 0x74, 0x72, 0x69, 0x63, 0x73, 0x22, 0xd6, 0x01, 0x0a, 0x0d, 0x50, 0x72, 0x6f, 0x6a, 0x65,
	0x63, 0x74, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x1b, 0x0a, 0x09,
	0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x08, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x49, 0x64, 0x12, 0x2e, 0x0a, 0x13, 0x68, 0x75, 0x6d,
	0x61, 0x6e, 0x5f, 0x72, 0x65, 0x61, 0x64, 0x61, 0x62, 0x6c, 0x65, 0x5f, 0x6e, 0x61, 0x6d, 0x65,
	0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x11, 0x68, 0x75, 0x6d, 0x61, 0x6e, 0x52, 0x65, 0x61,
	0x64, 0x61, 0x62, 0x6c, 0x65, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x20, 0x0a, 0x0b, 0x64, 0x65, 0x73,
	0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b,
	0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x1d, 0x0a, 0x0a, 0x69,
	0x73, 0x5f, 0x64, 0x65, 0x66, 0x61, 0x75, 0x6c, 0x74, 0x18, 0x05, 0x20, 0x01, 0x28, 0x08, 0x52,
	0x09, 0x69, 0x73, 0x44, 0x65, 0x66, 0x61, 0x75, 0x6c, 0x74, 0x12, 0x23, 0x0a, 0x0d, 0x73, 0x6f,
	0x72, 0x74, 0x5f, 0x70, 0x72, 0x69, 0x6f, 0x72, 0x69, 0x74, 0x79, 0x18, 0x06, 0x20, 0x01, 0x28,
	0x05, 0x52, 0x0c, 0x73, 0x6f, 0x72, 0x74, 0x50, 0x72, 0x69, 0x6f, 0x72, 0x69, 0x74, 0x79, 0x32,
	0x78, 0x0a, 0x07, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x73, 0x12, 0x6d, 0x0a, 0x0e, 0x4c, 0x69,
	0x73, 0x74, 0x46, 0x6f, 0x72, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x12, 0x2b, 0x2e, 0x6c,
	0x75, 0x63, 0x69, 0x2e, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2e, 0x76, 0x31, 0x2e,
	0x4c, 0x69, 0x73, 0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x4d, 0x65, 0x74, 0x72, 0x69,
	0x63, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x2c, 0x2e, 0x6c, 0x75, 0x63, 0x69,
	0x2e, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x4c, 0x69, 0x73,
	0x74, 0x50, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x73, 0x52,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x42, 0x33, 0x5a, 0x31, 0x67, 0x6f, 0x2e,
	0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63,
	0x69, 0x2f, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x2f, 0x76, 0x31, 0x3b, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x70, 0x62, 0x62, 0x06,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_rawDescData []byte
)

func file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_rawDesc), len(file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_rawDescData
}

var file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_goTypes = []any{
	(*ListProjectMetricsRequest)(nil),  // 0: luci.analysis.v1.ListProjectMetricsRequest
	(*ListProjectMetricsResponse)(nil), // 1: luci.analysis.v1.ListProjectMetricsResponse
	(*ProjectMetric)(nil),              // 2: luci.analysis.v1.ProjectMetric
}
var file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_depIdxs = []int32{
	2, // 0: luci.analysis.v1.ListProjectMetricsResponse.metrics:type_name -> luci.analysis.v1.ProjectMetric
	0, // 1: luci.analysis.v1.Metrics.ListForProject:input_type -> luci.analysis.v1.ListProjectMetricsRequest
	1, // 2: luci.analysis.v1.Metrics.ListForProject:output_type -> luci.analysis.v1.ListProjectMetricsResponse
	2, // [2:3] is the sub-list for method output_type
	1, // [1:2] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_init() }
func file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_init() {
	if File_go_chromium_org_luci_analysis_proto_v1_metrics_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_rawDesc), len(file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_analysis_proto_v1_metrics_proto = out.File
	file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_goTypes = nil
	file_go_chromium_org_luci_analysis_proto_v1_metrics_proto_depIdxs = nil
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConnInterface

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion6

// MetricsClient is the client API for Metrics service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type MetricsClient interface {
	// ListForProject lists metrics in a given LUCI Project.
	// Designed to follow aip.dev/132.
	ListForProject(ctx context.Context, in *ListProjectMetricsRequest, opts ...grpc.CallOption) (*ListProjectMetricsResponse, error)
}
type metricsPRPCClient struct {
	client *prpc.Client
}

func NewMetricsPRPCClient(client *prpc.Client) MetricsClient {
	return &metricsPRPCClient{client}
}

func (c *metricsPRPCClient) ListForProject(ctx context.Context, in *ListProjectMetricsRequest, opts ...grpc.CallOption) (*ListProjectMetricsResponse, error) {
	out := new(ListProjectMetricsResponse)
	err := c.client.Call(ctx, "luci.analysis.v1.Metrics", "ListForProject", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type metricsClient struct {
	cc grpc.ClientConnInterface
}

func NewMetricsClient(cc grpc.ClientConnInterface) MetricsClient {
	return &metricsClient{cc}
}

func (c *metricsClient) ListForProject(ctx context.Context, in *ListProjectMetricsRequest, opts ...grpc.CallOption) (*ListProjectMetricsResponse, error) {
	out := new(ListProjectMetricsResponse)
	err := c.cc.Invoke(ctx, "/luci.analysis.v1.Metrics/ListForProject", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// MetricsServer is the server API for Metrics service.
type MetricsServer interface {
	// ListForProject lists metrics in a given LUCI Project.
	// Designed to follow aip.dev/132.
	ListForProject(context.Context, *ListProjectMetricsRequest) (*ListProjectMetricsResponse, error)
}

// UnimplementedMetricsServer can be embedded to have forward compatible implementations.
type UnimplementedMetricsServer struct {
}

func (*UnimplementedMetricsServer) ListForProject(context.Context, *ListProjectMetricsRequest) (*ListProjectMetricsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListForProject not implemented")
}

func RegisterMetricsServer(s prpc.Registrar, srv MetricsServer) {
	s.RegisterService(&_Metrics_serviceDesc, srv)
}

func _Metrics_ListForProject_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListProjectMetricsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(MetricsServer).ListForProject(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/luci.analysis.v1.Metrics/ListForProject",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MetricsServer).ListForProject(ctx, req.(*ListProjectMetricsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _Metrics_serviceDesc = grpc.ServiceDesc{
	ServiceName: "luci.analysis.v1.Metrics",
	HandlerType: (*MetricsServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "ListForProject",
			Handler:    _Metrics_ListForProject_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "go.chromium.org/luci/analysis/proto/v1/metrics.proto",
}
