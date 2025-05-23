// Copyright 2016 The LUCI Authors.
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
// source: go.chromium.org/luci/scheduler/api/scheduler/v1/scheduler.proto

package scheduler

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
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

type JobsRequest struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// If not specified or "", all projects' jobs are returned.
	Project string `protobuf:"bytes,1,opt,name=project,proto3" json:"project,omitempty"`
	Cursor  string `protobuf:"bytes,2,opt,name=cursor,proto3" json:"cursor,omitempty"`
	// page_size is currently not implemented and is ignored.
	PageSize      int32 `protobuf:"varint,3,opt,name=page_size,json=pageSize,proto3" json:"page_size,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *JobsRequest) Reset() {
	*x = JobsRequest{}
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *JobsRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*JobsRequest) ProtoMessage() {}

func (x *JobsRequest) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use JobsRequest.ProtoReflect.Descriptor instead.
func (*JobsRequest) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDescGZIP(), []int{0}
}

func (x *JobsRequest) GetProject() string {
	if x != nil {
		return x.Project
	}
	return ""
}

func (x *JobsRequest) GetCursor() string {
	if x != nil {
		return x.Cursor
	}
	return ""
}

func (x *JobsRequest) GetPageSize() int32 {
	if x != nil {
		return x.PageSize
	}
	return 0
}

type JobsReply struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Jobs          []*Job                 `protobuf:"bytes,1,rep,name=jobs,proto3" json:"jobs,omitempty"`
	NextCursor    string                 `protobuf:"bytes,2,opt,name=next_cursor,json=nextCursor,proto3" json:"next_cursor,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *JobsReply) Reset() {
	*x = JobsReply{}
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *JobsReply) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*JobsReply) ProtoMessage() {}

func (x *JobsReply) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use JobsReply.ProtoReflect.Descriptor instead.
func (*JobsReply) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDescGZIP(), []int{1}
}

func (x *JobsReply) GetJobs() []*Job {
	if x != nil {
		return x.Jobs
	}
	return nil
}

func (x *JobsReply) GetNextCursor() string {
	if x != nil {
		return x.NextCursor
	}
	return ""
}

type InvocationsRequest struct {
	state  protoimpl.MessageState `protogen:"open.v1"`
	JobRef *JobRef                `protobuf:"bytes,1,opt,name=job_ref,json=jobRef,proto3" json:"job_ref,omitempty"`
	Cursor string                 `protobuf:"bytes,2,opt,name=cursor,proto3" json:"cursor,omitempty"`
	// page_size defaults to 50 which is maximum.
	PageSize      int32 `protobuf:"varint,3,opt,name=page_size,json=pageSize,proto3" json:"page_size,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *InvocationsRequest) Reset() {
	*x = InvocationsRequest{}
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *InvocationsRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*InvocationsRequest) ProtoMessage() {}

func (x *InvocationsRequest) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use InvocationsRequest.ProtoReflect.Descriptor instead.
func (*InvocationsRequest) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDescGZIP(), []int{2}
}

func (x *InvocationsRequest) GetJobRef() *JobRef {
	if x != nil {
		return x.JobRef
	}
	return nil
}

func (x *InvocationsRequest) GetCursor() string {
	if x != nil {
		return x.Cursor
	}
	return ""
}

func (x *InvocationsRequest) GetPageSize() int32 {
	if x != nil {
		return x.PageSize
	}
	return 0
}

type InvocationsReply struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Invocations   []*Invocation          `protobuf:"bytes,1,rep,name=invocations,proto3" json:"invocations,omitempty"`
	NextCursor    string                 `protobuf:"bytes,2,opt,name=next_cursor,json=nextCursor,proto3" json:"next_cursor,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *InvocationsReply) Reset() {
	*x = InvocationsReply{}
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *InvocationsReply) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*InvocationsReply) ProtoMessage() {}

func (x *InvocationsReply) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use InvocationsReply.ProtoReflect.Descriptor instead.
func (*InvocationsReply) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDescGZIP(), []int{3}
}

func (x *InvocationsReply) GetInvocations() []*Invocation {
	if x != nil {
		return x.Invocations
	}
	return nil
}

