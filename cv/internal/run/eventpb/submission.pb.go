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
// source: go.chromium.org/luci/cv/internal/run/eventpb/submission.proto

package eventpb

import (
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

type SubmissionResult int32

const (
	SubmissionResult_SUBMISSION_RESULT_UNSPECIFIED SubmissionResult = 0
	// All CLs have been submitted successfully.
	SubmissionResult_SUCCEEDED SubmissionResult = 1
	// Encountered transient failure.
	//
	// RM should retry if the deadline hasn't been exceeded.
	SubmissionResult_FAILED_TRANSIENT SubmissionResult = 2
	// Encountered permanent failure.
	//
	// For example, lack of submit permission or experienced merge conflict.
	SubmissionResult_FAILED_PERMANENT SubmissionResult = 3
)

// Enum value maps for SubmissionResult.
var (
	SubmissionResult_name = map[int32]string{
		0: "SUBMISSION_RESULT_UNSPECIFIED",
		1: "SUCCEEDED",
		2: "FAILED_TRANSIENT",
		3: "FAILED_PERMANENT",
	}
	SubmissionResult_value = map[string]int32{
		"SUBMISSION_RESULT_UNSPECIFIED": 0,
		"SUCCEEDED":                     1,
		"FAILED_TRANSIENT":              2,
		"FAILED_PERMANENT":              3,
	}
)

func (x SubmissionResult) Enum() *SubmissionResult {
	p := new(SubmissionResult)
	*p = x
	return p
}

func (x SubmissionResult) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (SubmissionResult) Descriptor() protoreflect.EnumDescriptor {
	return file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_enumTypes[0].Descriptor()
}

func (SubmissionResult) Type() protoreflect.EnumType {
	return &file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_enumTypes[0]
}

func (x SubmissionResult) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use SubmissionResult.Descriptor instead.
func (SubmissionResult) EnumDescriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_rawDescGZIP(), []int{0}
}

