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
// 	protoc-gen-go v1.33.0
// 	protoc        v5.26.1
// source: go.chromium.org/luci/analysis/proto/bq/test_result_row.proto

package bqpb

import (
	v1 "go.chromium.org/luci/analysis/proto/v1"
	_ "go.chromium.org/luci/common/bq/pb"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// Represents a test result exported to BigQuery.
//
// A test result is the outcome of a single execution of a test variant
// (a way of running a test) in an invocation (a container of test
// results, such as a build).
//
// BigQuery tables using this schema will use the following settings:
//   - Partition by TIMESTAMP_TRUNC(partition_time, DAY),
//     retain data for 510 days.
//   - Cluster by project, test_id.
//
// NextId: 23
type TestResultRow struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// The LUCI Project. E.g. "chromium".
	Project string `protobuf:"bytes,1,opt,name=project,proto3" json:"project,omitempty"`
	// Is a unique identifier of the test in a LUCI project.
	// Refer to TestResult.test_id for details.
	TestId string `protobuf:"bytes,2,opt,name=test_id,json=testId,proto3" json:"test_id,omitempty"`
	// Describes one specific way of running the test,
	// e.g. a specific bucket, builder and a test suite.
	//
	// This will be encoded as a JSON object like
	// {"builder":"linux-rel","os":"Ubuntu-18.04",...}
	// to take advantage of BigQuery's JSON support, so that
	// the query will only be billed for the variant
	// keys it reads.
	//
	// In the protocol buffer, it must be a string as per
	// https://cloud.google.com/bigquery/docs/write-api#data_type_conversions
	Variant string `protobuf:"bytes,3,opt,name=variant,proto3" json:"variant,omitempty"`
	// A hash of the variant, encoded as lowercase hexadecimal characters.
	// The computation is an implementation detail of ResultDB.
	VariantHash string `protobuf:"bytes,4,opt,name=variant_hash,json=variantHash,proto3" json:"variant_hash,omitempty"`
	// Invocation is the ResultDB invocation marked is_export_root
	// that the test result is being exported under.
	//
	// Note: The test result may not have been directly uploaded to
	// this invocation, but rather one of its included invocations.
	// See `parent`.
	Invocation *TestResultRow_InvocationRecord `protobuf:"bytes,5,opt,name=invocation,proto3" json:"invocation,omitempty"`
	// Partition_time is used to partition the table.
	// It is the time when exported invocation was created in Spanner.
	// Note: it is NOT the time when the row is inserted into BigQuery table.
	// https://cloud.google.com/bigquery/docs/creating-column-partitions#limitations
	// mentions "The partitioning column must be a top-level field."
	// So we keep this column here instead of adding the CreateTime to InvocationRecord.
	PartitionTime *timestamppb.Timestamp `protobuf:"bytes,6,opt,name=partition_time,json=partitionTime,proto3" json:"partition_time,omitempty"`
	// Parent contains info of the result's immediate parent invocation.
	Parent *TestResultRow_ParentInvocationRecord `protobuf:"bytes,7,opt,name=parent,proto3" json:"parent,omitempty"`
	// The global identifier of a test result in ResultDB.
	// Format:
	// "invocations/{INVOCATION_ID}/tests/{URL_ESCAPED_TEST_ID}/results/{RESULT_ID}".
	Name string `protobuf:"bytes,8,opt,name=name,proto3" json:"name,omitempty"`
	// Identifies a test result in a given invocation and test id.
	ResultId string `protobuf:"bytes,9,opt,name=result_id,json=resultId,proto3" json:"result_id,omitempty"`
	// Expected is a flag indicating whether the result of test case execution is
	// expected. Refer to TestResult.Expected for details.
	Expected bool `protobuf:"varint,10,opt,name=expected,proto3" json:"expected,omitempty"`
	// Status of the test result.
	Status v1.TestResultStatus `protobuf:"varint,11,opt,name=status,proto3,enum=luci.analysis.v1.TestResultStatus" json:"status,omitempty"`
	// A human-readable explanation of the result, in HTML.
	// MUST be sanitized before rendering in the browser.
	SummaryHtml string `protobuf:"bytes,12,opt,name=summary_html,json=summaryHtml,proto3" json:"summary_html,omitempty"`
	// The point in time when the test case started to execute.
	StartTime *timestamppb.Timestamp `protobuf:"bytes,13,opt,name=start_time,json=startTime,proto3" json:"start_time,omitempty"`
	// Duration of the test case execution in seconds.
	DurationSecs float64 `protobuf:"fixed64,14,opt,name=duration_secs,json=durationSecs,proto3" json:"duration_secs,omitempty"`
	// Tags contains metadata for this test result.
	// It might describe this particular execution or the test case.
	Tags []*v1.StringPair `protobuf:"bytes,15,rep,name=tags,proto3" json:"tags,omitempty"`
	// Information about failed tests.
	// e.g. the assertion failure message.
	FailureReason *v1.FailureReason `protobuf:"bytes,16,opt,name=failure_reason,json=failureReason,proto3" json:"failure_reason,omitempty"`
	// Reasoning behind a test skip, in machine-readable form.
	// Only set when status is SKIP.
	// It is the string representation of luci.analysis.v1.SkipReason when
	// specified and "" when the skip reason is unspecified.
	SkipReason string `protobuf:"bytes,17,opt,name=skip_reason,json=skipReason,proto3" json:"skip_reason,omitempty"`
	// Arbitrary JSON object that contains structured, domain-specific properties
	// of the test result. Stored here stringified as this is the only protocol
	// buffer type that maps to the JSON BigQuery type:
	// https://cloud.google.com/bigquery/docs/write-api#data_type_conversions
	Properties string `protobuf:"bytes,18,opt,name=properties,proto3" json:"properties,omitempty"`
	// The code sources tested. Obtained from one of the verdict's test results.
	// If the invocation which contained the test result
	// specified that code sources directly, this is those sources.
	// If the code sources were marked as are inherited from the including
	// invocation, this is the resolved code sources (if they could be resolved).
	// Unset otherwise.
	Sources *v1.Sources `protobuf:"bytes,19,opt,name=sources,proto3" json:"sources,omitempty"`
	// The branch in source control that was tested, if known.
	// For example, the `refs/heads/main` branch in the `chromium/src` repo
	// hosted by `chromium.googlesource.com`.
	// This is a subset of the information in the `sources` field.
	SourceRef *v1.SourceRef `protobuf:"bytes,20,opt,name=source_ref,json=sourceRef,proto3" json:"source_ref,omitempty"`
	// Hash of the source_ref field, as 16 lowercase hexadecimal characters.
	// Can be used to uniquely identify a branch in a source code
	// version control system.
	SourceRefHash string `protobuf:"bytes,21,opt,name=source_ref_hash,json=sourceRefHash,proto3" json:"source_ref_hash,omitempty"`
	// Metadata of the test case,
	// e.g. the original test name and test location.
	TestMetadata *v1.TestMetadata `protobuf:"bytes,22,opt,name=test_metadata,json=testMetadata,proto3" json:"test_metadata,omitempty"`
}

