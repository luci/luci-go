// Copyright 2020 The LUCI Authors.
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
// 	protoc-gen-go v1.26.0
// 	protoc        v3.17.0
// source: go.chromium.org/luci/resultdb/internal/proto/ui/ui.proto

package uipb

import prpc "go.chromium.org/luci/grpc/prpc"

import (
	context "context"
	v1 "go.chromium.org/luci/resultdb/proto/v1"
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

// Status of a test variant.
type TestVariantStatus int32

const (
	// a test variant must not have this status.
	// This is only used when filtering variants.
	TestVariantStatus_TEST_VARIANT_STATUS_UNSPECIFIED TestVariantStatus = 0
	// The test variant has no exonerations, and all results are unexpected.
	TestVariantStatus_UNEXPECTED TestVariantStatus = 10
	// The test variant has no exonerations, and all results are unexpectedly skipped.
	TestVariantStatus_UNEXPECTEDLY_SKIPPED TestVariantStatus = 20
	// The test variant has no exonerations, and has both expected and unexpected
	// results.
	TestVariantStatus_FLAKY TestVariantStatus = 30
	// The test variant has one or more test exonerations.
	TestVariantStatus_EXONERATED TestVariantStatus = 40
	// The test variant has no exonerations, and all results are expected.
	TestVariantStatus_EXPECTED TestVariantStatus = 50
)

// Enum value maps for TestVariantStatus.
var (
	TestVariantStatus_name = map[int32]string{
		0:  "TEST_VARIANT_STATUS_UNSPECIFIED",
		10: "UNEXPECTED",
		20: "UNEXPECTEDLY_SKIPPED",
		30: "FLAKY",
		40: "EXONERATED",
		50: "EXPECTED",
	}
	TestVariantStatus_value = map[string]int32{
		"TEST_VARIANT_STATUS_UNSPECIFIED": 0,
		"UNEXPECTED":                      10,
		"UNEXPECTEDLY_SKIPPED":            20,
		"FLAKY":                           30,
		"EXONERATED":                      40,
		"EXPECTED":                        50,
	}
)

func (x TestVariantStatus) Enum() *TestVariantStatus {
	p := new(TestVariantStatus)
	*p = x
	return p
}

func (x TestVariantStatus) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (TestVariantStatus) Descriptor() protoreflect.EnumDescriptor {
	return file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_enumTypes[0].Descriptor()
}

func (TestVariantStatus) Type() protoreflect.EnumType {
	return &file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_enumTypes[0]
}

func (x TestVariantStatus) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use TestVariantStatus.Descriptor instead.
func (TestVariantStatus) EnumDescriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_rawDescGZIP(), []int{0}
}

// A request message for QueryTestVariants RPC.
// Next id: 7.
type QueryTestVariantsRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Retrieve test variants included in these invocations, directly or indirectly
	// (via Invocation.included_invocations).
	//
	// Specifying multiple invocations is equivalent to querying one invocation
	// that includes these.
	Invocations []string `protobuf:"bytes,2,rep,name=invocations,proto3" json:"invocations,omitempty"`
	// A test variant must satisfy this predicate.
	Predicate *TestVariantPredicate `protobuf:"bytes,6,opt,name=predicate,proto3" json:"predicate,omitempty"`
	// The maximum number of test variants to return.
	//
	// The service may return fewer than this value.
	// If unspecified, at most 100 test variants will be returned.
	// The maximum value is 1000; values above 1000 will be coerced to 1000.
	PageSize int32 `protobuf:"varint,4,opt,name=page_size,json=pageSize,proto3" json:"page_size,omitempty"`
	// A page token, received from a previous `QueryTestVariants` call.
	// Provide this to retrieve the subsequent page.
	//
	// When paginating, all other parameters provided to `QueryTestVariants` MUST
	// match the call that provided the page token.
	PageToken string `protobuf:"bytes,5,opt,name=page_token,json=pageToken,proto3" json:"page_token,omitempty"`
}