func (x *InvocationsReply) GetNextCursor() string {
	if x != nil {
		return x.NextCursor
	}
	return ""
}

type EmitTriggersRequest struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// A trigger and jobs it should be delivered to.
	//
	// Order is important. Triggers that are listed earlier are considered older.
	Batches []*EmitTriggersRequest_Batch `protobuf:"bytes,1,rep,name=batches,proto3" json:"batches,omitempty"`
	// An optional timestamp to use as trigger creation time, as unix timestamp in
	// microseconds. Assigned by the server by default. If given, must be within
	// +-15 min of the current time.
	//
	// Under some conditions triggers are ordered by timestamp of when they are
	// created. By allowing the client to specify this timestamp, we make
	// EmitTrigger RPC idempotent: if EmitTrigger call fails midway, the caller
	// can retry it providing exact same timestamp to get the correct final order
	// of the triggers.
	Timestamp     int64 `protobuf:"varint,2,opt,name=timestamp,proto3" json:"timestamp,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *EmitTriggersRequest) Reset() {
	*x = EmitTriggersRequest{}
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[4]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *EmitTriggersRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*EmitTriggersRequest) ProtoMessage() {}

func (x *EmitTriggersRequest) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[4]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use EmitTriggersRequest.ProtoReflect.Descriptor instead.
func (*EmitTriggersRequest) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDescGZIP(), []int{4}
}

func (x *EmitTriggersRequest) GetBatches() []*EmitTriggersRequest_Batch {
	if x != nil {
		return x.Batches
	}
	return nil
}

func (x *EmitTriggersRequest) GetTimestamp() int64 {
	if x != nil {
		return x.Timestamp
	}
	return 0
}

// JobRef uniquely identifies a job.
type JobRef struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Project       string                 `protobuf:"bytes,1,opt,name=project,proto3" json:"project,omitempty"`
	Job           string                 `protobuf:"bytes,2,opt,name=job,proto3" json:"job,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *JobRef) Reset() {
	*x = JobRef{}
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[5]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *JobRef) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*JobRef) ProtoMessage() {}

func (x *JobRef) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[5]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use JobRef.ProtoReflect.Descriptor instead.
func (*JobRef) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDescGZIP(), []int{5}
}

func (x *JobRef) GetProject() string {
	if x != nil {
		return x.Project
	}
	return ""
}

func (x *JobRef) GetJob() string {
	if x != nil {
		return x.Job
	}
	return ""
}

// InvocationRef uniquely identifies an invocation of a job.
type InvocationRef struct {
	state  protoimpl.MessageState `protogen:"open.v1"`
	JobRef *JobRef                `protobuf:"bytes,1,opt,name=job_ref,json=jobRef,proto3" json:"job_ref,omitempty"`
	// invocation_id is a unique integer among all invocations for a given job.
	// However, there could be invocations with the same invocation_id but
	// belonging to different jobs.
	InvocationId  int64 `protobuf:"varint,2,opt,name=invocation_id,json=invocationId,proto3" json:"invocation_id,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *InvocationRef) Reset() {
	*x = InvocationRef{}
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[6]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *InvocationRef) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*InvocationRef) ProtoMessage() {}

func (x *InvocationRef) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[6]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use InvocationRef.ProtoReflect.Descriptor instead.
func (*InvocationRef) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDescGZIP(), []int{6}
}

func (x *InvocationRef) GetJobRef() *JobRef {
	if x != nil {
		return x.JobRef
	}
	return nil
}

func (x *InvocationRef) GetInvocationId() int64 {
	if x != nil {
		return x.InvocationId
	}
	return 0
}

// Job descibes currently configured job.
type Job struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	JobRef        *JobRef                `protobuf:"bytes,1,opt,name=job_ref,json=jobRef,proto3" json:"job_ref,omitempty"`
	Schedule      string                 `protobuf:"bytes,2,opt,name=schedule,proto3" json:"schedule,omitempty"`
	State         *JobState              `protobuf:"bytes,3,opt,name=state,proto3" json:"state,omitempty"`
	Paused        bool                   `protobuf:"varint,4,opt,name=paused,proto3" json:"paused,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Job) Reset() {
	*x = Job{}
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[7]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Job) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Job) ProtoMessage() {}

