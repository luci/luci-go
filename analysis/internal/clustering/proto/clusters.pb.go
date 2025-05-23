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
// source: go.chromium.org/luci/analysis/internal/clustering/proto/clusters.proto

package clusteringpb

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

// Represents the clusters a chunk of test results are included in.
type ChunkClusters struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The types of clusters in this proto.
	ClusterTypes []*ClusterType `protobuf:"bytes,1,rep,name=cluster_types,json=clusterTypes,proto3" json:"cluster_types,omitempty"`
	// The identifiers of the clusters referenced in this proto.
	ReferencedClusters []*ReferencedCluster `protobuf:"bytes,2,rep,name=referenced_clusters,json=referencedClusters,proto3" json:"referenced_clusters,omitempty"`
	// The clusters of test results in the chunk. This is a list, so the first
	// TestResultClusters message is for first test result in the chunk,
	// the second message is for the second test result, and so on.
	ResultClusters []*TestResultClusters `protobuf:"bytes,3,rep,name=result_clusters,json=resultClusters,proto3" json:"result_clusters,omitempty"`
	unknownFields  protoimpl.UnknownFields
	sizeCache      protoimpl.SizeCache
}

func (x *ChunkClusters) Reset() {
	*x = ChunkClusters{}
	mi := &file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ChunkClusters) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ChunkClusters) ProtoMessage() {}

func (x *ChunkClusters) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ChunkClusters.ProtoReflect.Descriptor instead.
func (*ChunkClusters) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_rawDescGZIP(), []int{0}
}

func (x *ChunkClusters) GetClusterTypes() []*ClusterType {
	if x != nil {
		return x.ClusterTypes
	}
	return nil
}

func (x *ChunkClusters) GetReferencedClusters() []*ReferencedCluster {
	if x != nil {
		return x.ReferencedClusters
	}
	return nil
}

func (x *ChunkClusters) GetResultClusters() []*TestResultClusters {
	if x != nil {
		return x.ResultClusters
	}
	return nil
}

// Defines a type of cluster.
type ClusterType struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The algorithm used to create the cluster, e.g. "reason-0.1" for reason-based
	// clustering or "rule-0.1" for clusters based on failure association rules.
	// If specific algorithm versions are deprecated, this will allow us to target
	// cluster references for deletion.
	Algorithm     string `protobuf:"bytes,1,opt,name=algorithm,proto3" json:"algorithm,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ClusterType) Reset() {
	*x = ClusterType{}
	mi := &file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ClusterType) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ClusterType) ProtoMessage() {}

func (x *ClusterType) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ClusterType.ProtoReflect.Descriptor instead.
func (*ClusterType) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_rawDescGZIP(), []int{1}
}

func (x *ClusterType) GetAlgorithm() string {
	if x != nil {
		return x.Algorithm
	}
	return ""
}

// Represents a reference to a cluster.
type ReferencedCluster struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The type of the referenced cluster, represented by an index
	// into the cluster_types list of ChunkClusters.
	TypeRef int64 `protobuf:"varint,1,opt,name=type_ref,json=typeRef,proto3" json:"type_ref,omitempty"`
	// The identifier of the referenced cluster (up to 16 bytes).
	ClusterId     []byte `protobuf:"bytes,2,opt,name=cluster_id,json=clusterId,proto3" json:"cluster_id,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ReferencedCluster) Reset() {
	*x = ReferencedCluster{}
	mi := &file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ReferencedCluster) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ReferencedCluster) ProtoMessage() {}

func (x *ReferencedCluster) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ReferencedCluster.ProtoReflect.Descriptor instead.
func (*ReferencedCluster) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_rawDescGZIP(), []int{2}
}

func (x *ReferencedCluster) GetTypeRef() int64 {
	if x != nil {
		return x.TypeRef
	}
	return 0
}

func (x *ReferencedCluster) GetClusterId() []byte {
	if x != nil {
		return x.ClusterId
	}
	return nil
}