func (x *QueryTestVariantsRequest) Reset() {
	*x = QueryTestVariantsRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *QueryTestVariantsRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*QueryTestVariantsRequest) ProtoMessage() {}

func (x *QueryTestVariantsRequest) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use QueryTestVariantsRequest.ProtoReflect.Descriptor instead.
func (*QueryTestVariantsRequest) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_rawDescGZIP(), []int{0}
}

func (x *QueryTestVariantsRequest) GetInvocations() []string {
	if x != nil {
		return x.Invocations
	}
	return nil
}

func (x *QueryTestVariantsRequest) GetPredicate() *TestVariantPredicate {
	if x != nil {
		return x.Predicate
	}
	return nil
}

func (x *QueryTestVariantsRequest) GetPageSize() int32 {
	if x != nil {
		return x.PageSize
	}
	return 0
}

func (x *QueryTestVariantsRequest) GetPageToken() string {
	if x != nil {
		return x.PageToken
	}
	return ""
}

// A response message for QueryTestVariants RPC.
type QueryTestVariantsResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Matched test variants.
	// Ordered by TestVariantStatus, test_id, then variant_hash
	TestVariants []*TestVariant `protobuf:"bytes,1,rep,name=test_variants,json=testVariants,proto3" json:"test_variants,omitempty"`
	// A token, which can be sent as `page_token` to retrieve the next page.
	// If this field is omitted, there were no subsequent pages at the time of
	// request.
	NextPageToken string `protobuf:"bytes,2,opt,name=next_page_token,json=nextPageToken,proto3" json:"next_page_token,omitempty"`
}

func (x *QueryTestVariantsResponse) Reset() {
	*x = QueryTestVariantsResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *QueryTestVariantsResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*QueryTestVariantsResponse) ProtoMessage() {}

func (x *QueryTestVariantsResponse) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use QueryTestVariantsResponse.ProtoReflect.Descriptor instead.
func (*QueryTestVariantsResponse) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_rawDescGZIP(), []int{1}
}

func (x *QueryTestVariantsResponse) GetTestVariants() []*TestVariant {
	if x != nil {
		return x.TestVariants
	}
	return nil
}

func (x *QueryTestVariantsResponse) GetNextPageToken() string {
	if x != nil {
		return x.NextPageToken
	}
	return ""
}

// Represents a matching test variant with its outcomes.
type TestVariant struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// A unique identifier of the test in a LUCI project.
	// Regex: ^[[::print::]]{1,256}$
	//
	// Refer to luci.resultdb.v1.TestResult.test_id for details.
	TestId string `protobuf:"bytes,1,opt,name=test_id,json=testId,proto3" json:"test_id,omitempty"`
	// Description of one specific way of running the test,
	// e.g. a specific bucket, builder and a test suite.
	Variant *v1.Variant `protobuf:"bytes,2,opt,name=variant,proto3" json:"variant,omitempty"`
	// Hash of the variant.
	// hex(sha256(sorted(''.join('%s:%s\n' for k, v in variant.items())))).
	VariantHash string `protobuf:"bytes,3,opt,name=variant_hash,json=variantHash,proto3" json:"variant_hash,omitempty"`
	// Status of the test variant.
	Status TestVariantStatus `protobuf:"varint,4,opt,name=status,proto3,enum=luci.resultdb.internal.ui.TestVariantStatus" json:"status,omitempty"`
	// Outcomes of the test variant.
	Results []*TestResultBundle `protobuf:"bytes,5,rep,name=results,proto3" json:"results,omitempty"`
	// Test exonerations if any test variant is exonerated.
	Exonerations []*v1.TestExoneration `protobuf:"bytes,6,rep,name=exonerations,proto3" json:"exonerations,omitempty"`
	// Information about the test at the time of its execution.
	//
	// All test results of the same test variant should report the same test
	// metadata. This RPC relies on this rule and returns test metadata from
	// *arbitrary* result of the test variant.
	TestMetadata *v1.TestMetadata `protobuf:"bytes,7,opt,name=test_metadata,json=testMetadata,proto3" json:"test_metadata,omitempty"`
}

