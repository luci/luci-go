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
// 	protoc-gen-go v1.36.5
// 	protoc        v6.30.2
// source: go.chromium.org/luci/tree_status/proto/v1/tree_status.proto

package v1

import prpc "go.chromium.org/luci/grpc/prpc"

import (
	context "context"
	_ "google.golang.org/genproto/googleapis/api/annotations"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
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

// GeneralState are the possible states for a tree to be in.
type GeneralState int32

const (
	// GeneralState was not specified.
	// This should not be used, it is the default value for an unset field.
	GeneralState_GENERAL_STATE_UNSPECIFIED GeneralState = 0
	// The tree is open and accepting new commits.
	GeneralState_OPEN GeneralState = 1
	// The tree is closed, no new commits are currently being accepted.
	GeneralState_CLOSED GeneralState = 2
	// The tree is throttled.  The meaning of this state can vary by project,
	// but generally it is between the open and closed states.
	GeneralState_THROTTLED GeneralState = 3
	// The tree is in maintenance.  Generally CLs will not be accepted while the
	// tree is in this state.
	GeneralState_MAINTENANCE GeneralState = 4
)

// Enum value maps for GeneralState.
var (
	GeneralState_name = map[int32]string{
		0: "GENERAL_STATE_UNSPECIFIED",
		1: "OPEN",
		2: "CLOSED",
		3: "THROTTLED",
		4: "MAINTENANCE",
	}
	GeneralState_value = map[string]int32{
		"GENERAL_STATE_UNSPECIFIED": 0,
		"OPEN":                      1,
		"CLOSED":                    2,
		"THROTTLED":                 3,
		"MAINTENANCE":               4,
	}
)

func (x GeneralState) Enum() *GeneralState {
	p := new(GeneralState)
	*p = x
	return p
}

func (x GeneralState) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (GeneralState) Descriptor() protoreflect.EnumDescriptor {
	return file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_enumTypes[0].Descriptor()
}

func (GeneralState) Type() protoreflect.EnumType {
	return &file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_enumTypes[0]
}

func (x GeneralState) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use GeneralState.Descriptor instead.
func (GeneralState) EnumDescriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_rawDescGZIP(), []int{0}
}

type GetStatusRequest struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The status value to get.
	//
	// You can use 'latest' as the id to get the latest status for a tree,
	// i.e. set the name to 'trees/{tree_id}/status/latest'.
	//
	// If you request the 'latest' status and no status updates are in the
	// database (possibly due to the 140 day TTL), a fallback status will
	// be returned with general_state OPEN.  You can tell that the fallback
	// status was returned by checking the name which will be
	// 'trees/{tree_id}/status/fallback', which is otherwise not a valid name.
	//
	// Format: trees/{tree_id}/status/{status_id}
	Name          string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *GetStatusRequest) Reset() {
	*x = GetStatusRequest{}
	mi := &file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetStatusRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetStatusRequest) ProtoMessage() {}

func (x *GetStatusRequest) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetStatusRequest.ProtoReflect.Descriptor instead.
func (*GetStatusRequest) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_rawDescGZIP(), []int{0}
}

func (x *GetStatusRequest) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

