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
// source: go.chromium.org/luci/cv/internal/tryjob/task.proto

package tryjob

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

// UpdateTryjobTask checks the status of a Tryjob and updates and saves the
// Datastore entity, and notifies Runs which care about this Tryjob.
//
// It does NOT involve deciding next actions to take based on changes in Tryjob
// state; e.g. it doesn't involve triggering retries or ending the Run; the
// Tryjob Executor is responsible for this, see also ExecuteTryjobsPayload.
//
// Queue: "tryjob-update".
type UpdateTryjobTask struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// id is the Tryjob entity datastore ID. Internal to CV.
	Id int64 `protobuf:"varint,1,opt,name=id,proto3" json:"id,omitempty"`
	// external_id is the ID that identifies the Tryjob in the backend.
	// e.g. in the case of Buildbucket, it's the build ID.
	ExternalId    string `protobuf:"bytes,2,opt,name=external_id,json=externalId,proto3" json:"external_id,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *UpdateTryjobTask) Reset() {
	*x = UpdateTryjobTask{}
	mi := &file_go_chromium_org_luci_cv_internal_tryjob_task_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *UpdateTryjobTask) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*UpdateTryjobTask) ProtoMessage() {}

func (x *UpdateTryjobTask) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_cv_internal_tryjob_task_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use UpdateTryjobTask.ProtoReflect.Descriptor instead.
func (*UpdateTryjobTask) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_cv_internal_tryjob_task_proto_rawDescGZIP(), []int{0}
}

func (x *UpdateTryjobTask) GetId() int64 {
	if x != nil {
		return x.Id
	}
	return 0
}

func (x *UpdateTryjobTask) GetExternalId() string {
	if x != nil {
		return x.ExternalId
	}
	return ""
}

// CancelStaleTryjobs cancels all Tryjobs that are intended to verify the given
// CL, that are now stale because a new non-trivial patchset has been uploaded.
//
// Queue: "cancel-stale-tryjobs"
type CancelStaleTryjobsTask struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// clid is the ID that identifies a CL entity. Internal to CV.
	Clid int64 `protobuf:"varint,1,opt,name=clid,proto3" json:"clid,omitempty"`
	// previous_min_equiv_patchset is the patchset that stale tryjobs will be
	// running at.
	PreviousMinEquivPatchset int32 `protobuf:"varint,2,opt,name=previous_min_equiv_patchset,json=previousMinEquivPatchset,proto3" json:"previous_min_equiv_patchset,omitempty"`
	// current_min_equiv_patchset is the patchset at or after which the
	// associated Tryjobs are no longer considered stale.
	CurrentMinEquivPatchset int32 `protobuf:"varint,3,opt,name=current_min_equiv_patchset,json=currentMinEquivPatchset,proto3" json:"current_min_equiv_patchset,omitempty"`
	unknownFields           protoimpl.UnknownFields
	sizeCache               protoimpl.SizeCache
}

func (x *CancelStaleTryjobsTask) Reset() {
	*x = CancelStaleTryjobsTask{}
	mi := &file_go_chromium_org_luci_cv_internal_tryjob_task_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *CancelStaleTryjobsTask) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CancelStaleTryjobsTask) ProtoMessage() {}

func (x *CancelStaleTryjobsTask) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_cv_internal_tryjob_task_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CancelStaleTryjobsTask.ProtoReflect.Descriptor instead.
func (*CancelStaleTryjobsTask) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_cv_internal_tryjob_task_proto_rawDescGZIP(), []int{1}
}

func (x *CancelStaleTryjobsTask) GetClid() int64 {
	if x != nil {
		return x.Clid
	}
	return 0
}

func (x *CancelStaleTryjobsTask) GetPreviousMinEquivPatchset() int32 {
	if x != nil {
		return x.PreviousMinEquivPatchset
	}
	return 0
}

func (x *CancelStaleTryjobsTask) GetCurrentMinEquivPatchset() int32 {
	if x != nil {
		return x.CurrentMinEquivPatchset
	}
	return 0
}

// ExecuteTryjobsPayload is the payload of the long-op task that invokes
// the Tryjob Executor.
//
// The payload contains the event happens outside so that Tryjob Executor could
// react on the event.
//
// Exactly one event should be provided. Not using oneof for the sake of
// simplicity.
type ExecuteTryjobsPayload struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// RequirementChanged indicates the Tryjob Requirement of the Run has
	// changed.
	RequirementChanged bool `protobuf:"varint,1,opt,name=requirement_changed,json=requirementChanged,proto3" json:"requirement_changed,omitempty"`
	// TryjobsUpdated contains IDs of all Tryjobs that have status updates.
	TryjobsUpdated []int64 `protobuf:"varint,2,rep,packed,name=tryjobs_updated,json=tryjobsUpdated,proto3" json:"tryjobs_updated,omitempty"`
	unknownFields  protoimpl.UnknownFields
	sizeCache      protoimpl.SizeCache
}

func (x *ExecuteTryjobsPayload) Reset() {
	*x = ExecuteTryjobsPayload{}
	mi := &file_go_chromium_org_luci_cv_internal_tryjob_task_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ExecuteTryjobsPayload) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ExecuteTryjobsPayload) ProtoMessage() {}

func (x *ExecuteTryjobsPayload) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_cv_internal_tryjob_task_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ExecuteTryjobsPayload.ProtoReflect.Descriptor instead.
func (*ExecuteTryjobsPayload) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_cv_internal_tryjob_task_proto_rawDescGZIP(), []int{2}
}

func (x *ExecuteTryjobsPayload) GetRequirementChanged() bool {
	if x != nil {
		return x.RequirementChanged
	}
	return false
}

func (x *ExecuteTryjobsPayload) GetTryjobsUpdated() []int64 {
	if x != nil {
		return x.TryjobsUpdated
	}
	return nil
}

// ExecuteTryjobsResult is the result of Tryjob executor.
type ExecuteTryjobsResult struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ExecuteTryjobsResult) Reset() {
	*x = ExecuteTryjobsResult{}
	mi := &file_go_chromium_org_luci_cv_internal_tryjob_task_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ExecuteTryjobsResult) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ExecuteTryjobsResult) ProtoMessage() {}

func (x *ExecuteTryjobsResult) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_cv_internal_tryjob_task_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ExecuteTryjobsResult.ProtoReflect.Descriptor instead.
func (*ExecuteTryjobsResult) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_cv_internal_tryjob_task_proto_rawDescGZIP(), []int{3}
}

var File_go_chromium_org_luci_cv_internal_tryjob_task_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_cv_internal_tryjob_task_proto_rawDesc = string([]byte{
	0x0a, 0x32, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x63, 0x76, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e,
	0x61, 0x6c, 0x2f, 0x74, 0x72, 0x79, 0x6a, 0x6f, 0x62, 0x2f, 0x74, 0x61, 0x73, 0x6b, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x12, 0x12, 0x63, 0x76, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61,
	0x6c, 0x2e, 0x74, 0x72, 0x79, 0x6a, 0x6f, 0x62, 0x22, 0x43, 0x0a, 0x10, 0x55, 0x70, 0x64, 0x61,
	0x74, 0x65, 0x54, 0x72, 0x79, 0x6a, 0x6f, 0x62, 0x54, 0x61, 0x73, 0x6b, 0x12, 0x0e, 0x0a, 0x02,
	0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x03, 0x52, 0x02, 0x69, 0x64, 0x12, 0x1f, 0x0a, 0x0b,
	0x65, 0x78, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x0a, 0x65, 0x78, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x49, 0x64, 0x22, 0xa8, 0x01,
	0x0a, 0x16, 0x43, 0x61, 0x6e, 0x63, 0x65, 0x6c, 0x53, 0x74, 0x61, 0x6c, 0x65, 0x54, 0x72, 0x79,
	0x6a, 0x6f, 0x62, 0x73, 0x54, 0x61, 0x73, 0x6b, 0x12, 0x12, 0x0a, 0x04, 0x63, 0x6c, 0x69, 0x64,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x03, 0x52, 0x04, 0x63, 0x6c, 0x69, 0x64, 0x12, 0x3d, 0x0a, 0x1b,
	0x70, 0x72, 0x65, 0x76, 0x69, 0x6f, 0x75, 0x73, 0x5f, 0x6d, 0x69, 0x6e, 0x5f, 0x65, 0x71, 0x75,
	0x69, 0x76, 0x5f, 0x70, 0x61, 0x74, 0x63, 0x68, 0x73, 0x65, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x05, 0x52, 0x18, 0x70, 0x72, 0x65, 0x76, 0x69, 0x6f, 0x75, 0x73, 0x4d, 0x69, 0x6e, 0x45, 0x71,
	0x75, 0x69, 0x76, 0x50, 0x61, 0x74, 0x63, 0x68, 0x73, 0x65, 0x74, 0x12, 0x3b, 0x0a, 0x1a, 0x63,
	0x75, 0x72, 0x72, 0x65, 0x6e, 0x74, 0x5f, 0x6d, 0x69, 0x6e, 0x5f, 0x65, 0x71, 0x75, 0x69, 0x76,
	0x5f, 0x70, 0x61, 0x74, 0x63, 0x68, 0x73, 0x65, 0x74, 0x18, 0x03, 0x20, 0x01, 0x28, 0x05, 0x52,
	0x17, 0x63, 0x75, 0x72, 0x72, 0x65, 0x6e, 0x74, 0x4d, 0x69, 0x6e, 0x45, 0x71, 0x75, 0x69, 0x76,
	0x50, 0x61, 0x74, 0x63, 0x68, 0x73, 0x65, 0x74, 0x22, 0x71, 0x0a, 0x15, 0x45, 0x78, 0x65, 0x63,
	0x75, 0x74, 0x65, 0x54, 0x72, 0x79, 0x6a, 0x6f, 0x62, 0x73, 0x50, 0x61, 0x79, 0x6c, 0x6f, 0x61,
	0x64, 0x12, 0x2f, 0x0a, 0x13, 0x72, 0x65, 0x71, 0x75, 0x69, 0x72, 0x65, 0x6d, 0x65, 0x6e, 0x74,
	0x5f, 0x63, 0x68, 0x61, 0x6e, 0x67, 0x65, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x08, 0x52, 0x12,
	0x72, 0x65, 0x71, 0x75, 0x69, 0x72, 0x65, 0x6d, 0x65, 0x6e, 0x74, 0x43, 0x68, 0x61, 0x6e, 0x67,
	0x65, 0x64, 0x12, 0x27, 0x0a, 0x0f, 0x74, 0x72, 0x79, 0x6a, 0x6f, 0x62, 0x73, 0x5f, 0x75, 0x70,
	0x64, 0x61, 0x74, 0x65, 0x64, 0x18, 0x02, 0x20, 0x03, 0x28, 0x03, 0x52, 0x0e, 0x74, 0x72, 0x79,
	0x6a, 0x6f, 0x62, 0x73, 0x55, 0x70, 0x64, 0x61, 0x74, 0x65, 0x64, 0x22, 0x16, 0x0a, 0x14, 0x45,
	0x78, 0x65, 0x63, 0x75, 0x74, 0x65, 0x54, 0x72, 0x79, 0x6a, 0x6f, 0x62, 0x73, 0x52, 0x65, 0x73,
	0x75, 0x6c, 0x74, 0x42, 0x30, 0x5a, 0x2e, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69,
	0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x63, 0x76, 0x2f, 0x69,
	0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x74, 0x72, 0x79, 0x6a, 0x6f, 0x62, 0x3b, 0x74,
	0x72, 0x79, 0x6a, 0x6f, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_cv_internal_tryjob_task_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_cv_internal_tryjob_task_proto_rawDescData []byte
)

func file_go_chromium_org_luci_cv_internal_tryjob_task_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_cv_internal_tryjob_task_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_cv_internal_tryjob_task_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_cv_internal_tryjob_task_proto_rawDesc), len(file_go_chromium_org_luci_cv_internal_tryjob_task_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_cv_internal_tryjob_task_proto_rawDescData
}

var file_go_chromium_org_luci_cv_internal_tryjob_task_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_go_chromium_org_luci_cv_internal_tryjob_task_proto_goTypes = []any{
	(*UpdateTryjobTask)(nil),       // 0: cv.internal.tryjob.UpdateTryjobTask
	(*CancelStaleTryjobsTask)(nil), // 1: cv.internal.tryjob.CancelStaleTryjobsTask
	(*ExecuteTryjobsPayload)(nil),  // 2: cv.internal.tryjob.ExecuteTryjobsPayload
	(*ExecuteTryjobsResult)(nil),   // 3: cv.internal.tryjob.ExecuteTryjobsResult
}
var file_go_chromium_org_luci_cv_internal_tryjob_task_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_cv_internal_tryjob_task_proto_init() }
func file_go_chromium_org_luci_cv_internal_tryjob_task_proto_init() {
	if File_go_chromium_org_luci_cv_internal_tryjob_task_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_cv_internal_tryjob_task_proto_rawDesc), len(file_go_chromium_org_luci_cv_internal_tryjob_task_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_cv_internal_tryjob_task_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_cv_internal_tryjob_task_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_cv_internal_tryjob_task_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_cv_internal_tryjob_task_proto = out.File
	file_go_chromium_org_luci_cv_internal_tryjob_task_proto_goTypes = nil
	file_go_chromium_org_luci_cv_internal_tryjob_task_proto_depIdxs = nil
}