func (x *Job) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[7]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Job.ProtoReflect.Descriptor instead.
func (*Job) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDescGZIP(), []int{7}
}

func (x *Job) GetJobRef() *JobRef {
	if x != nil {
		return x.JobRef
	}
	return nil
}

func (x *Job) GetSchedule() string {
	if x != nil {
		return x.Schedule
	}
	return ""
}

func (x *Job) GetState() *JobState {
	if x != nil {
		return x.State
	}
	return nil
}

func (x *Job) GetPaused() bool {
	if x != nil {
		return x.Paused
	}
	return false
}

// JobState describes current Job state as one of these strings:
//
//	"DISABLED"
//	"PAUSED"
//	"RUNNING"
//	"SCHEDULED"
//	"WAITING"
type JobState struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	UiStatus      string                 `protobuf:"bytes,1,opt,name=ui_status,json=uiStatus,proto3" json:"ui_status,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *JobState) Reset() {
	*x = JobState{}
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[8]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *JobState) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*JobState) ProtoMessage() {}

func (x *JobState) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[8]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use JobState.ProtoReflect.Descriptor instead.
func (*JobState) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDescGZIP(), []int{8}
}

func (x *JobState) GetUiStatus() string {
	if x != nil {
		return x.UiStatus
	}
	return ""
}

// Invocation describes properties of one job execution.
type Invocation struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	InvocationRef *InvocationRef         `protobuf:"bytes,1,opt,name=invocation_ref,json=invocationRef,proto3" json:"invocation_ref,omitempty"`
	// start_ts is unix timestamp in microseconds.
	StartedTs int64 `protobuf:"varint,2,opt,name=started_ts,json=startedTs,proto3" json:"started_ts,omitempty"`
	// finished_ts is unix timestamp in microseconds. Set only if final is true.
	FinishedTs int64 `protobuf:"varint,3,opt,name=finished_ts,json=finishedTs,proto3" json:"finished_ts,omitempty"`
	// triggered_by is an identity ("kind:value") which is specified only if
	// invocation was triggered by not the scheduler service itself.
	TriggeredBy string `protobuf:"bytes,4,opt,name=triggered_by,json=triggeredBy,proto3" json:"triggered_by,omitempty"`
	// Latest status of a job.
	Status string `protobuf:"bytes,5,opt,name=status,proto3" json:"status,omitempty"`
	// If true, this invocation properties are final and won't be changed.
	Final bool `protobuf:"varint,6,opt,name=final,proto3" json:"final,omitempty"`
	// config_revision pins project/job config version according to which this
	// invocation was created.
	ConfigRevision string `protobuf:"bytes,7,opt,name=config_revision,json=configRevision,proto3" json:"config_revision,omitempty"`
	// view_url points to human readable page for a given invocation if available.
	ViewUrl       string `protobuf:"bytes,8,opt,name=view_url,json=viewUrl,proto3" json:"view_url,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Invocation) Reset() {
	*x = Invocation{}
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[9]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Invocation) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Invocation) ProtoMessage() {}

func (x *Invocation) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[9]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Invocation.ProtoReflect.Descriptor instead.
func (*Invocation) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDescGZIP(), []int{9}
}

func (x *Invocation) GetInvocationRef() *InvocationRef {
	if x != nil {
		return x.InvocationRef
	}
	return nil
}

func (x *Invocation) GetStartedTs() int64 {
	if x != nil {
		return x.StartedTs
	}
	return 0
}

func (x *Invocation) GetFinishedTs() int64 {
	if x != nil {
		return x.FinishedTs
	}
	return 0
}

func (x *Invocation) GetTriggeredBy() string {
	if x != nil {
		return x.TriggeredBy
	}
	return ""
}

func (x *Invocation) GetStatus() string {
	if x != nil {
		return x.Status
	}
	return ""
}

func (x *Invocation) GetFinal() bool {
	if x != nil {
		return x.Final
	}
	return false
}

func (x *Invocation) GetConfigRevision() string {
	if x != nil {
		return x.ConfigRevision
	}
	return ""
}