func (x *TestResultRow) Reset() {
	*x = TestResultRow{}
	if protoimpl.UnsafeEnabled {
		mi := &file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *TestResultRow) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TestResultRow) ProtoMessage() {}

func (x *TestResultRow) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TestResultRow.ProtoReflect.Descriptor instead.
func (*TestResultRow) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_rawDescGZIP(), []int{0}
}

func (x *TestResultRow) GetProject() string {
	if x != nil {
		return x.Project
	}
	return ""
}

func (x *TestResultRow) GetTestId() string {
	if x != nil {
		return x.TestId
	}
	return ""
}

func (x *TestResultRow) GetVariant() string {
	if x != nil {
		return x.Variant
	}
	return ""
}

func (x *TestResultRow) GetVariantHash() string {
	if x != nil {
		return x.VariantHash
	}
	return ""
}

func (x *TestResultRow) GetInvocation() *TestResultRow_InvocationRecord {
	if x != nil {
		return x.Invocation
	}
	return nil
}

func (x *TestResultRow) GetPartitionTime() *timestamppb.Timestamp {
	if x != nil {
		return x.PartitionTime
	}
	return nil
}

func (x *TestResultRow) GetParent() *TestResultRow_ParentInvocationRecord {
	if x != nil {
		return x.Parent
	}
	return nil
}