// The Status of a tree for an interval of time.
type Status struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The name of this status.
	// Format: trees/{tree_id}/status/{status_id}
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// The general state of the tree.  Possible values are open, closed, throttled
	// and maintenance.
	GeneralState GeneralState `protobuf:"varint,2,opt,name=general_state,json=generalState,proto3,enum=luci.tree_status.v1.GeneralState" json:"general_state,omitempty"`
	// The message explaining details about the status.  This may contain HTML,
	// it is the responsibility of the caller to sanitize the HTML before display.
	// Maximum length of 1024 bytes.  Must be a valid UTF-8 string in normalized form
	// C without any non-printable runes.
	Message string `protobuf:"bytes,3,opt,name=message,proto3" json:"message,omitempty"`
	// The email address of the user who added this.  May be empty if
	// the reader does not have permission to see personal data.  Will also be
	// set to 'user' after the user data TTL (of 30 days).
	CreateUser string `protobuf:"bytes,4,opt,name=create_user,json=createUser,proto3" json:"create_user,omitempty"`
	// The time the status update was made.
	CreateTime *timestamppb.Timestamp `protobuf:"bytes,5,opt,name=create_time,json=createTime,proto3" json:"create_time,omitempty"`
	// Optional. Only applicable when general_state == CLOSED.
	// If this field is set when general_state != CLOSED, it will be ignored.
	// The name of the LUCI builder that caused the tree to close.
	// Format: projects/{project}/buckets/{bucket}/builders/{builder}.
	// This field will be populated by LUCI Notify, when it automatically
	// closes a tree. When a human closes a tree, we do not require this field
	// to be set.
	// Note: If a tree is closed due to multiple builders, only the first failure
	// will be recorded.
	// This field will be exported to BigQuery for analysis.
	ClosingBuilderName string `protobuf:"bytes,6,opt,name=closing_builder_name,json=closingBuilderName,proto3" json:"closing_builder_name,omitempty"`
	unknownFields      protoimpl.UnknownFields
	sizeCache          protoimpl.SizeCache
}

func (x *Status) Reset() {
	*x = Status{}
	mi := &file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Status) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Status) ProtoMessage() {}

func (x *Status) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Status.ProtoReflect.Descriptor instead.
func (*Status) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_rawDescGZIP(), []int{1}
}

func (x *Status) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *Status) GetGeneralState() GeneralState {
	if x != nil {
		return x.GeneralState
	}
	return GeneralState_GENERAL_STATE_UNSPECIFIED
}

func (x *Status) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

func (x *Status) GetCreateUser() string {
	if x != nil {
		return x.CreateUser
	}
	return ""
}

func (x *Status) GetCreateTime() *timestamppb.Timestamp {
	if x != nil {
		return x.CreateTime
	}
	return nil
}

func (x *Status) GetClosingBuilderName() string {
	if x != nil {
		return x.ClosingBuilderName
	}
	return ""
}

type ListStatusRequest struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The parent tree which the status values belongs to.
	// Format: trees/{tree_id}/status
	Parent string `protobuf:"bytes,1,opt,name=parent,proto3" json:"parent,omitempty"`
	// The maximum number of status values to return. The service may return fewer
	// than this value. If unspecified, at most 50 status values will be returned.
	// The maximum value is 1000; values above 1000 will be coerced to 1000.
	PageSize int32 `protobuf:"varint,2,opt,name=page_size,json=pageSize,proto3" json:"page_size,omitempty"`
	// A page token, received from a previous `ListStatus` call.
	// Provide this to retrieve the subsequent page.
	//
	// When paginating, all other parameters provided to `ListStatus` must match
	// the call that provided the page token.
	PageToken     string `protobuf:"bytes,3,opt,name=page_token,json=pageToken,proto3" json:"page_token,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ListStatusRequest) Reset() {
	*x = ListStatusRequest{}
	mi := &file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ListStatusRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ListStatusRequest) ProtoMessage() {}

func (x *ListStatusRequest) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ListStatusRequest.ProtoReflect.Descriptor instead.
func (*ListStatusRequest) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_rawDescGZIP(), []int{2}
}

func (x *ListStatusRequest) GetParent() string {
	if x != nil {
		return x.Parent
	}
	return ""
}

func (x *ListStatusRequest) GetPageSize() int32 {
	if x != nil {
		return x.PageSize
	}
	return 0
}

func (x *ListStatusRequest) GetPageToken() string {
	if x != nil {
		return x.PageToken
	}
	return ""
}

type ListStatusResponse struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The status values of the tree.
	Status []*Status `protobuf:"bytes,1,rep,name=status,proto3" json:"status,omitempty"`
	// A token, which can be sent as `page_token` to retrieve the next page.
	// If this field is omitted, there are no subsequent pages.
	NextPageToken string `protobuf:"bytes,2,opt,name=next_page_token,json=nextPageToken,proto3" json:"next_page_token,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ListStatusResponse) Reset() {
	*x = ListStatusResponse{}
	mi := &file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ListStatusResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ListStatusResponse) ProtoMessage() {}