func (x *TestVariant) Reset() {
	*x = TestVariant{}
	if protoimpl.UnsafeEnabled {
		mi := &file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *TestVariant) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TestVariant) ProtoMessage() {}

func (x *TestVariant) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TestVariant.ProtoReflect.Descriptor instead.
func (*TestVariant) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_rawDescGZIP(), []int{2}
}

func (x *TestVariant) GetTestId() string {
	if x != nil {
		return x.TestId
	}
	return ""
}

func (x *TestVariant) GetVariant() *v1.Variant {
	if x != nil {
		return x.Variant
	}
	return nil
}

func (x *TestVariant) GetVariantHash() string {
	if x != nil {
		return x.VariantHash
	}
	return ""
}

func (x *TestVariant) GetStatus() TestVariantStatus {
	if x != nil {
		return x.Status
	}
	return TestVariantStatus_TEST_VARIANT_STATUS_UNSPECIFIED
}

func (x *TestVariant) GetResults() []*TestResultBundle {
	if x != nil {
		return x.Results
	}
	return nil
}

func (x *TestVariant) GetExonerations() []*v1.TestExoneration {
	if x != nil {
		return x.Exonerations
	}
	return nil
}

func (x *TestVariant) GetTestMetadata() *v1.TestMetadata {
	if x != nil {
		return x.TestMetadata
	}
	return nil
}

// Outcomes of an execution of the test variant.
type TestResultBundle struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Result of the test variant execution.
	Result *v1.TestResult `protobuf:"bytes,1,opt,name=result,proto3" json:"result,omitempty"`
}

func (x *TestResultBundle) Reset() {
	*x = TestResultBundle{}
	if protoimpl.UnsafeEnabled {
		mi := &file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *TestResultBundle) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TestResultBundle) ProtoMessage() {}

func (x *TestResultBundle) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TestResultBundle.ProtoReflect.Descriptor instead.
func (*TestResultBundle) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_rawDescGZIP(), []int{3}
}

func (x *TestResultBundle) GetResult() *v1.TestResult {
	if x != nil {
		return x.Result
	}
	return nil
}

// Represents a function TestVariant -> bool.
// Empty message matches all test variants.
type TestVariantPredicate struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// A test variant must have this status.
	Status TestVariantStatus `protobuf:"varint,1,opt,name=status,proto3,enum=luci.resultdb.internal.ui.TestVariantStatus" json:"status,omitempty"`
}

func (x *TestVariantPredicate) Reset() {
	*x = TestVariantPredicate{}
	if protoimpl.UnsafeEnabled {
		mi := &file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *TestVariantPredicate) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TestVariantPredicate) ProtoMessage() {}

func (x *TestVariantPredicate) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TestVariantPredicate.ProtoReflect.Descriptor instead.
func (*TestVariantPredicate) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_rawDescGZIP(), []int{4}
}

func (x *TestVariantPredicate) GetStatus() TestVariantStatus {
	if x != nil {
		return x.Status
	}
	return TestVariantStatus_TEST_VARIANT_STATUS_UNSPECIFIED
}

