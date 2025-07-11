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
// 	protoc        v6.31.1
// source: go.chromium.org/luci/resultdb/proto/bq/text_artifact_row.proto

package resultpb

import (
	_ "go.chromium.org/luci/common/bq/pb"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
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

// TextArtifactRow represents a row in a BigQuery table `luci-resultdb.internal.text_artifacts`.
// Next ID: 22
type TextArtifactRow struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The LUCI project that the artifact belongs to (e.g. chromium).
	Project string `protobuf:"bytes,1,opt,name=project,proto3" json:"project,omitempty"`
	// The LUCI Realm the the artifact exists under.
	// Only contain the sub-realm (e.g. "try", instead of "chromium:try).
	// The project is stored in the project field.
	Realm string `protobuf:"bytes,2,opt,name=realm,proto3" json:"realm,omitempty"`
	// The invocation ID of the parent invocation.
	InvocationId string `protobuf:"bytes,3,opt,name=invocation_id,json=invocationId,proto3" json:"invocation_id,omitempty"`
	// The test that the artifact belongs to.
	// It will be empty if the artifact is an invocation-level artifact.
	TestId string `protobuf:"bytes,4,opt,name=test_id,json=testId,proto3" json:"test_id,omitempty"`
	// The result that the artifact belongs to.
	// It will be empty if the artifact is an invocation-level artifact.
	ResultId string `protobuf:"bytes,5,opt,name=result_id,json=resultId,proto3" json:"result_id,omitempty"`
	// Id of the artifact.
	// Refer to luci.resultdb.v1.Artifact.artifact_id for details.
	ArtifactId string `protobuf:"bytes,6,opt,name=artifact_id,json=artifactId,proto3" json:"artifact_id,omitempty"`
	// The number of shards needed to store this artifact.
	NumShards int32 `protobuf:"varint,7,opt,name=num_shards,json=numShards,proto3" json:"num_shards,omitempty"`
	// Id of the artifact shard.
	// Row size limit is 10MB according to
	// https://cloud.google.com/bigquery/quotas#write-api-limits.
	// The content itself will have a smaller limit because we will
	// have other data in the row and overhead.
	// If the size of the artifact content is larger than the limit, the data will be
	// sharded.
	//
	// When sharding, we try to keep the content size as close to the
	// limit as possible, but we will also prefer sharding at line-break
	// or white-space characters if such characters exist near the sharding
	// position (within 1KB). Sharding will never break a multi-byte Unicode
	// character.
	//
	// shard_id is monotonically increasing and starts at 0.
	ShardId int32 `protobuf:"varint,8,opt,name=shard_id,json=shardId,proto3" json:"shard_id,omitempty"`
	// Optional. Content type of the artifact (e.g. text/plain).
	ContentType string `protobuf:"bytes,9,opt,name=content_type,json=contentType,proto3" json:"content_type,omitempty"`
	// Artifact shard content.
	// Encoded as UTF-8.
	Content string `protobuf:"bytes,10,opt,name=content,proto3" json:"content,omitempty"`
	// Size of the artifact content in bytes.
	// This is the sum of shard_content_size of all shards of the artifact.
	ArtifactContentSize int32 `protobuf:"varint,11,opt,name=artifact_content_size,json=artifactContentSize,proto3" json:"artifact_content_size,omitempty"`
	// Size of the shard content in bytes.
	ShardContentSize int32 `protobuf:"varint,12,opt,name=shard_content_size,json=shardContentSize,proto3" json:"shard_content_size,omitempty"`
	// Partition_time is used to partition the table.
	// It is the time when the exported invocation was created in Spanner.
	// It is NOT the time when the row is inserted into BigQuery table.
	PartitionTime *timestamppb.Timestamp `protobuf:"bytes,13,opt,name=partition_time,json=partitionTime,proto3" json:"partition_time,omitempty"`
	// Status of the test result that contains the artifact.
	// See luci.resultdb.v1.TestStatus for possible values.
	// For invocation-level artifact, an this will be an empty string.
	TestStatus string `protobuf:"bytes,15,opt,name=test_status,json=testStatus,proto3" json:"test_status,omitempty"`
	// Status of the test result that contains the artifact (v2).
	// See luci.resultdb.v1.TestResult_Status for possible values.
	// For invocation-level artifact, an this will be an empty string.
	TestStatusV2 string `protobuf:"bytes,21,opt,name=test_status_v2,json=testStatusV2,proto3" json:"test_status_v2,omitempty"`
	// The variant of the test result that contains the artifact.
	// This will be encoded as a JSON object like
	// {"builder":"linux-rel","os":"Ubuntu-18.04",...}
	// to take advantage of BigQuery's JSON support, so that
	// the query will only be billed for the variant
	// keys it reads.
	//
	// In the protocol buffer, it must be a string as per
	// https://cloud.google.com/bigquery/docs/write-api#data_type_conversions
	// For invocation level artifact, this value will be set to
	// empty JSON string "{}".
	TestVariant string `protobuf:"bytes,16,opt,name=test_variant,json=testVariant,proto3" json:"test_variant,omitempty"`
	// A hash of the variant of the test result that contains the artifact.
	// This is encoded as lowercase hexadecimal characters.
	// The computation is an implementation detail of ResultDB.
	TestVariantHash string `protobuf:"bytes,17,opt,name=test_variant_hash,json=testVariantHash,proto3" json:"test_variant_hash,omitempty"`
	// Union of all variants of test results directly included by the invocation.
	// It will be an empty JSON string "{}", if the artifact is an test-result-level artifact.
	//
	// In the protocol buffer, it must be a string as per
	// https://cloud.google.com/bigquery/docs/write-api#data_type_conversions
	InvocationVariantUnion string `protobuf:"bytes,18,opt,name=invocation_variant_union,json=invocationVariantUnion,proto3" json:"invocation_variant_union,omitempty"`
	// A hash of the invocation_variant_union.
	// It will be empty if the artifact is an test-result-level artifact.
	InvocationVariantUnionHash string `protobuf:"bytes,19,opt,name=invocation_variant_union_hash,json=invocationVariantUnionHash,proto3" json:"invocation_variant_union_hash,omitempty"`
	// Insert_time is a rough estimation of when is row has been inserted to BigQuery using the server time.
	// This time does NOT provide ordering guarantee as spanner commit timestamp. This means
	// row with insert_time t0 might be visible after row with insert_time t1, given t0 < t1.
	// However it is relatively safe to assume that row with insert_time (t - 10 seconds) is visible
	// before row with insert_time t.
	InsertTime    *timestamppb.Timestamp `protobuf:"bytes,20,opt,name=insert_time,json=insertTime,proto3" json:"insert_time,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *TextArtifactRow) Reset() {
	*x = TextArtifactRow{}
	mi := &file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TextArtifactRow) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TextArtifactRow) ProtoMessage() {}

func (x *TextArtifactRow) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TextArtifactRow.ProtoReflect.Descriptor instead.
func (*TextArtifactRow) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_rawDescGZIP(), []int{0}
}

func (x *TextArtifactRow) GetProject() string {
	if x != nil {
		return x.Project
	}
	return ""
}

func (x *TextArtifactRow) GetRealm() string {
	if x != nil {
		return x.Realm
	}
	return ""
}

func (x *TextArtifactRow) GetInvocationId() string {
	if x != nil {
		return x.InvocationId
	}
	return ""
}

func (x *TextArtifactRow) GetTestId() string {
	if x != nil {
		return x.TestId
	}
	return ""
}

func (x *TextArtifactRow) GetResultId() string {
	if x != nil {
		return x.ResultId
	}
	return ""
}

func (x *TextArtifactRow) GetArtifactId() string {
	if x != nil {
		return x.ArtifactId
	}
	return ""
}

func (x *TextArtifactRow) GetNumShards() int32 {
	if x != nil {
		return x.NumShards
	}
	return 0
}

func (x *TextArtifactRow) GetShardId() int32 {
	if x != nil {
		return x.ShardId
	}
	return 0
}

func (x *TextArtifactRow) GetContentType() string {
	if x != nil {
		return x.ContentType
	}
	return ""
}

func (x *TextArtifactRow) GetContent() string {
	if x != nil {
		return x.Content
	}
	return ""
}

func (x *TextArtifactRow) GetArtifactContentSize() int32 {
	if x != nil {
		return x.ArtifactContentSize
	}
	return 0
}

func (x *TextArtifactRow) GetShardContentSize() int32 {
	if x != nil {
		return x.ShardContentSize
	}
	return 0
}

func (x *TextArtifactRow) GetPartitionTime() *timestamppb.Timestamp {
	if x != nil {
		return x.PartitionTime
	}
	return nil
}

func (x *TextArtifactRow) GetTestStatus() string {
	if x != nil {
		return x.TestStatus
	}
	return ""
}

func (x *TextArtifactRow) GetTestStatusV2() string {
	if x != nil {
		return x.TestStatusV2
	}
	return ""
}

func (x *TextArtifactRow) GetTestVariant() string {
	if x != nil {
		return x.TestVariant
	}
	return ""
}

func (x *TextArtifactRow) GetTestVariantHash() string {
	if x != nil {
		return x.TestVariantHash
	}
	return ""
}

func (x *TextArtifactRow) GetInvocationVariantUnion() string {
	if x != nil {
		return x.InvocationVariantUnion
	}
	return ""
}

func (x *TextArtifactRow) GetInvocationVariantUnionHash() string {
	if x != nil {
		return x.InvocationVariantUnionHash
	}
	return ""
}

func (x *TextArtifactRow) GetInsertTime() *timestamppb.Timestamp {
	if x != nil {
		return x.InsertTime
	}
	return nil
}

var File_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_rawDesc = string([]byte{
	0x0a, 0x3e, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2f,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x62, 0x71, 0x2f, 0x74, 0x65, 0x78, 0x74, 0x5f, 0x61, 0x72,
	0x74, 0x69, 0x66, 0x61, 0x63, 0x74, 0x5f, 0x72, 0x6f, 0x77, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x12, 0x10, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2e,
	0x62, 0x71, 0x1a, 0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x62, 0x75, 0x66, 0x2f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x1a, 0x2f, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d,
	0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e,
	0x2f, 0x62, 0x71, 0x2f, 0x70, 0x62, 0x2f, 0x6f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x22, 0xc7, 0x06, 0x0a, 0x0f, 0x54, 0x65, 0x78, 0x74, 0x41, 0x72, 0x74,
	0x69, 0x66, 0x61, 0x63, 0x74, 0x52, 0x6f, 0x77, 0x12, 0x18, 0x0a, 0x07, 0x70, 0x72, 0x6f, 0x6a,
	0x65, 0x63, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x70, 0x72, 0x6f, 0x6a, 0x65,
	0x63, 0x74, 0x12, 0x14, 0x0a, 0x05, 0x72, 0x65, 0x61, 0x6c, 0x6d, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x05, 0x72, 0x65, 0x61, 0x6c, 0x6d, 0x12, 0x23, 0x0a, 0x0d, 0x69, 0x6e, 0x76, 0x6f,
	0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x69, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x0c, 0x69, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x49, 0x64, 0x12, 0x17, 0x0a,
	0x07, 0x74, 0x65, 0x73, 0x74, 0x5f, 0x69, 0x64, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06,
	0x74, 0x65, 0x73, 0x74, 0x49, 0x64, 0x12, 0x1b, 0x0a, 0x09, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74,
	0x5f, 0x69, 0x64, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x72, 0x65, 0x73, 0x75, 0x6c,
	0x74, 0x49, 0x64, 0x12, 0x1f, 0x0a, 0x0b, 0x61, 0x72, 0x74, 0x69, 0x66, 0x61, 0x63, 0x74, 0x5f,
	0x69, 0x64, 0x18, 0x06, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x61, 0x72, 0x74, 0x69, 0x66, 0x61,
	0x63, 0x74, 0x49, 0x64, 0x12, 0x1d, 0x0a, 0x0a, 0x6e, 0x75, 0x6d, 0x5f, 0x73, 0x68, 0x61, 0x72,
	0x64, 0x73, 0x18, 0x07, 0x20, 0x01, 0x28, 0x05, 0x52, 0x09, 0x6e, 0x75, 0x6d, 0x53, 0x68, 0x61,
	0x72, 0x64, 0x73, 0x12, 0x19, 0x0a, 0x08, 0x73, 0x68, 0x61, 0x72, 0x64, 0x5f, 0x69, 0x64, 0x18,
	0x08, 0x20, 0x01, 0x28, 0x05, 0x52, 0x07, 0x73, 0x68, 0x61, 0x72, 0x64, 0x49, 0x64, 0x12, 0x21,
	0x0a, 0x0c, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x5f, 0x74, 0x79, 0x70, 0x65, 0x18, 0x09,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x0b, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x54, 0x79, 0x70,
	0x65, 0x12, 0x18, 0x0a, 0x07, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x18, 0x0a, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x07, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x12, 0x32, 0x0a, 0x15, 0x61,
	0x72, 0x74, 0x69, 0x66, 0x61, 0x63, 0x74, 0x5f, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x5f,
	0x73, 0x69, 0x7a, 0x65, 0x18, 0x0b, 0x20, 0x01, 0x28, 0x05, 0x52, 0x13, 0x61, 0x72, 0x74, 0x69,
	0x66, 0x61, 0x63, 0x74, 0x43, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x53, 0x69, 0x7a, 0x65, 0x12,
	0x2c, 0x0a, 0x12, 0x73, 0x68, 0x61, 0x72, 0x64, 0x5f, 0x63, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74,
	0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x0c, 0x20, 0x01, 0x28, 0x05, 0x52, 0x10, 0x73, 0x68, 0x61,
	0x72, 0x64, 0x43, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x41, 0x0a,
	0x0e, 0x70, 0x61, 0x72, 0x74, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x74, 0x69, 0x6d, 0x65, 0x18,
	0x0d, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d,
	0x70, 0x52, 0x0d, 0x70, 0x61, 0x72, 0x74, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x54, 0x69, 0x6d, 0x65,
	0x12, 0x1f, 0x0a, 0x0b, 0x74, 0x65, 0x73, 0x74, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x18,
	0x0f, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x74, 0x65, 0x73, 0x74, 0x53, 0x74, 0x61, 0x74, 0x75,
	0x73, 0x12, 0x24, 0x0a, 0x0e, 0x74, 0x65, 0x73, 0x74, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73,
	0x5f, 0x76, 0x32, 0x18, 0x15, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0c, 0x74, 0x65, 0x73, 0x74, 0x53,
	0x74, 0x61, 0x74, 0x75, 0x73, 0x56, 0x32, 0x12, 0x2d, 0x0a, 0x0c, 0x74, 0x65, 0x73, 0x74, 0x5f,
	0x76, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x18, 0x10, 0x20, 0x01, 0x28, 0x09, 0x42, 0x0a, 0xe2,
	0xbc, 0x24, 0x06, 0x0a, 0x04, 0x4a, 0x53, 0x4f, 0x4e, 0x52, 0x0b, 0x74, 0x65, 0x73, 0x74, 0x56,
	0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x12, 0x2a, 0x0a, 0x11, 0x74, 0x65, 0x73, 0x74, 0x5f, 0x76,
	0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x5f, 0x68, 0x61, 0x73, 0x68, 0x18, 0x11, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x0f, 0x74, 0x65, 0x73, 0x74, 0x56, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x48, 0x61,
	0x73, 0x68, 0x12, 0x44, 0x0a, 0x18, 0x69, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e,
	0x5f, 0x76, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x5f, 0x75, 0x6e, 0x69, 0x6f, 0x6e, 0x18, 0x12,
	0x20, 0x01, 0x28, 0x09, 0x42, 0x0a, 0xe2, 0xbc, 0x24, 0x06, 0x0a, 0x04, 0x4a, 0x53, 0x4f, 0x4e,
	0x52, 0x16, 0x69, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x56, 0x61, 0x72, 0x69,
	0x61, 0x6e, 0x74, 0x55, 0x6e, 0x69, 0x6f, 0x6e, 0x12, 0x41, 0x0a, 0x1d, 0x69, 0x6e, 0x76, 0x6f,
	0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x76, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x5f, 0x75,
	0x6e, 0x69, 0x6f, 0x6e, 0x5f, 0x68, 0x61, 0x73, 0x68, 0x18, 0x13, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x1a, 0x69, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x56, 0x61, 0x72, 0x69, 0x61,
	0x6e, 0x74, 0x55, 0x6e, 0x69, 0x6f, 0x6e, 0x48, 0x61, 0x73, 0x68, 0x12, 0x3b, 0x0a, 0x0b, 0x69,
	0x6e, 0x73, 0x65, 0x72, 0x74, 0x5f, 0x74, 0x69, 0x6d, 0x65, 0x18, 0x14, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62,
	0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x52, 0x0a, 0x69, 0x6e,
	0x73, 0x65, 0x72, 0x74, 0x54, 0x69, 0x6d, 0x65, 0x4a, 0x04, 0x08, 0x0e, 0x10, 0x0f, 0x42, 0x31,
	0x5a, 0x2f, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2f,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x62, 0x71, 0x3b, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x70,
	0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_rawDescData []byte
)

func file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_rawDesc), len(file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_rawDescData
}

var file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_goTypes = []any{
	(*TextArtifactRow)(nil),       // 0: luci.resultdb.bq.TextArtifactRow
	(*timestamppb.Timestamp)(nil), // 1: google.protobuf.Timestamp
}
var file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_depIdxs = []int32{
	1, // 0: luci.resultdb.bq.TextArtifactRow.partition_time:type_name -> google.protobuf.Timestamp
	1, // 1: luci.resultdb.bq.TextArtifactRow.insert_time:type_name -> google.protobuf.Timestamp
	2, // [2:2] is the sub-list for method output_type
	2, // [2:2] is the sub-list for method input_type
	2, // [2:2] is the sub-list for extension type_name
	2, // [2:2] is the sub-list for extension extendee
	0, // [0:2] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_init() }
func file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_init() {
	if File_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_rawDesc), len(file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto = out.File
	file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_goTypes = nil
	file_go_chromium_org_luci_resultdb_proto_bq_text_artifact_row_proto_depIdxs = nil
}