// Represents the clusters a test result is included in.
type TestResultClusters struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The clusters the test result is a member of. Clusters are identified by
	// their index in the referenced_clusters list.
	ClusterRefs   []int64 `protobuf:"varint,1,rep,packed,name=cluster_refs,json=clusterRefs,proto3" json:"cluster_refs,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TestResultClusters) Reset() {
	*x = TestResultClusters{}
	mi := &file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TestResultClusters) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TestResultClusters) ProtoMessage() {}

func (x *TestResultClusters) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TestResultClusters.ProtoReflect.Descriptor instead.
func (*TestResultClusters) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_rawDescGZIP(), []int{3}
}

func (x *TestResultClusters) GetClusterRefs() []int64 {
	if x != nil {
		return x.ClusterRefs
	}
	return nil
}

var File_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_rawDesc = string([]byte{
	0x0a, 0x46, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2f,
	0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72,
	0x69, 0x6e, 0x67, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65,
	0x72, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x21, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x61,
	0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c,
	0x2e, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x69, 0x6e, 0x67, 0x22, 0xab, 0x02, 0x0a, 0x0d,
	0x43, 0x68, 0x75, 0x6e, 0x6b, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x73, 0x12, 0x53, 0x0a,
	0x0d, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x5f, 0x74, 0x79, 0x70, 0x65, 0x73, 0x18, 0x01,
	0x20, 0x03, 0x28, 0x0b, 0x32, 0x2e, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x61, 0x6e, 0x61, 0x6c,
	0x79, 0x73, 0x69, 0x73, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x63, 0x6c,
	0x75, 0x73, 0x74, 0x65, 0x72, 0x69, 0x6e, 0x67, 0x2e, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72,
	0x54, 0x79, 0x70, 0x65, 0x52, 0x0c, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x54, 0x79, 0x70,
	0x65, 0x73, 0x12, 0x65, 0x0a, 0x13, 0x72, 0x65, 0x66, 0x65, 0x72, 0x65, 0x6e, 0x63, 0x65, 0x64,
	0x5f, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28, 0x0b, 0x32,
	0x34, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2e,
	0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72,
	0x69, 0x6e, 0x67, 0x2e, 0x52, 0x65, 0x66, 0x65, 0x72, 0x65, 0x6e, 0x63, 0x65, 0x64, 0x43, 0x6c,
	0x75, 0x73, 0x74, 0x65, 0x72, 0x52, 0x12, 0x72, 0x65, 0x66, 0x65, 0x72, 0x65, 0x6e, 0x63, 0x65,
	0x64, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x73, 0x12, 0x5e, 0x0a, 0x0f, 0x72, 0x65, 0x73,
	0x75, 0x6c, 0x74, 0x5f, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x73, 0x18, 0x03, 0x20, 0x03,
	0x28, 0x0b, 0x32, 0x35, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73,
	0x69, 0x73, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x63, 0x6c, 0x75, 0x73,
	0x74, 0x65, 0x72, 0x69, 0x6e, 0x67, 0x2e, 0x54, 0x65, 0x73, 0x74, 0x52, 0x65, 0x73, 0x75, 0x6c,
	0x74, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x73, 0x52, 0x0e, 0x72, 0x65, 0x73, 0x75, 0x6c,
	0x74, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x73, 0x22, 0x2b, 0x0a, 0x0b, 0x43, 0x6c, 0x75,
	0x73, 0x74, 0x65, 0x72, 0x54, 0x79, 0x70, 0x65, 0x12, 0x1c, 0x0a, 0x09, 0x61, 0x6c, 0x67, 0x6f,
	0x72, 0x69, 0x74, 0x68, 0x6d, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x61, 0x6c, 0x67,
	0x6f, 0x72, 0x69, 0x74, 0x68, 0x6d, 0x22, 0x4d, 0x0a, 0x11, 0x52, 0x65, 0x66, 0x65, 0x72, 0x65,
	0x6e, 0x63, 0x65, 0x64, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x12, 0x19, 0x0a, 0x08, 0x74,
	0x79, 0x70, 0x65, 0x5f, 0x72, 0x65, 0x66, 0x18, 0x01, 0x20, 0x01, 0x28, 0x03, 0x52, 0x07, 0x74,
	0x79, 0x70, 0x65, 0x52, 0x65, 0x66, 0x12, 0x1d, 0x0a, 0x0a, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65,
	0x72, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x09, 0x63, 0x6c, 0x75, 0x73,
	0x74, 0x65, 0x72, 0x49, 0x64, 0x22, 0x3b, 0x0a, 0x12, 0x54, 0x65, 0x73, 0x74, 0x52, 0x65, 0x73,
	0x75, 0x6c, 0x74, 0x43, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x73, 0x12, 0x25, 0x0a, 0x0c, 0x63,
	0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x5f, 0x72, 0x65, 0x66, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28,
	0x03, 0x42, 0x02, 0x10, 0x01, 0x52, 0x0b, 0x63, 0x6c, 0x75, 0x73, 0x74, 0x65, 0x72, 0x52, 0x65,
	0x66, 0x73, 0x42, 0x46, 0x5a, 0x44, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75,
	0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x61, 0x6e, 0x61, 0x6c, 0x79,
	0x73, 0x69, 0x73, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x63, 0x6c, 0x75,
	0x73, 0x74, 0x65, 0x72, 0x69, 0x6e, 0x67, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x3b, 0x63, 0x6c,
	0x75, 0x73, 0x74, 0x65, 0x72, 0x69, 0x6e, 0x67, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_rawDescData []byte
)

func file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_rawDesc), len(file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_rawDescData
}

var file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_goTypes = []any{
	(*ChunkClusters)(nil),      // 0: luci.analysis.internal.clustering.ChunkClusters
	(*ClusterType)(nil),        // 1: luci.analysis.internal.clustering.ClusterType
	(*ReferencedCluster)(nil),  // 2: luci.analysis.internal.clustering.ReferencedCluster
	(*TestResultClusters)(nil), // 3: luci.analysis.internal.clustering.TestResultClusters
}
var file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_depIdxs = []int32{
	1, // 0: luci.analysis.internal.clustering.ChunkClusters.cluster_types:type_name -> luci.analysis.internal.clustering.ClusterType
	2, // 1: luci.analysis.internal.clustering.ChunkClusters.referenced_clusters:type_name -> luci.analysis.internal.clustering.ReferencedCluster
	3, // 2: luci.analysis.internal.clustering.ChunkClusters.result_clusters:type_name -> luci.analysis.internal.clustering.TestResultClusters
	3, // [3:3] is the sub-list for method output_type
	3, // [3:3] is the sub-list for method input_type
	3, // [3:3] is the sub-list for extension type_name
	3, // [3:3] is the sub-list for extension extendee
	0, // [0:3] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_init() }
func file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_init() {
	if File_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_rawDesc), len(file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto = out.File
	file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_goTypes = nil
	file_go_chromium_org_luci_analysis_internal_clustering_proto_clusters_proto_depIdxs = nil
}