type SubmissionCompleted struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Result of this submission.
	Result                SubmissionResult       `protobuf:"varint,1,opt,name=result,proto3,enum=cv.internal.run.eventpb.SubmissionResult" json:"result,omitempty"`
	QueueReleaseTimestamp *timestamppb.Timestamp `protobuf:"bytes,5,opt,name=queue_release_timestamp,json=queueReleaseTimestamp,proto3" json:"queue_release_timestamp,omitempty"`
	// Types that are valid to be assigned to FailureReason:
	//
	//	*SubmissionCompleted_Timeout
	//	*SubmissionCompleted_ClFailures
	FailureReason isSubmissionCompleted_FailureReason `protobuf_oneof:"failure_reason"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SubmissionCompleted) Reset() {
	*x = SubmissionCompleted{}
	mi := &file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SubmissionCompleted) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SubmissionCompleted) ProtoMessage() {}

func (x *SubmissionCompleted) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SubmissionCompleted.ProtoReflect.Descriptor instead.
func (*SubmissionCompleted) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_rawDescGZIP(), []int{0}
}

func (x *SubmissionCompleted) GetResult() SubmissionResult {
	if x != nil {
		return x.Result
	}
	return SubmissionResult_SUBMISSION_RESULT_UNSPECIFIED
}

func (x *SubmissionCompleted) GetQueueReleaseTimestamp() *timestamppb.Timestamp {
	if x != nil {
		return x.QueueReleaseTimestamp
	}
	return nil
}

func (x *SubmissionCompleted) GetFailureReason() isSubmissionCompleted_FailureReason {
	if x != nil {
		return x.FailureReason
	}
	return nil
}

func (x *SubmissionCompleted) GetTimeout() bool {
	if x != nil {
		if x, ok := x.FailureReason.(*SubmissionCompleted_Timeout); ok {
			return x.Timeout
		}
	}
	return false
}

func (x *SubmissionCompleted) GetClFailures() *SubmissionCompleted_CLSubmissionFailures {
	if x != nil {
		if x, ok := x.FailureReason.(*SubmissionCompleted_ClFailures); ok {
			return x.ClFailures
		}
	}
	return nil
}

type isSubmissionCompleted_FailureReason interface {
	isSubmissionCompleted_FailureReason()
}

type SubmissionCompleted_Timeout struct {
	// Submission deadline is exceeded. Must be permanent failure.
	Timeout bool `protobuf:"varint,3,opt,name=timeout,proto3,oneof"`
}

type SubmissionCompleted_ClFailures struct {
	// CLs that fail to submit. Could be transient or permanent.
	//
	// As of June 2021, CLs are submitted serially and submitter returns
	// immediately upon failure so `cl_failures` will have only one entry.
	// However, submitter may report multiple CL submission failures in the
	// future (e.g. CV supports parallel CL submission or CV submits a CL
	// stack in one RPC).
	ClFailures *SubmissionCompleted_CLSubmissionFailures `protobuf:"bytes,4,opt,name=cl_failures,json=clFailures,proto3,oneof"`
}

func (*SubmissionCompleted_Timeout) isSubmissionCompleted_FailureReason() {}

func (*SubmissionCompleted_ClFailures) isSubmissionCompleted_FailureReason() {}

type SubmissionCompleted_CLSubmissionFailure struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Clid          int64                  `protobuf:"varint,1,opt,name=clid,proto3" json:"clid,omitempty"`      // Required
	Message       string                 `protobuf:"bytes,2,opt,name=message,proto3" json:"message,omitempty"` // Required
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SubmissionCompleted_CLSubmissionFailure) Reset() {
	*x = SubmissionCompleted_CLSubmissionFailure{}
	mi := &file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SubmissionCompleted_CLSubmissionFailure) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SubmissionCompleted_CLSubmissionFailure) ProtoMessage() {}

func (x *SubmissionCompleted_CLSubmissionFailure) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SubmissionCompleted_CLSubmissionFailure.ProtoReflect.Descriptor instead.
func (*SubmissionCompleted_CLSubmissionFailure) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_rawDescGZIP(), []int{0, 0}
}

func (x *SubmissionCompleted_CLSubmissionFailure) GetClid() int64 {
	if x != nil {
		return x.Clid
	}
	return 0
}

func (x *SubmissionCompleted_CLSubmissionFailure) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

type SubmissionCompleted_CLSubmissionFailures struct {
	state         protoimpl.MessageState                     `protogen:"open.v1"`
	Failures      []*SubmissionCompleted_CLSubmissionFailure `protobuf:"bytes,1,rep,name=failures,proto3" json:"failures,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *SubmissionCompleted_CLSubmissionFailures) Reset() {
	*x = SubmissionCompleted_CLSubmissionFailures{}
	mi := &file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *SubmissionCompleted_CLSubmissionFailures) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SubmissionCompleted_CLSubmissionFailures) ProtoMessage() {}

func (x *SubmissionCompleted_CLSubmissionFailures) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SubmissionCompleted_CLSubmissionFailures.ProtoReflect.Descriptor instead.
func (*SubmissionCompleted_CLSubmissionFailures) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_rawDescGZIP(), []int{0, 1}
}

func (x *SubmissionCompleted_CLSubmissionFailures) GetFailures() []*SubmissionCompleted_CLSubmissionFailure {
	if x != nil {
		return x.Failures
	}
	return nil
}