var File_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_rawDesc = []byte{
	0x0a, 0x38, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2f,
	0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x75,
	0x69, 0x2f, 0x75, 0x69, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x19, 0x6c, 0x75, 0x63, 0x69,
	0x2e, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e,
	0x61, 0x6c, 0x2e, 0x75, 0x69, 0x1a, 0x33, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69,
	0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x72, 0x65, 0x73, 0x75,
	0x6c, 0x74, 0x64, 0x62, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x76, 0x31, 0x2f, 0x63, 0x6f,
	0x6d, 0x6d, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x3a, 0x67, 0x6f, 0x2e, 0x63,
	0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69,
	0x2f, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f,
	0x76, 0x31, 0x2f, 0x74, 0x65, 0x73, 0x74, 0x5f, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x38, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d,
	0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x72, 0x65, 0x73,
	0x75, 0x6c, 0x74, 0x64, 0x62, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x76, 0x31, 0x2f, 0x74,
	0x65, 0x73, 0x74, 0x5f, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x22, 0xc7, 0x01, 0x0a, 0x18, 0x51, 0x75, 0x65, 0x72, 0x79, 0x54, 0x65, 0x73, 0x74, 0x56, 0x61,
	0x72, 0x69, 0x61, 0x6e, 0x74, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x20, 0x0a,
	0x0b, 0x69, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0x02, 0x20, 0x03,
	0x28, 0x09, 0x52, 0x0b, 0x69, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x12,
	0x4d, 0x0a, 0x09, 0x70, 0x72, 0x65, 0x64, 0x69, 0x63, 0x61, 0x74, 0x65, 0x18, 0x06, 0x20, 0x01,
	0x28, 0x0b, 0x32, 0x2f, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74,
	0x64, 0x62, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x75, 0x69, 0x2e, 0x54,
	0x65, 0x73, 0x74, 0x56, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x50, 0x72, 0x65, 0x64, 0x69, 0x63,
	0x61, 0x74, 0x65, 0x52, 0x09, 0x70, 0x72, 0x65, 0x64, 0x69, 0x63, 0x61, 0x74, 0x65, 0x12, 0x1b,
	0x0a, 0x09, 0x70, 0x61, 0x67, 0x65, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28,
	0x05, 0x52, 0x08, 0x70, 0x61, 0x67, 0x65, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x1d, 0x0a, 0x0a, 0x70,
	0x61, 0x67, 0x65, 0x5f, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x09, 0x70, 0x61, 0x67, 0x65, 0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x22, 0x90, 0x01, 0x0a, 0x19, 0x51,
	0x75, 0x65, 0x72, 0x79, 0x54, 0x65, 0x73, 0x74, 0x56, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x73,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x4b, 0x0a, 0x0d, 0x74, 0x65, 0x73, 0x74,
	0x5f, 0x76, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32,
	0x26, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2e,
	0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x75, 0x69, 0x2e, 0x54, 0x65, 0x73, 0x74,
	0x56, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x52, 0x0c, 0x74, 0x65, 0x73, 0x74, 0x56, 0x61, 0x72,
	0x69, 0x61, 0x6e, 0x74, 0x73, 0x12, 0x26, 0x0a, 0x0f, 0x6e, 0x65, 0x78, 0x74, 0x5f, 0x70, 0x61,
	0x67, 0x65, 0x5f, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0d,
	0x6e, 0x65, 0x78, 0x74, 0x50, 0x61, 0x67, 0x65, 0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x22, 0x97, 0x03,
	0x0a, 0x0b, 0x54, 0x65, 0x73, 0x74, 0x56, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x12, 0x17, 0x0a,
	0x07, 0x74, 0x65, 0x73, 0x74, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06,
	0x74, 0x65, 0x73, 0x74, 0x49, 0x64, 0x12, 0x33, 0x0a, 0x07, 0x76, 0x61, 0x72, 0x69, 0x61, 0x6e,
	0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x72,
	0x65, 0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2e, 0x76, 0x31, 0x2e, 0x56, 0x61, 0x72, 0x69, 0x61,
	0x6e, 0x74, 0x52, 0x07, 0x76, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x12, 0x21, 0x0a, 0x0c, 0x76,
	0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x5f, 0x68, 0x61, 0x73, 0x68, 0x18, 0x03, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x0b, 0x76, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x48, 0x61, 0x73, 0x68, 0x12, 0x44,
	0x0a, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x2c,
	0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2e, 0x69,
	0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x75, 0x69, 0x2e, 0x54, 0x65, 0x73, 0x74, 0x56,
	0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x06, 0x73, 0x74,
	0x61, 0x74, 0x75, 0x73, 0x12, 0x45, 0x0a, 0x07, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x73, 0x18,
	0x05, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x2b, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x72, 0x65, 0x73,
	0x75, 0x6c, 0x74, 0x64, 0x62, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x75,
	0x69, 0x2e, 0x54, 0x65, 0x73, 0x74, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x42, 0x75, 0x6e, 0x64,
	0x6c, 0x65, 0x52, 0x07, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x73, 0x12, 0x45, 0x0a, 0x0c, 0x65,
	0x78, 0x6f, 0x6e, 0x65, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0x06, 0x20, 0x03, 0x28,
	0x0b, 0x32, 0x21, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64,
	0x62, 0x2e, 0x76, 0x31, 0x2e, 0x54, 0x65, 0x73, 0x74, 0x45, 0x78, 0x6f, 0x6e, 0x65, 0x72, 0x61,
	0x74, 0x69, 0x6f, 0x6e, 0x52, 0x0c, 0x65, 0x78, 0x6f, 0x6e, 0x65, 0x72, 0x61, 0x74, 0x69, 0x6f,
	0x6e, 0x73, 0x12, 0x43, 0x0a, 0x0d, 0x74, 0x65, 0x73, 0x74, 0x5f, 0x6d, 0x65, 0x74, 0x61, 0x64,
	0x61, 0x74, 0x61, 0x18, 0x07, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1e, 0x2e, 0x6c, 0x75, 0x63, 0x69,
	0x2e, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2e, 0x76, 0x31, 0x2e, 0x54, 0x65, 0x73,
	0x74, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x52, 0x0c, 0x74, 0x65, 0x73, 0x74, 0x4d,
	0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x22, 0x48, 0x0a, 0x10, 0x54, 0x65, 0x73, 0x74, 0x52,
	0x65, 0x73, 0x75, 0x6c, 0x74, 0x42, 0x75, 0x6e, 0x64, 0x6c, 0x65, 0x12, 0x34, 0x0a, 0x06, 0x72,
	0x65, 0x73, 0x75, 0x6c, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1c, 0x2e, 0x6c, 0x75,
	0x63, 0x69, 0x2e, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2e, 0x76, 0x31, 0x2e, 0x54,
	0x65, 0x73, 0x74, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x52, 0x06, 0x72, 0x65, 0x73, 0x75, 0x6c,
	0x74, 0x22, 0x5c, 0x0a, 0x14, 0x54, 0x65, 0x73, 0x74, 0x56, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74,
	0x50, 0x72, 0x65, 0x64, 0x69, 0x63, 0x61, 0x74, 0x65, 0x12, 0x44, 0x0a, 0x06, 0x73, 0x74, 0x61,
	0x74, 0x75, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x2c, 0x2e, 0x6c, 0x75, 0x63, 0x69,
	0x2e, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e,
	0x61, 0x6c, 0x2e, 0x75, 0x69, 0x2e, 0x54, 0x65, 0x73, 0x74, 0x56, 0x61, 0x72, 0x69, 0x61, 0x6e,
	0x74, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x2a,
	0x8b, 0x01, 0x0a, 0x11, 0x54, 0x65, 0x73, 0x74, 0x56, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x53,
	0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x23, 0x0a, 0x1f, 0x54, 0x45, 0x53, 0x54, 0x5f, 0x56, 0x41,
	0x52, 0x49, 0x41, 0x4e, 0x54, 0x5f, 0x53, 0x54, 0x41, 0x54, 0x55, 0x53, 0x5f, 0x55, 0x4e, 0x53,
	0x50, 0x45, 0x43, 0x49, 0x46, 0x49, 0x45, 0x44, 0x10, 0x00, 0x12, 0x0e, 0x0a, 0x0a, 0x55, 0x4e,
	0x45, 0x58, 0x50, 0x45, 0x43, 0x54, 0x45, 0x44, 0x10, 0x0a, 0x12, 0x18, 0x0a, 0x14, 0x55, 0x4e,
	0x45, 0x58, 0x50, 0x45, 0x43, 0x54, 0x45, 0x44, 0x4c, 0x59, 0x5f, 0x53, 0x4b, 0x49, 0x50, 0x50,
	0x45, 0x44, 0x10, 0x14, 0x12, 0x09, 0x0a, 0x05, 0x46, 0x4c, 0x41, 0x4b, 0x59, 0x10, 0x1e, 0x12,
	0x0e, 0x0a, 0x0a, 0x45, 0x58, 0x4f, 0x4e, 0x45, 0x52, 0x41, 0x54, 0x45, 0x44, 0x10, 0x28, 0x12,
	0x0c, 0x0a, 0x08, 0x45, 0x58, 0x50, 0x45, 0x43, 0x54, 0x45, 0x44, 0x10, 0x32, 0x32, 0x87, 0x01,
	0x0a, 0x02, 0x55, 0x49, 0x12, 0x80, 0x01, 0x0a, 0x11, 0x51, 0x75, 0x65, 0x72, 0x79, 0x54, 0x65,
	0x73, 0x74, 0x56, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x73, 0x12, 0x33, 0x2e, 0x6c, 0x75, 0x63,
	0x69, 0x2e, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72,
	0x6e, 0x61, 0x6c, 0x2e, 0x75, 0x69, 0x2e, 0x51, 0x75, 0x65, 0x72, 0x79, 0x54, 0x65, 0x73, 0x74,
	0x56, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a,
	0x34, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2e,
	0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x75, 0x69, 0x2e, 0x51, 0x75, 0x65, 0x72,
	0x79, 0x54, 0x65, 0x73, 0x74, 0x56, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x73, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x42, 0x36, 0x5a, 0x34, 0x67, 0x6f, 0x2e, 0x63, 0x68,
	0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f,
	0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61,
	0x6c, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x75, 0x69, 0x3b, 0x75, 0x69, 0x70, 0x62, 0x62,
	0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_rawDescData = file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_rawDesc
)