func (x *TestResultRow) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *TestResultRow) GetResultId() string {
	if x != nil {
		return x.ResultId
	}
	return ""
}

func (x *TestResultRow) GetExpected() bool {
	if x != nil {
		return x.Expected
	}
	return false
}

func (x *TestResultRow) GetStatus() v1.TestResultStatus {
	if x != nil {
		return x.Status
	}
	return v1.TestResultStatus(0)
}

func (x *TestResultRow) GetSummaryHtml() string {
	if x != nil {
		return x.SummaryHtml
	}
	return ""
}

func (x *TestResultRow) GetStartTime() *timestamppb.Timestamp {
	if x != nil {
		return x.StartTime
	}
	return nil
}

func (x *TestResultRow) GetDurationSecs() float64 {
	if x != nil {
		return x.DurationSecs
	}
	return 0
}

func (x *TestResultRow) GetTags() []*v1.StringPair {
	if x != nil {
		return x.Tags
	}
	return nil
}

func (x *TestResultRow) GetFailureReason() *v1.FailureReason {
	if x != nil {
		return x.FailureReason
	}
	return nil
}

func (x *TestResultRow) GetSkipReason() string {
	if x != nil {
		return x.SkipReason
	}
	return ""
}

func (x *TestResultRow) GetProperties() string {
	if x != nil {
		return x.Properties
	}
	return ""
}

func (x *TestResultRow) GetSources() *v1.Sources {
	if x != nil {
		return x.Sources
	}
	return nil
}

func (x *TestResultRow) GetSourceRef() *v1.SourceRef {
	if x != nil {
		return x.SourceRef
	}
	return nil
}

func (x *TestResultRow) GetSourceRefHash() string {
	if x != nil {
		return x.SourceRefHash
	}
	return ""
}

func (x *TestResultRow) GetTestMetadata() *v1.TestMetadata {
	if x != nil {
		return x.TestMetadata
	}
	return nil
}

type TestResultRow_InvocationRecord struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// The ID of the invocation.
	Id string `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	// The LUCI Realm the invocation exists under.
	// For example, "chromium:try".
	Realm string `protobuf:"bytes,2,opt,name=realm,proto3" json:"realm,omitempty"`
}

func (x *TestResultRow_InvocationRecord) Reset() {
	*x = TestResultRow_InvocationRecord{}
	if protoimpl.UnsafeEnabled {
		mi := &file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *TestResultRow_InvocationRecord) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TestResultRow_InvocationRecord) ProtoMessage() {}

func (x *TestResultRow_InvocationRecord) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TestResultRow_InvocationRecord.ProtoReflect.Descriptor instead.
func (*TestResultRow_InvocationRecord) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_rawDescGZIP(), []int{0, 0}
}

func (x *TestResultRow_InvocationRecord) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *TestResultRow_InvocationRecord) GetRealm() string {
	if x != nil {
		return x.Realm
	}
	return ""
}

// ParentInvocationRecord for a test result is the immediate parent invocation
// that directly contains the test result.
type TestResultRow_ParentInvocationRecord struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// The ID of the invocation.
	Id string `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	// Tags represents Invocation-level string key-value pairs.
	// A key can be repeated.
	Tags []*v1.StringPair `protobuf:"bytes,2,rep,name=tags,proto3" json:"tags,omitempty"`
	// The LUCI Realm the invocation exists under.
	// For example, "chromium:try".
	Realm string `protobuf:"bytes,3,opt,name=realm,proto3" json:"realm,omitempty"`
	// Arbitrary JSON object that contains structured, domain-specific properties
	// of the invocation. Stored here stringified as this is the only protocol
	// buffer type that maps to the JSON BigQuery type:
	// https://cloud.google.com/bigquery/docs/write-api#data_type_conversions
	Properties string `protobuf:"bytes,4,opt,name=properties,proto3" json:"properties,omitempty"`
}

func (x *TestResultRow_ParentInvocationRecord) Reset() {
	*x = TestResultRow_ParentInvocationRecord{}
	if protoimpl.UnsafeEnabled {
		mi := &file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *TestResultRow_ParentInvocationRecord) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TestResultRow_ParentInvocationRecord) ProtoMessage() {}