var File_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_rawDesc = string([]byte{
	0x0a, 0x3d, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x63, 0x76, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e,
	0x61, 0x6c, 0x2f, 0x72, 0x75, 0x6e, 0x2f, 0x65, 0x76, 0x65, 0x6e, 0x74, 0x70, 0x62, 0x2f, 0x73,
	0x75, 0x62, 0x6d, 0x69, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12,
	0x17, 0x63, 0x76, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x72, 0x75, 0x6e,
	0x2e, 0x65, 0x76, 0x65, 0x6e, 0x74, 0x70, 0x62, 0x1a, 0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74,
	0x61, 0x6d, 0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0xfb, 0x03, 0x0a, 0x13, 0x53, 0x75,
	0x62, 0x6d, 0x69, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x43, 0x6f, 0x6d, 0x70, 0x6c, 0x65, 0x74, 0x65,
	0x64, 0x12, 0x41, 0x0a, 0x06, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x0e, 0x32, 0x29, 0x2e, 0x63, 0x76, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e,
	0x72, 0x75, 0x6e, 0x2e, 0x65, 0x76, 0x65, 0x6e, 0x74, 0x70, 0x62, 0x2e, 0x53, 0x75, 0x62, 0x6d,
	0x69, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x52, 0x06, 0x72, 0x65,
	0x73, 0x75, 0x6c, 0x74, 0x12, 0x52, 0x0a, 0x17, 0x71, 0x75, 0x65, 0x75, 0x65, 0x5f, 0x72, 0x65,
	0x6c, 0x65, 0x61, 0x73, 0x65, 0x5f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x18,
	0x05, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d,
	0x70, 0x52, 0x15, 0x71, 0x75, 0x65, 0x75, 0x65, 0x52, 0x65, 0x6c, 0x65, 0x61, 0x73, 0x65, 0x54,
	0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x12, 0x1a, 0x0a, 0x07, 0x74, 0x69, 0x6d, 0x65,
	0x6f, 0x75, 0x74, 0x18, 0x03, 0x20, 0x01, 0x28, 0x08, 0x48, 0x00, 0x52, 0x07, 0x74, 0x69, 0x6d,
	0x65, 0x6f, 0x75, 0x74, 0x12, 0x64, 0x0a, 0x0b, 0x63, 0x6c, 0x5f, 0x66, 0x61, 0x69, 0x6c, 0x75,
	0x72, 0x65, 0x73, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x41, 0x2e, 0x63, 0x76, 0x2e, 0x69,
	0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x72, 0x75, 0x6e, 0x2e, 0x65, 0x76, 0x65, 0x6e,
	0x74, 0x70, 0x62, 0x2e, 0x53, 0x75, 0x62, 0x6d, 0x69, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x43, 0x6f,
	0x6d, 0x70, 0x6c, 0x65, 0x74, 0x65, 0x64, 0x2e, 0x43, 0x4c, 0x53, 0x75, 0x62, 0x6d, 0x69, 0x73,
	0x73, 0x69, 0x6f, 0x6e, 0x46, 0x61, 0x69, 0x6c, 0x75, 0x72, 0x65, 0x73, 0x48, 0x00, 0x52, 0x0a,
	0x63, 0x6c, 0x46, 0x61, 0x69, 0x6c, 0x75, 0x72, 0x65, 0x73, 0x1a, 0x43, 0x0a, 0x13, 0x43, 0x4c,
	0x53, 0x75, 0x62, 0x6d, 0x69, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x46, 0x61, 0x69, 0x6c, 0x75, 0x72,
	0x65, 0x12, 0x12, 0x0a, 0x04, 0x63, 0x6c, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x03, 0x52,
	0x04, 0x63, 0x6c, 0x69, 0x64, 0x12, 0x18, 0x0a, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x1a,
	0x74, 0x0a, 0x14, 0x43, 0x4c, 0x53, 0x75, 0x62, 0x6d, 0x69, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x46,
	0x61, 0x69, 0x6c, 0x75, 0x72, 0x65, 0x73, 0x12, 0x5c, 0x0a, 0x08, 0x66, 0x61, 0x69, 0x6c, 0x75,
	0x72, 0x65, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x40, 0x2e, 0x63, 0x76, 0x2e, 0x69,
	0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x72, 0x75, 0x6e, 0x2e, 0x65, 0x76, 0x65, 0x6e,
	0x74, 0x70, 0x62, 0x2e, 0x53, 0x75, 0x62, 0x6d, 0x69, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x43, 0x6f,
	0x6d, 0x70, 0x6c, 0x65, 0x74, 0x65, 0x64, 0x2e, 0x43, 0x4c, 0x53, 0x75, 0x62, 0x6d, 0x69, 0x73,
	0x73, 0x69, 0x6f, 0x6e, 0x46, 0x61, 0x69, 0x6c, 0x75, 0x72, 0x65, 0x52, 0x08, 0x66, 0x61, 0x69,
	0x6c, 0x75, 0x72, 0x65, 0x73, 0x42, 0x10, 0x0a, 0x0e, 0x66, 0x61, 0x69, 0x6c, 0x75, 0x72, 0x65,
	0x5f, 0x72, 0x65, 0x61, 0x73, 0x6f, 0x6e, 0x2a, 0x70, 0x0a, 0x10, 0x53, 0x75, 0x62, 0x6d, 0x69,
	0x73, 0x73, 0x69, 0x6f, 0x6e, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x12, 0x21, 0x0a, 0x1d, 0x53,
	0x55, 0x42, 0x4d, 0x49, 0x53, 0x53, 0x49, 0x4f, 0x4e, 0x5f, 0x52, 0x45, 0x53, 0x55, 0x4c, 0x54,
	0x5f, 0x55, 0x4e, 0x53, 0x50, 0x45, 0x43, 0x49, 0x46, 0x49, 0x45, 0x44, 0x10, 0x00, 0x12, 0x0d,
	0x0a, 0x09, 0x53, 0x55, 0x43, 0x43, 0x45, 0x45, 0x44, 0x45, 0x44, 0x10, 0x01, 0x12, 0x14, 0x0a,
	0x10, 0x46, 0x41, 0x49, 0x4c, 0x45, 0x44, 0x5f, 0x54, 0x52, 0x41, 0x4e, 0x53, 0x49, 0x45, 0x4e,
	0x54, 0x10, 0x02, 0x12, 0x14, 0x0a, 0x10, 0x46, 0x41, 0x49, 0x4c, 0x45, 0x44, 0x5f, 0x50, 0x45,
	0x52, 0x4d, 0x41, 0x4e, 0x45, 0x4e, 0x54, 0x10, 0x03, 0x42, 0x36, 0x5a, 0x34, 0x67, 0x6f, 0x2e,
	0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63,
	0x69, 0x2f, 0x63, 0x76, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x72, 0x75,
	0x6e, 0x2f, 0x65, 0x76, 0x65, 0x6e, 0x74, 0x70, 0x62, 0x3b, 0x65, 0x76, 0x65, 0x6e, 0x74, 0x70,
	0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_rawDescData []byte
)

func file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_rawDesc), len(file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_rawDescData
}