func (x *ListStatusResponse) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ListStatusResponse.ProtoReflect.Descriptor instead.
func (*ListStatusResponse) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_rawDescGZIP(), []int{3}
}

func (x *ListStatusResponse) GetStatus() []*Status {
	if x != nil {
		return x.Status
	}
	return nil
}

func (x *ListStatusResponse) GetNextPageToken() string {
	if x != nil {
		return x.NextPageToken
	}
	return ""
}

type CreateStatusRequest struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The parent tree which the status values belongs to.
	// Format: trees/{tree_id}/status
	Parent string `protobuf:"bytes,1,opt,name=parent,proto3" json:"parent,omitempty"`
	// The status to create.
	// Only the general state and message fields can be provided, the current date
	// will be used for the date and the RPC caller will be used for the username.
	Status        *Status `protobuf:"bytes,2,opt,name=status,proto3" json:"status,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *CreateStatusRequest) Reset() {
	*x = CreateStatusRequest{}
	mi := &file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_msgTypes[4]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *CreateStatusRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CreateStatusRequest) ProtoMessage() {}

func (x *CreateStatusRequest) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_msgTypes[4]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CreateStatusRequest.ProtoReflect.Descriptor instead.
func (*CreateStatusRequest) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_rawDescGZIP(), []int{4}
}

func (x *CreateStatusRequest) GetParent() string {
	if x != nil {
		return x.Parent
	}
	return ""
}

func (x *CreateStatusRequest) GetStatus() *Status {
	if x != nil {
		return x.Status
	}
	return nil
}

var File_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_rawDesc = string([]byte{
	0x0a, 0x3b, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x74, 0x72, 0x65, 0x65, 0x5f, 0x73, 0x74, 0x61, 0x74,
	0x75, 0x73, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x76, 0x31, 0x2f, 0x74, 0x72, 0x65, 0x65,
	0x5f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x13, 0x6c,
	0x75, 0x63, 0x69, 0x2e, 0x74, 0x72, 0x65, 0x65, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x2e,
	0x76, 0x31, 0x1a, 0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x66,
	0x69, 0x65, 0x6c, 0x64, 0x5f, 0x62, 0x65, 0x68, 0x61, 0x76, 0x69, 0x6f, 0x72, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x1a, 0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x62, 0x75, 0x66, 0x2f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x22, 0x26, 0x0a, 0x10, 0x47, 0x65, 0x74, 0x53, 0x74, 0x61, 0x74, 0x75,
	0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x22, 0xa0, 0x02, 0x0a,
	0x06, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x1a, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x42, 0x06, 0xe0, 0x41, 0x03, 0xe0, 0x41, 0x05, 0x52, 0x04, 0x6e,
	0x61, 0x6d, 0x65, 0x12, 0x46, 0x0a, 0x0d, 0x67, 0x65, 0x6e, 0x65, 0x72, 0x61, 0x6c, 0x5f, 0x73,
	0x74, 0x61, 0x74, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x21, 0x2e, 0x6c, 0x75, 0x63,
	0x69, 0x2e, 0x74, 0x72, 0x65, 0x65, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x2e, 0x76, 0x31,
	0x2e, 0x47, 0x65, 0x6e, 0x65, 0x72, 0x61, 0x6c, 0x53, 0x74, 0x61, 0x74, 0x65, 0x52, 0x0c, 0x67,
	0x65, 0x6e, 0x65, 0x72, 0x61, 0x6c, 0x53, 0x74, 0x61, 0x74, 0x65, 0x12, 0x18, 0x0a, 0x07, 0x6d,
	0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6d, 0x65,
	0x73, 0x73, 0x61, 0x67, 0x65, 0x12, 0x24, 0x0a, 0x0b, 0x63, 0x72, 0x65, 0x61, 0x74, 0x65, 0x5f,
	0x75, 0x73, 0x65, 0x72, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x42, 0x03, 0xe0, 0x41, 0x03, 0x52,
	0x0a, 0x63, 0x72, 0x65, 0x61, 0x74, 0x65, 0x55, 0x73, 0x65, 0x72, 0x12, 0x40, 0x0a, 0x0b, 0x63,
	0x72, 0x65, 0x61, 0x74, 0x65, 0x5f, 0x74, 0x69, 0x6d, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62,
	0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x42, 0x03, 0xe0, 0x41,
	0x03, 0x52, 0x0a, 0x63, 0x72, 0x65, 0x61, 0x74, 0x65, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x30, 0x0a,
	0x14, 0x63, 0x6c, 0x6f, 0x73, 0x69, 0x6e, 0x67, 0x5f, 0x62, 0x75, 0x69, 0x6c, 0x64, 0x65, 0x72,
	0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x06, 0x20, 0x01, 0x28, 0x09, 0x52, 0x12, 0x63, 0x6c, 0x6f,
	0x73, 0x69, 0x6e, 0x67, 0x42, 0x75, 0x69, 0x6c, 0x64, 0x65, 0x72, 0x4e, 0x61, 0x6d, 0x65, 0x22,
	0x67, 0x0a, 0x11, 0x4c, 0x69, 0x73, 0x74, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x71,
	0x75, 0x65, 0x73, 0x74, 0x12, 0x16, 0x0a, 0x06, 0x70, 0x61, 0x72, 0x65, 0x6e, 0x74, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x70, 0x61, 0x72, 0x65, 0x6e, 0x74, 0x12, 0x1b, 0x0a, 0x09,
	0x70, 0x61, 0x67, 0x65, 0x5f, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x05, 0x52,
	0x08, 0x70, 0x61, 0x67, 0x65, 0x53, 0x69, 0x7a, 0x65, 0x12, 0x1d, 0x0a, 0x0a, 0x70, 0x61, 0x67,
	0x65, 0x5f, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x70,
	0x61, 0x67, 0x65, 0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x22, 0x71, 0x0a, 0x12, 0x4c, 0x69, 0x73, 0x74,
	0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x33,
	0x0a, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x1b,
	0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x74, 0x72, 0x65, 0x65, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x75,
	0x73, 0x2e, 0x76, 0x31, 0x2e, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x06, 0x73, 0x74, 0x61,
	0x74, 0x75, 0x73, 0x12, 0x26, 0x0a, 0x0f, 0x6e, 0x65, 0x78, 0x74, 0x5f, 0x70, 0x61, 0x67, 0x65,
	0x5f, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0d, 0x6e, 0x65,
	0x78, 0x74, 0x50, 0x61, 0x67, 0x65, 0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x22, 0x62, 0x0a, 0x13, 0x43,
	0x72, 0x65, 0x61, 0x74, 0x65, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x12, 0x16, 0x0a, 0x06, 0x70, 0x61, 0x72, 0x65, 0x6e, 0x74, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x06, 0x70, 0x61, 0x72, 0x65, 0x6e, 0x74, 0x12, 0x33, 0x0a, 0x06, 0x73, 0x74,
	0x61, 0x74, 0x75, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1b, 0x2e, 0x6c, 0x75, 0x63,
	0x69, 0x2e, 0x74, 0x72, 0x65, 0x65, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x2e, 0x76, 0x31,
	0x2e, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x2a,
	0x63, 0x0a, 0x0c, 0x47, 0x65, 0x6e, 0x65, 0x72, 0x61, 0x6c, 0x53, 0x74, 0x61, 0x74, 0x65, 0x12,
	0x1d, 0x0a, 0x19, 0x47, 0x45, 0x4e, 0x45, 0x52, 0x41, 0x4c, 0x5f, 0x53, 0x54, 0x41, 0x54, 0x45,
	0x5f, 0x55, 0x4e, 0x53, 0x50, 0x45, 0x43, 0x49, 0x46, 0x49, 0x45, 0x44, 0x10, 0x00, 0x12, 0x08,
	0x0a, 0x04, 0x4f, 0x50, 0x45, 0x4e, 0x10, 0x01, 0x12, 0x0a, 0x0a, 0x06, 0x43, 0x4c, 0x4f, 0x53,
	0x45, 0x44, 0x10, 0x02, 0x12, 0x0d, 0x0a, 0x09, 0x54, 0x48, 0x52, 0x4f, 0x54, 0x54, 0x4c, 0x45,
	0x44, 0x10, 0x03, 0x12, 0x0f, 0x0a, 0x0b, 0x4d, 0x41, 0x49, 0x4e, 0x54, 0x45, 0x4e, 0x41, 0x4e,
	0x43, 0x45, 0x10, 0x04, 0x32, 0x99, 0x02, 0x0a, 0x0a, 0x54, 0x72, 0x65, 0x65, 0x53, 0x74, 0x61,
	0x74, 0x75, 0x73, 0x12, 0x5f, 0x0a, 0x0a, 0x4c, 0x69, 0x73, 0x74, 0x53, 0x74, 0x61, 0x74, 0x75,
	0x73, 0x12, 0x26, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x74, 0x72, 0x65, 0x65, 0x5f, 0x73, 0x74,
	0x61, 0x74, 0x75, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x4c, 0x69, 0x73, 0x74, 0x53, 0x74, 0x61, 0x74,
	0x75, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x27, 0x2e, 0x6c, 0x75, 0x63, 0x69,
	0x2e, 0x74, 0x72, 0x65, 0x65, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x2e, 0x76, 0x31, 0x2e,
	0x4c, 0x69, 0x73, 0x74, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e,
	0x73, 0x65, 0x22, 0x00, 0x12, 0x51, 0x0a, 0x09, 0x47, 0x65, 0x74, 0x53, 0x74, 0x61, 0x74, 0x75,
	0x73, 0x12, 0x25, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x74, 0x72, 0x65, 0x65, 0x5f, 0x73, 0x74,
	0x61, 0x74, 0x75, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x47, 0x65, 0x74, 0x53, 0x74, 0x61, 0x74, 0x75,
	0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x1b, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e,
	0x74, 0x72, 0x65, 0x65, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x53,
	0x74, 0x61, 0x74, 0x75, 0x73, 0x22, 0x00, 0x12, 0x57, 0x0a, 0x0c, 0x43, 0x72, 0x65, 0x61, 0x74,
	0x65, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x28, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x74,
	0x72, 0x65, 0x65, 0x5f, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x43, 0x72,
	0x65, 0x61, 0x74, 0x65, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x1a, 0x1b, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x74, 0x72, 0x65, 0x65, 0x5f, 0x73, 0x74,
	0x61, 0x74, 0x75, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x22, 0x00,
	0x42, 0x2b, 0x5a, 0x29, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e,
	0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x74, 0x72, 0x65, 0x65, 0x5f, 0x73, 0x74,
	0x61, 0x74, 0x75, 0x73, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x76, 0x31, 0x62, 0x06, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_rawDescData []byte
)

func file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_rawDesc), len(file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_rawDescData
}

var file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_msgTypes = make([]protoimpl.MessageInfo, 5)
var file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_goTypes = []any{
	(GeneralState)(0),             // 0: luci.tree_status.v1.GeneralState
	(*GetStatusRequest)(nil),      // 1: luci.tree_status.v1.GetStatusRequest
	(*Status)(nil),                // 2: luci.tree_status.v1.Status
	(*ListStatusRequest)(nil),     // 3: luci.tree_status.v1.ListStatusRequest
	(*ListStatusResponse)(nil),    // 4: luci.tree_status.v1.ListStatusResponse
	(*CreateStatusRequest)(nil),   // 5: luci.tree_status.v1.CreateStatusRequest
	(*timestamppb.Timestamp)(nil), // 6: google.protobuf.Timestamp
}
var file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_depIdxs = []int32{
	0, // 0: luci.tree_status.v1.Status.general_state:type_name -> luci.tree_status.v1.GeneralState
	6, // 1: luci.tree_status.v1.Status.create_time:type_name -> google.protobuf.Timestamp
	2, // 2: luci.tree_status.v1.ListStatusResponse.status:type_name -> luci.tree_status.v1.Status
	2, // 3: luci.tree_status.v1.CreateStatusRequest.status:type_name -> luci.tree_status.v1.Status
	3, // 4: luci.tree_status.v1.TreeStatus.ListStatus:input_type -> luci.tree_status.v1.ListStatusRequest
	1, // 5: luci.tree_status.v1.TreeStatus.GetStatus:input_type -> luci.tree_status.v1.GetStatusRequest
	5, // 6: luci.tree_status.v1.TreeStatus.CreateStatus:input_type -> luci.tree_status.v1.CreateStatusRequest
	4, // 7: luci.tree_status.v1.TreeStatus.ListStatus:output_type -> luci.tree_status.v1.ListStatusResponse
	2, // 8: luci.tree_status.v1.TreeStatus.GetStatus:output_type -> luci.tree_status.v1.Status
	2, // 9: luci.tree_status.v1.TreeStatus.CreateStatus:output_type -> luci.tree_status.v1.Status
	7, // [7:10] is the sub-list for method output_type
	4, // [4:7] is the sub-list for method input_type
	4, // [4:4] is the sub-list for extension type_name
	4, // [4:4] is the sub-list for extension extendee
	0, // [0:4] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_init() }
func file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_init() {
	if File_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_rawDesc), len(file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_rawDesc)),
			NumEnums:      1,
			NumMessages:   5,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_depIdxs,
		EnumInfos:         file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_enumTypes,
		MessageInfos:      file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto = out.File
	file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_goTypes = nil
	file_go_chromium_org_luci_tree_status_proto_v1_tree_status_proto_depIdxs = nil
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConnInterface

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion6

// TreeStatusClient is the client API for TreeStatus service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type TreeStatusClient interface {
	// List all status values for a tree in reverse chronological order.
	ListStatus(ctx context.Context, in *ListStatusRequest, opts ...grpc.CallOption) (*ListStatusResponse, error)
	// Get a status for a tree.
	// Use the resource alias 'latest' to get just the current status.
	GetStatus(ctx context.Context, in *GetStatusRequest, opts ...grpc.CallOption) (*Status, error)
	// Create a new status update for the tree.
	CreateStatus(ctx context.Context, in *CreateStatusRequest, opts ...grpc.CallOption) (*Status, error)
}
type treeStatusPRPCClient struct {
	client *prpc.Client
}

func NewTreeStatusPRPCClient(client *prpc.Client) TreeStatusClient {
	return &treeStatusPRPCClient{client}
}

func (c *treeStatusPRPCClient) ListStatus(ctx context.Context, in *ListStatusRequest, opts ...grpc.CallOption) (*ListStatusResponse, error) {
	out := new(ListStatusResponse)
	err := c.client.Call(ctx, "luci.tree_status.v1.TreeStatus", "ListStatus", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *treeStatusPRPCClient) GetStatus(ctx context.Context, in *GetStatusRequest, opts ...grpc.CallOption) (*Status, error) {
	out := new(Status)
	err := c.client.Call(ctx, "luci.tree_status.v1.TreeStatus", "GetStatus", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *treeStatusPRPCClient) CreateStatus(ctx context.Context, in *CreateStatusRequest, opts ...grpc.CallOption) (*Status, error) {
	out := new(Status)
	err := c.client.Call(ctx, "luci.tree_status.v1.TreeStatus", "CreateStatus", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type treeStatusClient struct {
	cc grpc.ClientConnInterface
}

func NewTreeStatusClient(cc grpc.ClientConnInterface) TreeStatusClient {
	return &treeStatusClient{cc}
}

func (c *treeStatusClient) ListStatus(ctx context.Context, in *ListStatusRequest, opts ...grpc.CallOption) (*ListStatusResponse, error) {
	out := new(ListStatusResponse)
	err := c.cc.Invoke(ctx, "/luci.tree_status.v1.TreeStatus/ListStatus", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *treeStatusClient) GetStatus(ctx context.Context, in *GetStatusRequest, opts ...grpc.CallOption) (*Status, error) {
	out := new(Status)
	err := c.cc.Invoke(ctx, "/luci.tree_status.v1.TreeStatus/GetStatus", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *treeStatusClient) CreateStatus(ctx context.Context, in *CreateStatusRequest, opts ...grpc.CallOption) (*Status, error) {
	out := new(Status)
	err := c.cc.Invoke(ctx, "/luci.tree_status.v1.TreeStatus/CreateStatus", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// TreeStatusServer is the server API for TreeStatus service.
type TreeStatusServer interface {
	// List all status values for a tree in reverse chronological order.
	ListStatus(context.Context, *ListStatusRequest) (*ListStatusResponse, error)
	// Get a status for a tree.
	// Use the resource alias 'latest' to get just the current status.
	GetStatus(context.Context, *GetStatusRequest) (*Status, error)
	// Create a new status update for the tree.
	CreateStatus(context.Context, *CreateStatusRequest) (*Status, error)
}

// UnimplementedTreeStatusServer can be embedded to have forward compatible implementations.
type UnimplementedTreeStatusServer struct {
}

func (*UnimplementedTreeStatusServer) ListStatus(context.Context, *ListStatusRequest) (*ListStatusResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListStatus not implemented")
}
func (*UnimplementedTreeStatusServer) GetStatus(context.Context, *GetStatusRequest) (*Status, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetStatus not implemented")
}
func (*UnimplementedTreeStatusServer) CreateStatus(context.Context, *CreateStatusRequest) (*Status, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateStatus not implemented")
}

func RegisterTreeStatusServer(s prpc.Registrar, srv TreeStatusServer) {
	s.RegisterService(&_TreeStatus_serviceDesc, srv)
}

func _TreeStatus_ListStatus_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListStatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TreeStatusServer).ListStatus(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/luci.tree_status.v1.TreeStatus/ListStatus",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TreeStatusServer).ListStatus(ctx, req.(*ListStatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TreeStatus_GetStatus_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetStatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TreeStatusServer).GetStatus(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/luci.tree_status.v1.TreeStatus/GetStatus",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TreeStatusServer).GetStatus(ctx, req.(*GetStatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TreeStatus_CreateStatus_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateStatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TreeStatusServer).CreateStatus(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/luci.tree_status.v1.TreeStatus/CreateStatus",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TreeStatusServer).CreateStatus(ctx, req.(*CreateStatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _TreeStatus_serviceDesc = grpc.ServiceDesc{
	ServiceName: "luci.tree_status.v1.TreeStatus",
	HandlerType: (*TreeStatusServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "ListStatus",
			Handler:    _TreeStatus_ListStatus_Handler,
		},
		{
			MethodName: "GetStatus",
			Handler:    _TreeStatus_GetStatus_Handler,
		},
		{
			MethodName: "CreateStatus",
			Handler:    _TreeStatus_CreateStatus_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "go.chromium.org/luci/tree_status/proto/v1/tree_status.proto",
}