func (x *TestResultRow_ParentInvocationRecord) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TestResultRow_ParentInvocationRecord.ProtoReflect.Descriptor instead.
func (*TestResultRow_ParentInvocationRecord) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_rawDescGZIP(), []int{0, 1}
}

func (x *TestResultRow_ParentInvocationRecord) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *TestResultRow_ParentInvocationRecord) GetTags() []*v1.StringPair {
	if x != nil {
		return x.Tags
	}
	return nil
}

func (x *TestResultRow_ParentInvocationRecord) GetRealm() string {
	if x != nil {
		return x.Realm
	}
	return ""
}

func (x *TestResultRow_ParentInvocationRecord) GetProperties() string {
	if x != nil {
		return x.Properties
	}
	return ""
}

var File_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_rawDesc = []byte{
	0x0a, 0x3c, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2f,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x62, 0x71, 0x2f, 0x74, 0x65, 0x73, 0x74, 0x5f, 0x72, 0x65,
	0x73, 0x75, 0x6c, 0x74, 0x5f, 0x72, 0x6f, 0x77, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x10,
	0x6c, 0x75, 0x63, 0x69, 0x2e, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2e, 0x62, 0x71,
	0x1a, 0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75,
	0x66, 0x2f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x1a, 0x33, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f,
	0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73,
	0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x76, 0x31, 0x2f, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x3b, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d,
	0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x61, 0x6e, 0x61,
	0x6c, 0x79, 0x73, 0x69, 0x73, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x76, 0x31, 0x2f, 0x66,
	0x61, 0x69, 0x6c, 0x75, 0x72, 0x65, 0x5f, 0x72, 0x65, 0x61, 0x73, 0x6f, 0x6e, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x1a, 0x34, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d,
	0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73,
	0x69, 0x73, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x76, 0x31, 0x2f, 0x73, 0x6f, 0x75, 0x72,
	0x63, 0x65, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x3a, 0x67, 0x6f, 0x2e, 0x63, 0x68,
	0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f,
	0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x76,
	0x31, 0x2f, 0x74, 0x65, 0x73, 0x74, 0x5f, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x39, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69,
	0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x61, 0x6e, 0x61, 0x6c,
	0x79, 0x73, 0x69, 0x73, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x76, 0x31, 0x2f, 0x74, 0x65,
	0x73, 0x74, 0x5f, 0x76, 0x65, 0x72, 0x64, 0x69, 0x63, 0x74, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x1a, 0x2f, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x2f, 0x62, 0x71,
	0x2f, 0x70, 0x62, 0x2f, 0x6f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x22, 0xfa, 0x09, 0x0a, 0x0d, 0x54, 0x65, 0x73, 0x74, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74,
	0x52, 0x6f, 0x77, 0x12, 0x18, 0x0a, 0x07, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x12, 0x17, 0x0a,
	0x07, 0x74, 0x65, 0x73, 0x74, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06,
	0x74, 0x65, 0x73, 0x74, 0x49, 0x64, 0x12, 0x24, 0x0a, 0x07, 0x76, 0x61, 0x72, 0x69, 0x61, 0x6e,
	0x74, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x42, 0x0a, 0xe2, 0xbc, 0x24, 0x06, 0x0a, 0x04, 0x4a,
	0x53, 0x4f, 0x4e, 0x52, 0x07, 0x76, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x12, 0x21, 0x0a, 0x0c,
	0x76, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x5f, 0x68, 0x61, 0x73, 0x68, 0x18, 0x04, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x0b, 0x76, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x48, 0x61, 0x73, 0x68, 0x12,
	0x50, 0x0a, 0x0a, 0x69, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x05, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x30, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x61, 0x6e, 0x61, 0x6c, 0x79,
	0x73, 0x69, 0x73, 0x2e, 0x62, 0x71, 0x2e, 0x54, 0x65, 0x73, 0x74, 0x52, 0x65, 0x73, 0x75, 0x6c,
	0x74, 0x52, 0x6f, 0x77, 0x2e, 0x49, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52,
	0x65, 0x63, 0x6f, 0x72, 0x64, 0x52, 0x0a, 0x69, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f,
	0x6e, 0x12, 0x41, 0x0a, 0x0e, 0x70, 0x61, 0x72, 0x74, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x74,
	0x69, 0x6d, 0x65, 0x18, 0x06, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65,
	0x73, 0x74, 0x61, 0x6d, 0x70, 0x52, 0x0d, 0x70, 0x61, 0x72, 0x74, 0x69, 0x74, 0x69, 0x6f, 0x6e,
	0x54, 0x69, 0x6d, 0x65, 0x12, 0x4e, 0x0a, 0x06, 0x70, 0x61, 0x72, 0x65, 0x6e, 0x74, 0x18, 0x07,
	0x20, 0x01, 0x28, 0x0b, 0x32, 0x36, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x61, 0x6e, 0x61, 0x6c,
	0x79, 0x73, 0x69, 0x73, 0x2e, 0x62, 0x71, 0x2e, 0x54, 0x65, 0x73, 0x74, 0x52, 0x65, 0x73, 0x75,
	0x6c, 0x74, 0x52, 0x6f, 0x77, 0x2e, 0x50, 0x61, 0x72, 0x65, 0x6e, 0x74, 0x49, 0x6e, 0x76, 0x6f,
	0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x65, 0x63, 0x6f, 0x72, 0x64, 0x52, 0x06, 0x70, 0x61,
	0x72, 0x65, 0x6e, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x08, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x1b, 0x0a, 0x09, 0x72, 0x65, 0x73, 0x75,
	0x6c, 0x74, 0x5f, 0x69, 0x64, 0x18, 0x09, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x72, 0x65, 0x73,
	0x75, 0x6c, 0x74, 0x49, 0x64, 0x12, 0x1a, 0x0a, 0x08, 0x65, 0x78, 0x70, 0x65, 0x63, 0x74, 0x65,
	0x64, 0x18, 0x0a, 0x20, 0x01, 0x28, 0x08, 0x52, 0x08, 0x65, 0x78, 0x70, 0x65, 0x63, 0x74, 0x65,
	0x64, 0x12, 0x3a, 0x0a, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x18, 0x0b, 0x20, 0x01, 0x28,
	0x0e, 0x32, 0x22, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69,
	0x73, 0x2e, 0x76, 0x31, 0x2e, 0x54, 0x65, 0x73, 0x74, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x53,
	0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x21, 0x0a,
	0x0c, 0x73, 0x75, 0x6d, 0x6d, 0x61, 0x72, 0x79, 0x5f, 0x68, 0x74, 0x6d, 0x6c, 0x18, 0x0c, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x0b, 0x73, 0x75, 0x6d, 0x6d, 0x61, 0x72, 0x79, 0x48, 0x74, 0x6d, 0x6c,
	0x12, 0x39, 0x0a, 0x0a, 0x73, 0x74, 0x61, 0x72, 0x74, 0x5f, 0x74, 0x69, 0x6d, 0x65, 0x18, 0x0d,
	0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70,
	0x52, 0x09, 0x73, 0x74, 0x61, 0x72, 0x74, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x23, 0x0a, 0x0d, 0x64,
	0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x73, 0x65, 0x63, 0x73, 0x18, 0x0e, 0x20, 0x01,
	0x28, 0x01, 0x52, 0x0c, 0x64, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x53, 0x65, 0x63, 0x73,
	0x12, 0x30, 0x0a, 0x04, 0x74, 0x61, 0x67, 0x73, 0x18, 0x0f, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x1c,
	0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2e, 0x76,
	0x31, 0x2e, 0x53, 0x74, 0x72, 0x69, 0x6e, 0x67, 0x50, 0x61, 0x69, 0x72, 0x52, 0x04, 0x74, 0x61,
	0x67, 0x73, 0x12, 0x46, 0x0a, 0x0e, 0x66, 0x61, 0x69, 0x6c, 0x75, 0x72, 0x65, 0x5f, 0x72, 0x65,
	0x61, 0x73, 0x6f, 0x6e, 0x18, 0x10, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1f, 0x2e, 0x6c, 0x75, 0x63,
	0x69, 0x2e, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x46, 0x61,
	0x69, 0x6c, 0x75, 0x72, 0x65, 0x52, 0x65, 0x61, 0x73, 0x6f, 0x6e, 0x52, 0x0d, 0x66, 0x61, 0x69,
	0x6c, 0x75, 0x72, 0x65, 0x52, 0x65, 0x61, 0x73, 0x6f, 0x6e, 0x12, 0x1f, 0x0a, 0x0b, 0x73, 0x6b,
	0x69, 0x70, 0x5f, 0x72, 0x65, 0x61, 0x73, 0x6f, 0x6e, 0x18, 0x11, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x0a, 0x73, 0x6b, 0x69, 0x70, 0x52, 0x65, 0x61, 0x73, 0x6f, 0x6e, 0x12, 0x2a, 0x0a, 0x0a, 0x70,
	0x72, 0x6f, 0x70, 0x65, 0x72, 0x74, 0x69, 0x65, 0x73, 0x18, 0x12, 0x20, 0x01, 0x28, 0x09, 0x42,
	0x0a, 0xe2, 0xbc, 0x24, 0x06, 0x0a, 0x04, 0x4a, 0x53, 0x4f, 0x4e, 0x52, 0x0a, 0x70, 0x72, 0x6f,
	0x70, 0x65, 0x72, 0x74, 0x69, 0x65, 0x73, 0x12, 0x33, 0x0a, 0x07, 0x73, 0x6f, 0x75, 0x72, 0x63,
	0x65, 0x73, 0x18, 0x13, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e,
	0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x53, 0x6f, 0x75, 0x72,
	0x63, 0x65, 0x73, 0x52, 0x07, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x73, 0x12, 0x3a, 0x0a, 0x0a,
	0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x5f, 0x72, 0x65, 0x66, 0x18, 0x14, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x1b, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73,
	0x2e, 0x76, 0x31, 0x2e, 0x53, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x52, 0x65, 0x66, 0x52, 0x09, 0x73,
	0x6f, 0x75, 0x72, 0x63, 0x65, 0x52, 0x65, 0x66, 0x12, 0x26, 0x0a, 0x0f, 0x73, 0x6f, 0x75, 0x72,
	0x63, 0x65, 0x5f, 0x72, 0x65, 0x66, 0x5f, 0x68, 0x61, 0x73, 0x68, 0x18, 0x15, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x0d, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x52, 0x65, 0x66, 0x48, 0x61, 0x73, 0x68,
	0x12, 0x43, 0x0a, 0x0d, 0x74, 0x65, 0x73, 0x74, 0x5f, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74,
	0x61, 0x18, 0x16, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1e, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x61,
	0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x54, 0x65, 0x73, 0x74, 0x4d,
	0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x52, 0x0c, 0x74, 0x65, 0x73, 0x74, 0x4d, 0x65, 0x74,
	0x61, 0x64, 0x61, 0x74, 0x61, 0x1a, 0x38, 0x0a, 0x10, 0x49, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74,
	0x69, 0x6f, 0x6e, 0x52, 0x65, 0x63, 0x6f, 0x72, 0x64, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x69, 0x64, 0x12, 0x14, 0x0a, 0x05, 0x72, 0x65, 0x61,
	0x6c, 0x6d, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x72, 0x65, 0x61, 0x6c, 0x6d, 0x1a,
	0x9c, 0x01, 0x0a, 0x16, 0x50, 0x61, 0x72, 0x65, 0x6e, 0x74, 0x49, 0x6e, 0x76, 0x6f, 0x63, 0x61,
	0x74, 0x69, 0x6f, 0x6e, 0x52, 0x65, 0x63, 0x6f, 0x72, 0x64, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x69, 0x64, 0x12, 0x30, 0x0a, 0x04, 0x74, 0x61,
	0x67, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x1c, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e,
	0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x53, 0x74, 0x72, 0x69,
	0x6e, 0x67, 0x50, 0x61, 0x69, 0x72, 0x52, 0x04, 0x74, 0x61, 0x67, 0x73, 0x12, 0x14, 0x0a, 0x05,
	0x72, 0x65, 0x61, 0x6c, 0x6d, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x72, 0x65, 0x61,
	0x6c, 0x6d, 0x12, 0x2a, 0x0a, 0x0a, 0x70, 0x72, 0x6f, 0x70, 0x65, 0x72, 0x74, 0x69, 0x65, 0x73,
	0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x42, 0x0a, 0xe2, 0xbc, 0x24, 0x06, 0x0a, 0x04, 0x4a, 0x53,
	0x4f, 0x4e, 0x52, 0x0a, 0x70, 0x72, 0x6f, 0x70, 0x65, 0x72, 0x74, 0x69, 0x65, 0x73, 0x42, 0x2d,
	0x5a, 0x2b, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2f,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x62, 0x71, 0x3b, 0x62, 0x71, 0x70, 0x62, 0x62, 0x06, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_rawDescData = file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_rawDesc
)