var file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_goTypes = []any{
	(SubmissionResult)(0),                            // 0: cv.internal.run.eventpb.SubmissionResult
	(*SubmissionCompleted)(nil),                      // 1: cv.internal.run.eventpb.SubmissionCompleted
	(*SubmissionCompleted_CLSubmissionFailure)(nil),  // 2: cv.internal.run.eventpb.SubmissionCompleted.CLSubmissionFailure
	(*SubmissionCompleted_CLSubmissionFailures)(nil), // 3: cv.internal.run.eventpb.SubmissionCompleted.CLSubmissionFailures
	(*timestamppb.Timestamp)(nil),                    // 4: google.protobuf.Timestamp
}
var file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_depIdxs = []int32{
	0, // 0: cv.internal.run.eventpb.SubmissionCompleted.result:type_name -> cv.internal.run.eventpb.SubmissionResult
	4, // 1: cv.internal.run.eventpb.SubmissionCompleted.queue_release_timestamp:type_name -> google.protobuf.Timestamp
	3, // 2: cv.internal.run.eventpb.SubmissionCompleted.cl_failures:type_name -> cv.internal.run.eventpb.SubmissionCompleted.CLSubmissionFailures
	2, // 3: cv.internal.run.eventpb.SubmissionCompleted.CLSubmissionFailures.failures:type_name -> cv.internal.run.eventpb.SubmissionCompleted.CLSubmissionFailure
	4, // [4:4] is the sub-list for method output_type
	4, // [4:4] is the sub-list for method input_type
	4, // [4:4] is the sub-list for extension type_name
	4, // [4:4] is the sub-list for extension extendee
	0, // [0:4] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_init() }
func file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_init() {
	if File_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto != nil {
		return
	}
	file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_msgTypes[0].OneofWrappers = []any{
		(*SubmissionCompleted_Timeout)(nil),
		(*SubmissionCompleted_ClFailures)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_rawDesc), len(file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_rawDesc)),
			NumEnums:      1,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_depIdxs,
		EnumInfos:         file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_enumTypes,
		MessageInfos:      file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto = out.File
	file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_goTypes = nil
	file_go_chromium_org_luci_cv_internal_run_eventpb_submission_proto_depIdxs = nil
}