func (x *Invocation) GetViewUrl() string {
	if x != nil {
		return x.ViewUrl
	}
	return ""
}

type EmitTriggersRequest_Batch struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Trigger       *Trigger               `protobuf:"bytes,1,opt,name=trigger,proto3" json:"trigger,omitempty"`
	Jobs          []*JobRef              `protobuf:"bytes,2,rep,name=jobs,proto3" json:"jobs,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *EmitTriggersRequest_Batch) Reset() {
	*x = EmitTriggersRequest_Batch{}
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[10]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *EmitTriggersRequest_Batch) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*EmitTriggersRequest_Batch) ProtoMessage() {}

func (x *EmitTriggersRequest_Batch) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes[10]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use EmitTriggersRequest_Batch.ProtoReflect.Descriptor instead.
func (*EmitTriggersRequest_Batch) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDescGZIP(), []int{4, 0}
}

func (x *EmitTriggersRequest_Batch) GetTrigger() *Trigger {
	if x != nil {
		return x.Trigger
	}
	return nil
}

func (x *EmitTriggersRequest_Batch) GetJobs() []*JobRef {
	if x != nil {
		return x.Jobs
	}
	return nil
}

var File_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDesc = string([]byte{
	0x0a, 0x3f, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72,
	0x2f, 0x61, 0x70, 0x69, 0x2f, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x2f, 0x76,
	0x31, 0x2f, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x12, 0x09, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x1a, 0x1b, 0x67, 0x6f,
	0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x65, 0x6d,
	0x70, 0x74, 0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x3e, 0x67, 0x6f, 0x2e, 0x63, 0x68,
	0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f,
	0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x73, 0x63,
	0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x2f, 0x76, 0x31, 0x2f, 0x74, 0x72, 0x69, 0x67, 0x67,
	0x65, 0x72, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x5c, 0x0a, 0x0b, 0x4a, 0x6f, 0x62,
	0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x18, 0x0a, 0x07, 0x70, 0x72, 0x6f, 0x6a,
	0x65, 0x63, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x70, 0x72, 0x6f, 0x6a, 0x65,
	0x63, 0x74, 0x12, 0x16, 0x0a, 0x06, 0x63, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x06, 0x63, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x12, 0x1b, 0x0a, 0x09, 0x70, 0x61,
	0x67, 0x65, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x05, 0x52, 0x08, 0x70,
	0x61, 0x67, 0x65, 0x53, 0x69, 0x7a, 0x65, 0x22, 0x50, 0x0a, 0x09, 0x4a, 0x6f, 0x62, 0x73, 0x52,
	0x65, 0x70, 0x6c, 0x79, 0x12, 0x22, 0x0a, 0x04, 0x6a, 0x6f, 0x62, 0x73, 0x18, 0x01, 0x20, 0x03,
	0x28, 0x0b, 0x32, 0x0e, 0x2e, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x2e, 0x4a,
	0x6f, 0x62, 0x52, 0x04, 0x6a, 0x6f, 0x62, 0x73, 0x12, 0x1f, 0x0a, 0x0b, 0x6e, 0x65, 0x78, 0x74,
	0x5f, 0x63, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x6e,
	0x65, 0x78, 0x74, 0x43, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x22, 0x75, 0x0a, 0x12, 0x49, 0x6e, 0x76,
	0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12,
	0x2a, 0x0a, 0x07, 0x6a, 0x6f, 0x62, 0x5f, 0x72, 0x65, 0x66, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x11, 0x2e, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x2e, 0x4a, 0x6f, 0x62,
	0x52, 0x65, 0x66, 0x52, 0x06, 0x6a, 0x6f, 0x62, 0x52, 0x65, 0x66, 0x12, 0x16, 0x0a, 0x06, 0x63,
	0x75, 0x72, 0x73, 0x6f, 0x72, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x63, 0x75, 0x72,
	0x73, 0x6f, 0x72, 0x12, 0x1b, 0x0a, 0x09, 0x70, 0x61, 0x67, 0x65, 0x5f, 0x73, 0x69, 0x7a, 0x65,
	0x18, 0x03, 0x20, 0x01, 0x28, 0x05, 0x52, 0x08, 0x70, 0x61, 0x67, 0x65, 0x53, 0x69, 0x7a, 0x65,
	0x22, 0x6c, 0x0a, 0x10, 0x49, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x52,
	0x65, 0x70, 0x6c, 0x79, 0x12, 0x37, 0x0a, 0x0b, 0x69, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74, 0x69,
	0x6f, 0x6e, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x15, 0x2e, 0x73, 0x63, 0x68, 0x65,
	0x64, 0x75, 0x6c, 0x65, 0x72, 0x2e, 0x49, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e,
	0x52, 0x0b, 0x69, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x12, 0x1f, 0x0a,
	0x0b, 0x6e, 0x65, 0x78, 0x74, 0x5f, 0x63, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x0a, 0x6e, 0x65, 0x78, 0x74, 0x43, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x22, 0xd1,
	0x01, 0x0a, 0x13, 0x45, 0x6d, 0x69, 0x74, 0x54, 0x72, 0x69, 0x67, 0x67, 0x65, 0x72, 0x73, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x3e, 0x0a, 0x07, 0x62, 0x61, 0x74, 0x63, 0x68, 0x65,
	0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x24, 0x2e, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75,
	0x6c, 0x65, 0x72, 0x2e, 0x45, 0x6d, 0x69, 0x74, 0x54, 0x72, 0x69, 0x67, 0x67, 0x65, 0x72, 0x73,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x2e, 0x42, 0x61, 0x74, 0x63, 0x68, 0x52, 0x07, 0x62,
	0x61, 0x74, 0x63, 0x68, 0x65, 0x73, 0x12, 0x1c, 0x0a, 0x09, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74,
	0x61, 0x6d, 0x70, 0x18, 0x02, 0x20, 0x01, 0x28, 0x03, 0x52, 0x09, 0x74, 0x69, 0x6d, 0x65, 0x73,
	0x74, 0x61, 0x6d, 0x70, 0x1a, 0x5c, 0x0a, 0x05, 0x42, 0x61, 0x74, 0x63, 0x68, 0x12, 0x2c, 0x0a,
	0x07, 0x74, 0x72, 0x69, 0x67, 0x67, 0x65, 0x72, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x12,
	0x2e, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x2e, 0x54, 0x72, 0x69, 0x67, 0x67,
	0x65, 0x72, 0x52, 0x07, 0x74, 0x72, 0x69, 0x67, 0x67, 0x65, 0x72, 0x12, 0x25, 0x0a, 0x04, 0x6a,
	0x6f, 0x62, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x11, 0x2e, 0x73, 0x63, 0x68, 0x65,
	0x64, 0x75, 0x6c, 0x65, 0x72, 0x2e, 0x4a, 0x6f, 0x62, 0x52, 0x65, 0x66, 0x52, 0x04, 0x6a, 0x6f,
	0x62, 0x73, 0x22, 0x34, 0x0a, 0x06, 0x4a, 0x6f, 0x62, 0x52, 0x65, 0x66, 0x12, 0x18, 0x0a, 0x07,
	0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x70,
	0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x12, 0x10, 0x0a, 0x03, 0x6a, 0x6f, 0x62, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x03, 0x6a, 0x6f, 0x62, 0x22, 0x60, 0x0a, 0x0d, 0x49, 0x6e, 0x76, 0x6f,
	0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x65, 0x66, 0x12, 0x2a, 0x0a, 0x07, 0x6a, 0x6f, 0x62,
	0x5f, 0x72, 0x65, 0x66, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x11, 0x2e, 0x73, 0x63, 0x68,
	0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x2e, 0x4a, 0x6f, 0x62, 0x52, 0x65, 0x66, 0x52, 0x06, 0x6a,
	0x6f, 0x62, 0x52, 0x65, 0x66, 0x12, 0x23, 0x0a, 0x0d, 0x69, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74,
	0x69, 0x6f, 0x6e, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x03, 0x52, 0x0c, 0x69, 0x6e,
	0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x49, 0x64, 0x22, 0x90, 0x01, 0x0a, 0x03, 0x4a,
	0x6f, 0x62, 0x12, 0x2a, 0x0a, 0x07, 0x6a, 0x6f, 0x62, 0x5f, 0x72, 0x65, 0x66, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x11, 0x2e, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x2e,
	0x4a, 0x6f, 0x62, 0x52, 0x65, 0x66, 0x52, 0x06, 0x6a, 0x6f, 0x62, 0x52, 0x65, 0x66, 0x12, 0x1a,
	0x0a, 0x08, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x08, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x12, 0x29, 0x0a, 0x05, 0x73, 0x74,
	0x61, 0x74, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x13, 0x2e, 0x73, 0x63, 0x68, 0x65,
	0x64, 0x75, 0x6c, 0x65, 0x72, 0x2e, 0x4a, 0x6f, 0x62, 0x53, 0x74, 0x61, 0x74, 0x65, 0x52, 0x05,
	0x73, 0x74, 0x61, 0x74, 0x65, 0x12, 0x16, 0x0a, 0x06, 0x70, 0x61, 0x75, 0x73, 0x65, 0x64, 0x18,
	0x04, 0x20, 0x01, 0x28, 0x08, 0x52, 0x06, 0x70, 0x61, 0x75, 0x73, 0x65, 0x64, 0x22, 0x27, 0x0a,
	0x08, 0x4a, 0x6f, 0x62, 0x53, 0x74, 0x61, 0x74, 0x65, 0x12, 0x1b, 0x0a, 0x09, 0x75, 0x69, 0x5f,
	0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x75, 0x69,
	0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x22, 0xa2, 0x02, 0x0a, 0x0a, 0x49, 0x6e, 0x76, 0x6f, 0x63,
	0x61, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x3f, 0x0a, 0x0e, 0x69, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74,
	0x69, 0x6f, 0x6e, 0x5f, 0x72, 0x65, 0x66, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x18, 0x2e,
	0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x2e, 0x49, 0x6e, 0x76, 0x6f, 0x63, 0x61,
	0x74, 0x69, 0x6f, 0x6e, 0x52, 0x65, 0x66, 0x52, 0x0d, 0x69, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74,
	0x69, 0x6f, 0x6e, 0x52, 0x65, 0x66, 0x12, 0x1d, 0x0a, 0x0a, 0x73, 0x74, 0x61, 0x72, 0x74, 0x65,
	0x64, 0x5f, 0x74, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28, 0x03, 0x52, 0x09, 0x73, 0x74, 0x61, 0x72,
	0x74, 0x65, 0x64, 0x54, 0x73, 0x12, 0x1f, 0x0a, 0x0b, 0x66, 0x69, 0x6e, 0x69, 0x73, 0x68, 0x65,
	0x64, 0x5f, 0x74, 0x73, 0x18, 0x03, 0x20, 0x01, 0x28, 0x03, 0x52, 0x0a, 0x66, 0x69, 0x6e, 0x69,
	0x73, 0x68, 0x65, 0x64, 0x54, 0x73, 0x12, 0x21, 0x0a, 0x0c, 0x74, 0x72, 0x69, 0x67, 0x67, 0x65,
	0x72, 0x65, 0x64, 0x5f, 0x62, 0x79, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b, 0x74, 0x72,
	0x69, 0x67, 0x67, 0x65, 0x72, 0x65, 0x64, 0x42, 0x79, 0x12, 0x16, 0x0a, 0x06, 0x73, 0x74, 0x61,
	0x74, 0x75, 0x73, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75,
	0x73, 0x12, 0x14, 0x0a, 0x05, 0x66, 0x69, 0x6e, 0x61, 0x6c, 0x18, 0x06, 0x20, 0x01, 0x28, 0x08,
	0x52, 0x05, 0x66, 0x69, 0x6e, 0x61, 0x6c, 0x12, 0x27, 0x0a, 0x0f, 0x63, 0x6f, 0x6e, 0x66, 0x69,
	0x67, 0x5f, 0x72, 0x65, 0x76, 0x69, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x07, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x0e, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x65, 0x76, 0x69, 0x73, 0x69, 0x6f, 0x6e,
	0x12, 0x19, 0x0a, 0x08, 0x76, 0x69, 0x65, 0x77, 0x5f, 0x75, 0x72, 0x6c, 0x18, 0x08, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x07, 0x76, 0x69, 0x65, 0x77, 0x55, 0x72, 0x6c, 0x32, 0x87, 0x04, 0x0a, 0x09,
	0x53, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x12, 0x37, 0x0a, 0x07, 0x47, 0x65, 0x74,
	0x4a, 0x6f, 0x62, 0x73, 0x12, 0x16, 0x2e, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72,
	0x2e, 0x4a, 0x6f, 0x62, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x14, 0x2e, 0x73,
	0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x2e, 0x4a, 0x6f, 0x62, 0x73, 0x52, 0x65, 0x70,
	0x6c, 0x79, 0x12, 0x4c, 0x0a, 0x0e, 0x47, 0x65, 0x74, 0x49, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74,
	0x69, 0x6f, 0x6e, 0x73, 0x12, 0x1d, 0x2e, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72,
	0x2e, 0x49, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x1a, 0x1b, 0x2e, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x2e,
	0x49, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x52, 0x65, 0x70, 0x6c, 0x79,
	0x12, 0x40, 0x0a, 0x0d, 0x47, 0x65, 0x74, 0x49, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f,
	0x6e, 0x12, 0x18, 0x2e, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x2e, 0x49, 0x6e,
	0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x65, 0x66, 0x1a, 0x15, 0x2e, 0x73, 0x63,
	0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x2e, 0x49, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74, 0x69,
	0x6f, 0x6e, 0x12, 0x35, 0x0a, 0x08, 0x50, 0x61, 0x75, 0x73, 0x65, 0x4a, 0x6f, 0x62, 0x12, 0x11,
	0x2e, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x2e, 0x4a, 0x6f, 0x62, 0x52, 0x65,
	0x66, 0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x12, 0x36, 0x0a, 0x09, 0x52, 0x65, 0x73,
	0x75, 0x6d, 0x65, 0x4a, 0x6f, 0x62, 0x12, 0x11, 0x2e, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c,
	0x65, 0x72, 0x2e, 0x4a, 0x6f, 0x62, 0x52, 0x65, 0x66, 0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74,
	0x79, 0x12, 0x35, 0x0a, 0x08, 0x41, 0x62, 0x6f, 0x72, 0x74, 0x4a, 0x6f, 0x62, 0x12, 0x11, 0x2e,
	0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x2e, 0x4a, 0x6f, 0x62, 0x52, 0x65, 0x66,
	0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62,
	0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x12, 0x43, 0x0a, 0x0f, 0x41, 0x62, 0x6f, 0x72,
	0x74, 0x49, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x18, 0x2e, 0x73, 0x63,
	0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x2e, 0x49, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74, 0x69,
	0x6f, 0x6e, 0x52, 0x65, 0x66, 0x1a, 0x16, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x45, 0x6d, 0x70, 0x74, 0x79, 0x12, 0x46, 0x0a,
	0x0c, 0x45, 0x6d, 0x69, 0x74, 0x54, 0x72, 0x69, 0x67, 0x67, 0x65, 0x72, 0x73, 0x12, 0x1e, 0x2e,
	0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x2e, 0x45, 0x6d, 0x69, 0x74, 0x54, 0x72,
	0x69, 0x67, 0x67, 0x65, 0x72, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x16, 0x2e,
	0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e,
	0x45, 0x6d, 0x70, 0x74, 0x79, 0x42, 0x3b, 0x5a, 0x39, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f,
	0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x73, 0x63,
	0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x72, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x73, 0x63, 0x68, 0x65,
	0x64, 0x75, 0x6c, 0x65, 0x72, 0x2f, 0x76, 0x31, 0x3b, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c,
	0x65, 0x72, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDescData []byte
)

func file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDesc), len(file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDescData
}

var file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes = make([]protoimpl.MessageInfo, 11)
var file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_goTypes = []any{
	(*JobsRequest)(nil),               // 0: scheduler.JobsRequest
	(*JobsReply)(nil),                 // 1: scheduler.JobsReply
	(*InvocationsRequest)(nil),        // 2: scheduler.InvocationsRequest
	(*InvocationsReply)(nil),          // 3: scheduler.InvocationsReply
	(*EmitTriggersRequest)(nil),       // 4: scheduler.EmitTriggersRequest
	(*JobRef)(nil),                    // 5: scheduler.JobRef
	(*InvocationRef)(nil),             // 6: scheduler.InvocationRef
	(*Job)(nil),                       // 7: scheduler.Job
	(*JobState)(nil),                  // 8: scheduler.JobState
	(*Invocation)(nil),                // 9: scheduler.Invocation
	(*EmitTriggersRequest_Batch)(nil), // 10: scheduler.EmitTriggersRequest.Batch
	(*Trigger)(nil),                   // 11: scheduler.Trigger
	(*emptypb.Empty)(nil),             // 12: google.protobuf.Empty
}
var file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_depIdxs = []int32{
	7,  // 0: scheduler.JobsReply.jobs:type_name -> scheduler.Job
	5,  // 1: scheduler.InvocationsRequest.job_ref:type_name -> scheduler.JobRef
	9,  // 2: scheduler.InvocationsReply.invocations:type_name -> scheduler.Invocation
	10, // 3: scheduler.EmitTriggersRequest.batches:type_name -> scheduler.EmitTriggersRequest.Batch
	5,  // 4: scheduler.InvocationRef.job_ref:type_name -> scheduler.JobRef
	5,  // 5: scheduler.Job.job_ref:type_name -> scheduler.JobRef
	8,  // 6: scheduler.Job.state:type_name -> scheduler.JobState
	6,  // 7: scheduler.Invocation.invocation_ref:type_name -> scheduler.InvocationRef
	11, // 8: scheduler.EmitTriggersRequest.Batch.trigger:type_name -> scheduler.Trigger
	5,  // 9: scheduler.EmitTriggersRequest.Batch.jobs:type_name -> scheduler.JobRef
	0,  // 10: scheduler.Scheduler.GetJobs:input_type -> scheduler.JobsRequest
	2,  // 11: scheduler.Scheduler.GetInvocations:input_type -> scheduler.InvocationsRequest
	6,  // 12: scheduler.Scheduler.GetInvocation:input_type -> scheduler.InvocationRef
	5,  // 13: scheduler.Scheduler.PauseJob:input_type -> scheduler.JobRef
	5,  // 14: scheduler.Scheduler.ResumeJob:input_type -> scheduler.JobRef
	5,  // 15: scheduler.Scheduler.AbortJob:input_type -> scheduler.JobRef
	6,  // 16: scheduler.Scheduler.AbortInvocation:input_type -> scheduler.InvocationRef
	4,  // 17: scheduler.Scheduler.EmitTriggers:input_type -> scheduler.EmitTriggersRequest
	1,  // 18: scheduler.Scheduler.GetJobs:output_type -> scheduler.JobsReply
	3,  // 19: scheduler.Scheduler.GetInvocations:output_type -> scheduler.InvocationsReply
	9,  // 20: scheduler.Scheduler.GetInvocation:output_type -> scheduler.Invocation
	12, // 21: scheduler.Scheduler.PauseJob:output_type -> google.protobuf.Empty
	12, // 22: scheduler.Scheduler.ResumeJob:output_type -> google.protobuf.Empty
	12, // 23: scheduler.Scheduler.AbortJob:output_type -> google.protobuf.Empty
	12, // 24: scheduler.Scheduler.AbortInvocation:output_type -> google.protobuf.Empty
	12, // 25: scheduler.Scheduler.EmitTriggers:output_type -> google.protobuf.Empty
	18, // [18:26] is the sub-list for method output_type
	10, // [10:18] is the sub-list for method input_type
	10, // [10:10] is the sub-list for extension type_name
	10, // [10:10] is the sub-list for extension extendee
	0,  // [0:10] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_init() }
func file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_init() {
	if File_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto != nil {
		return
	}
	file_go_chromium_org_luci_scheduler_api_scheduler_v1_triggers_proto_init()
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDesc), len(file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   11,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto = out.File
	file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_goTypes = nil
	file_go_chromium_org_luci_scheduler_api_scheduler_v1_scheduler_proto_depIdxs = nil
}