func file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_rawDescData = protoimpl.X.CompressGZIP(file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_rawDescData)
	})
	return file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_rawDescData
}

var file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_goTypes = []interface{}{
	(*TestResultRow)(nil),                        // 0: luci.analysis.bq.TestResultRow
	(*TestResultRow_InvocationRecord)(nil),       // 1: luci.analysis.bq.TestResultRow.InvocationRecord
	(*TestResultRow_ParentInvocationRecord)(nil), // 2: luci.analysis.bq.TestResultRow.ParentInvocationRecord
	(*timestamppb.Timestamp)(nil),                // 3: google.protobuf.Timestamp
	(v1.TestResultStatus)(0),                     // 4: luci.analysis.v1.TestResultStatus
	(*v1.StringPair)(nil),                        // 5: luci.analysis.v1.StringPair
	(*v1.FailureReason)(nil),                     // 6: luci.analysis.v1.FailureReason
	(*v1.Sources)(nil),                           // 7: luci.analysis.v1.Sources
	(*v1.SourceRef)(nil),                         // 8: luci.analysis.v1.SourceRef
	(*v1.TestMetadata)(nil),                      // 9: luci.analysis.v1.TestMetadata
}
var file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_depIdxs = []int32{
	1,  // 0: luci.analysis.bq.TestResultRow.invocation:type_name -> luci.analysis.bq.TestResultRow.InvocationRecord
	3,  // 1: luci.analysis.bq.TestResultRow.partition_time:type_name -> google.protobuf.Timestamp
	2,  // 2: luci.analysis.bq.TestResultRow.parent:type_name -> luci.analysis.bq.TestResultRow.ParentInvocationRecord
	4,  // 3: luci.analysis.bq.TestResultRow.status:type_name -> luci.analysis.v1.TestResultStatus
	3,  // 4: luci.analysis.bq.TestResultRow.start_time:type_name -> google.protobuf.Timestamp
	5,  // 5: luci.analysis.bq.TestResultRow.tags:type_name -> luci.analysis.v1.StringPair
	6,  // 6: luci.analysis.bq.TestResultRow.failure_reason:type_name -> luci.analysis.v1.FailureReason
	7,  // 7: luci.analysis.bq.TestResultRow.sources:type_name -> luci.analysis.v1.Sources
	8,  // 8: luci.analysis.bq.TestResultRow.source_ref:type_name -> luci.analysis.v1.SourceRef
	9,  // 9: luci.analysis.bq.TestResultRow.test_metadata:type_name -> luci.analysis.v1.TestMetadata
	5,  // 10: luci.analysis.bq.TestResultRow.ParentInvocationRecord.tags:type_name -> luci.analysis.v1.StringPair
	11, // [11:11] is the sub-list for method output_type
	11, // [11:11] is the sub-list for method input_type
	11, // [11:11] is the sub-list for extension type_name
	11, // [11:11] is the sub-list for extension extendee
	0,  // [0:11] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_init() }
func file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_init() {
	if File_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*TestResultRow); i {
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
		file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*TestResultRow_InvocationRecord); i {
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
		file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*TestResultRow_ParentInvocationRecord); i {
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
			RawDescriptor: file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto = out.File
	file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_rawDesc = nil
	file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_goTypes = nil
	file_go_chromium_org_luci_analysis_proto_bq_test_result_row_proto_depIdxs = nil
}