func file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_rawDescData = protoimpl.X.CompressGZIP(file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_rawDescData)
	})
	return file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_rawDescData
}

var file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_msgTypes = make([]protoimpl.MessageInfo, 5)
var file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_goTypes = []interface{}{
	(TestVariantStatus)(0),            // 0: luci.resultdb.internal.ui.TestVariantStatus
	(*QueryTestVariantsRequest)(nil),  // 1: luci.resultdb.internal.ui.QueryTestVariantsRequest
	(*QueryTestVariantsResponse)(nil), // 2: luci.resultdb.internal.ui.QueryTestVariantsResponse
	(*TestVariant)(nil),               // 3: luci.resultdb.internal.ui.TestVariant
	(*TestResultBundle)(nil),          // 4: luci.resultdb.internal.ui.TestResultBundle
	(*TestVariantPredicate)(nil),      // 5: luci.resultdb.internal.ui.TestVariantPredicate
	(*v1.Variant)(nil),                // 6: luci.resultdb.v1.Variant
	(*v1.TestExoneration)(nil),        // 7: luci.resultdb.v1.TestExoneration
	(*v1.TestMetadata)(nil),           // 8: luci.resultdb.v1.TestMetadata
	(*v1.TestResult)(nil),             // 9: luci.resultdb.v1.TestResult
}
var file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_depIdxs = []int32{
	5,  // 0: luci.resultdb.internal.ui.QueryTestVariantsRequest.predicate:type_name -> luci.resultdb.internal.ui.TestVariantPredicate
	3,  // 1: luci.resultdb.internal.ui.QueryTestVariantsResponse.test_variants:type_name -> luci.resultdb.internal.ui.TestVariant
	6,  // 2: luci.resultdb.internal.ui.TestVariant.variant:type_name -> luci.resultdb.v1.Variant
	0,  // 3: luci.resultdb.internal.ui.TestVariant.status:type_name -> luci.resultdb.internal.ui.TestVariantStatus
	4,  // 4: luci.resultdb.internal.ui.TestVariant.results:type_name -> luci.resultdb.internal.ui.TestResultBundle
	7,  // 5: luci.resultdb.internal.ui.TestVariant.exonerations:type_name -> luci.resultdb.v1.TestExoneration
	8,  // 6: luci.resultdb.internal.ui.TestVariant.test_metadata:type_name -> luci.resultdb.v1.TestMetadata
	9,  // 7: luci.resultdb.internal.ui.TestResultBundle.result:type_name -> luci.resultdb.v1.TestResult
	0,  // 8: luci.resultdb.internal.ui.TestVariantPredicate.status:type_name -> luci.resultdb.internal.ui.TestVariantStatus
	1,  // 9: luci.resultdb.internal.ui.UI.QueryTestVariants:input_type -> luci.resultdb.internal.ui.QueryTestVariantsRequest
	2,  // 10: luci.resultdb.internal.ui.UI.QueryTestVariants:output_type -> luci.resultdb.internal.ui.QueryTestVariantsResponse
	10, // [10:11] is the sub-list for method output_type
	9,  // [9:10] is the sub-list for method input_type
	9,  // [9:9] is the sub-list for extension type_name
	9,  // [9:9] is the sub-list for extension extendee
	0,  // [0:9] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_init() }
func file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_init() {
	if File_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*QueryTestVariantsRequest); i {
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
		file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*QueryTestVariantsResponse); i {
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
		file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*TestVariant); i {
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
		file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*TestResultBundle); i {
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
		file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*TestVariantPredicate); i {
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
			RawDescriptor: file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   5,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_depIdxs,
		EnumInfos:         file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_enumTypes,
		MessageInfos:      file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto = out.File
	file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_rawDesc = nil
	file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_goTypes = nil
	file_go_chromium_org_luci_resultdb_internal_proto_ui_ui_proto_depIdxs = nil
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConnInterface

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion6

// UIClient is the client API for UI service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type UIClient interface {
	// Retrieves test variants from an invocation, recursively.
	// Supports invocation inclusions.
	// For displaying test variants in the UI.
	QueryTestVariants(ctx context.Context, in *QueryTestVariantsRequest, opts ...grpc.CallOption) (*QueryTestVariantsResponse, error)
}
type uIPRPCClient struct {
	client *prpc.Client
}

func NewUIPRPCClient(client *prpc.Client) UIClient {
	return &uIPRPCClient{client}
}

func (c *uIPRPCClient) QueryTestVariants(ctx context.Context, in *QueryTestVariantsRequest, opts ...grpc.CallOption) (*QueryTestVariantsResponse, error) {
	out := new(QueryTestVariantsResponse)
	err := c.client.Call(ctx, "luci.resultdb.internal.ui.UI", "QueryTestVariants", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type uIClient struct {
	cc grpc.ClientConnInterface
}

func NewUIClient(cc grpc.ClientConnInterface) UIClient {
	return &uIClient{cc}
}

func (c *uIClient) QueryTestVariants(ctx context.Context, in *QueryTestVariantsRequest, opts ...grpc.CallOption) (*QueryTestVariantsResponse, error) {
	out := new(QueryTestVariantsResponse)
	err := c.cc.Invoke(ctx, "/luci.resultdb.internal.ui.UI/QueryTestVariants", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// UIServer is the server API for UI service.
type UIServer interface {
	// Retrieves test variants from an invocation, recursively.
	// Supports invocation inclusions.
	// For displaying test variants in the UI.
	QueryTestVariants(context.Context, *QueryTestVariantsRequest) (*QueryTestVariantsResponse, error)
}

// UnimplementedUIServer can be embedded to have forward compatible implementations.
type UnimplementedUIServer struct {
}

func (*UnimplementedUIServer) QueryTestVariants(context.Context, *QueryTestVariantsRequest) (*QueryTestVariantsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method QueryTestVariants not implemented")
}

func RegisterUIServer(s prpc.Registrar, srv UIServer) {
	s.RegisterService(&_UI_serviceDesc, srv)
}

func _UI_QueryTestVariants_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(QueryTestVariantsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(UIServer).QueryTestVariants(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/luci.resultdb.internal.ui.UI/QueryTestVariants",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(UIServer).QueryTestVariants(ctx, req.(*QueryTestVariantsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _UI_serviceDesc = grpc.ServiceDesc{
	ServiceName: "luci.resultdb.internal.ui.UI",
	HandlerType: (*UIServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "QueryTestVariants",
			Handler:    _UI_QueryTestVariants_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "go.chromium.org/luci/resultdb/internal/proto/ui/ui.proto",
}
