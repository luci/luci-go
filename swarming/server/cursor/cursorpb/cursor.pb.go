// Copyright 2024 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.5
// 	protoc        v6.30.2
// source: go.chromium.org/luci/swarming/server/cursor/cursorpb/cursor.proto

package cursorpb

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

// What request a cursor is for. Indirectly identifies what branch of the
// `payload` oneof is expected to be set.
type RequestKind int32

const (
	RequestKind_REQUEST_KIND_UNSPECIFIED RequestKind = 0
	RequestKind_LIST_BOT_EVENTS          RequestKind = 1 // uses OpaqueCursor
	RequestKind_LIST_BOT_TASKS           RequestKind = 2 // uses OpaqueCursor
	RequestKind_LIST_BOTS                RequestKind = 3 // uses BotsCursor
	RequestKind_LIST_TASKS               RequestKind = 4 // uses TasksCursor
	RequestKind_LIST_TASK_REQUESTS       RequestKind = 5 // uses TasksCursor
	RequestKind_CANCEL_TASKS             RequestKind = 6 // uses TasksCursor
)

// Enum value maps for RequestKind.
var (
	RequestKind_name = map[int32]string{
		0: "REQUEST_KIND_UNSPECIFIED",
		1: "LIST_BOT_EVENTS",
		2: "LIST_BOT_TASKS",
		3: "LIST_BOTS",
		4: "LIST_TASKS",
		5: "LIST_TASK_REQUESTS",
		6: "CANCEL_TASKS",
	}
	RequestKind_value = map[string]int32{
		"REQUEST_KIND_UNSPECIFIED": 0,
		"LIST_BOT_EVENTS":          1,
		"LIST_BOT_TASKS":           2,
		"LIST_BOTS":                3,
		"LIST_TASKS":               4,
		"LIST_TASK_REQUESTS":       5,
		"CANCEL_TASKS":             6,
	}
)

func (x RequestKind) Enum() *RequestKind {
	p := new(RequestKind)
	*p = x
	return p
}

func (x RequestKind) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (RequestKind) Descriptor() protoreflect.EnumDescriptor {
	return file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_enumTypes[0].Descriptor()
}

func (RequestKind) Type() protoreflect.EnumType {
	return &file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_enumTypes[0]
}

func (x RequestKind) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use RequestKind.Descriptor instead.
func (RequestKind) EnumDescriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_rawDescGZIP(), []int{0}
}

// Cursor is serialized, encrypted and base64-encoded before it is sent to the
// user.
//
// There are two reasons this exists:
//  1. To support "custom" (not native Cloud Datastore) cursors for queries
//     that use "|" tag filters. Such queries are executed by running a bunch of
//     datastore queries in parallel and merging their results. There is no
//     simple working generic mechanism for paginating through such merged
//     output. A custom cursor allows to exploit a known structure of Swarming
//     queries to implement a simple, but Swarming specific, cursor.
//  2. To make cursors produced by the Go implementation be easily distinguished
//     from cursors produced by the Python implementation. That allows us to
//     implement a "smart" traffic router that routes RPC calls with Go cursors
//     to the Go implementation.
//
// Encryption allows us not to worry about leaking any data or exposing internal
// implementation details or worry about users manually messing with cursors.
type Cursor struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// What request this cursor is for. Indirectly identifies what branch of the
	// `payload` oneof is expected to be set.
	Request RequestKind `protobuf:"varint,1,opt,name=request,proto3,enum=swarming.internals.cursor.RequestKind" json:"request,omitempty"`
	// When the cursor was created (for debugging).
	Created *timestamppb.Timestamp `protobuf:"bytes,2,opt,name=created,proto3" json:"created,omitempty"`
	// Possible kinds of cursors, based on RequestKind.
	//
	// Types that are valid to be assigned to Payload:
	//
	//	*Cursor_OpaqueCursor
	//	*Cursor_BotsCursor
	//	*Cursor_TasksCursor
	Payload       isCursor_Payload `protobuf_oneof:"payload"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Cursor) Reset() {
	*x = Cursor{}
	mi := &file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Cursor) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Cursor) ProtoMessage() {}

func (x *Cursor) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Cursor.ProtoReflect.Descriptor instead.
func (*Cursor) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_rawDescGZIP(), []int{0}
}

func (x *Cursor) GetRequest() RequestKind {
	if x != nil {
		return x.Request
	}
	return RequestKind_REQUEST_KIND_UNSPECIFIED
}

func (x *Cursor) GetCreated() *timestamppb.Timestamp {
	if x != nil {
		return x.Created
	}
	return nil
}

func (x *Cursor) GetPayload() isCursor_Payload {
	if x != nil {
		return x.Payload
	}
	return nil
}

func (x *Cursor) GetOpaqueCursor() *OpaqueCursor {
	if x != nil {
		if x, ok := x.Payload.(*Cursor_OpaqueCursor); ok {
			return x.OpaqueCursor
		}
	}
	return nil
}

func (x *Cursor) GetBotsCursor() *BotsCursor {
	if x != nil {
		if x, ok := x.Payload.(*Cursor_BotsCursor); ok {
			return x.BotsCursor
		}
	}
	return nil
}

func (x *Cursor) GetTasksCursor() *TasksCursor {
	if x != nil {
		if x, ok := x.Payload.(*Cursor_TasksCursor); ok {
			return x.TasksCursor
		}
	}
	return nil
}

type isCursor_Payload interface {
	isCursor_Payload()
}

type Cursor_OpaqueCursor struct {
	OpaqueCursor *OpaqueCursor `protobuf:"bytes,10,opt,name=opaque_cursor,json=opaqueCursor,proto3,oneof"`
}

type Cursor_BotsCursor struct {
	BotsCursor *BotsCursor `protobuf:"bytes,11,opt,name=bots_cursor,json=botsCursor,proto3,oneof"`
}

type Cursor_TasksCursor struct {
	TasksCursor *TasksCursor `protobuf:"bytes,12,opt,name=tasks_cursor,json=tasksCursor,proto3,oneof"`
}

func (*Cursor_OpaqueCursor) isCursor_Payload() {}

func (*Cursor_BotsCursor) isCursor_Payload() {}

func (*Cursor_TasksCursor) isCursor_Payload() {}

// An opaque datastore cursor (in its raw []byte form).
type OpaqueCursor struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Base64-decoded cursor returned by the Cloud Datastore API.
	Cursor        []byte `protobuf:"bytes,1,opt,name=cursor,proto3" json:"cursor,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *OpaqueCursor) Reset() {
	*x = OpaqueCursor{}
	mi := &file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *OpaqueCursor) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpaqueCursor) ProtoMessage() {}

func (x *OpaqueCursor) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpaqueCursor.ProtoReflect.Descriptor instead.
func (*OpaqueCursor) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_rawDescGZIP(), []int{1}
}

func (x *OpaqueCursor) GetCursor() []byte {
	if x != nil {
		return x.Cursor
	}
	return nil
}

// Cursor used in the bots listing query (there's only one).
//
// Bots are always ordered by bot ID. This cursor just contains the latest
// returned bot ID. To resume from it, we query for all bots with the ID larger
// than the one in the cursor.
type BotsCursor struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The last returned bot ID.
	LastBotId     string `protobuf:"bytes,1,opt,name=last_bot_id,json=lastBotId,proto3" json:"last_bot_id,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *BotsCursor) Reset() {
	*x = BotsCursor{}
	mi := &file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *BotsCursor) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BotsCursor) ProtoMessage() {}

func (x *BotsCursor) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BotsCursor.ProtoReflect.Descriptor instead.
func (*BotsCursor) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_rawDescGZIP(), []int{2}
}

func (x *BotsCursor) GetLastBotId() string {
	if x != nil {
		return x.LastBotId
	}
	return ""
}

// Cursor used in various task listing queries.
//
// Tasks are currently always ordered by their entity IDs (which encode their
// creation time). This cursor just contains the latest returned task ID (as
// the corresponding TaskRequest datastore ID). To resume from it, we query for
// all entities with the ID larger than the one in the cursor.
type TasksCursor struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The ID of TaskRequest entity of the latest returned task.
	LastTaskRequestEntityId int64 `protobuf:"varint,1,opt,name=last_task_request_entity_id,json=lastTaskRequestEntityId,proto3" json:"last_task_request_entity_id,omitempty"`
	unknownFields           protoimpl.UnknownFields
	sizeCache               protoimpl.SizeCache
}

func (x *TasksCursor) Reset() {
	*x = TasksCursor{}
	mi := &file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *TasksCursor) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*TasksCursor) ProtoMessage() {}

func (x *TasksCursor) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use TasksCursor.ProtoReflect.Descriptor instead.
func (*TasksCursor) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_rawDescGZIP(), []int{3}
}

func (x *TasksCursor) GetLastTaskRequestEntityId() int64 {
	if x != nil {
		return x.LastTaskRequestEntityId
	}
	return 0
}

var File_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_rawDesc = string([]byte{
	0x0a, 0x41, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x73, 0x77, 0x61, 0x72, 0x6d, 0x69, 0x6e, 0x67, 0x2f,
	0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2f, 0x63, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x2f, 0x63, 0x75,
	0x72, 0x73, 0x6f, 0x72, 0x70, 0x62, 0x2f, 0x63, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x12, 0x19, 0x73, 0x77, 0x61, 0x72, 0x6d, 0x69, 0x6e, 0x67, 0x2e, 0x69, 0x6e,
	0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x73, 0x2e, 0x63, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x1a, 0x1f,
	0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f,
	0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22,
	0xf2, 0x02, 0x0a, 0x06, 0x43, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x12, 0x40, 0x0a, 0x07, 0x72, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x26, 0x2e, 0x73, 0x77,
	0x61, 0x72, 0x6d, 0x69, 0x6e, 0x67, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x73,
	0x2e, 0x63, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x2e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x4b,
	0x69, 0x6e, 0x64, 0x52, 0x07, 0x72, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x34, 0x0a, 0x07,
	0x63, 0x72, 0x65, 0x61, 0x74, 0x65, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e,
	0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e,
	0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x52, 0x07, 0x63, 0x72, 0x65, 0x61, 0x74,
	0x65, 0x64, 0x12, 0x4e, 0x0a, 0x0d, 0x6f, 0x70, 0x61, 0x71, 0x75, 0x65, 0x5f, 0x63, 0x75, 0x72,
	0x73, 0x6f, 0x72, 0x18, 0x0a, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x27, 0x2e, 0x73, 0x77, 0x61, 0x72,
	0x6d, 0x69, 0x6e, 0x67, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x73, 0x2e, 0x63,
	0x75, 0x72, 0x73, 0x6f, 0x72, 0x2e, 0x4f, 0x70, 0x61, 0x71, 0x75, 0x65, 0x43, 0x75, 0x72, 0x73,
	0x6f, 0x72, 0x48, 0x00, 0x52, 0x0c, 0x6f, 0x70, 0x61, 0x71, 0x75, 0x65, 0x43, 0x75, 0x72, 0x73,
	0x6f, 0x72, 0x12, 0x48, 0x0a, 0x0b, 0x62, 0x6f, 0x74, 0x73, 0x5f, 0x63, 0x75, 0x72, 0x73, 0x6f,
	0x72, 0x18, 0x0b, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x25, 0x2e, 0x73, 0x77, 0x61, 0x72, 0x6d, 0x69,
	0x6e, 0x67, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x73, 0x2e, 0x63, 0x75, 0x72,
	0x73, 0x6f, 0x72, 0x2e, 0x42, 0x6f, 0x74, 0x73, 0x43, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x48, 0x00,
	0x52, 0x0a, 0x62, 0x6f, 0x74, 0x73, 0x43, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x12, 0x4b, 0x0a, 0x0c,
	0x74, 0x61, 0x73, 0x6b, 0x73, 0x5f, 0x63, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x18, 0x0c, 0x20, 0x01,
	0x28, 0x0b, 0x32, 0x26, 0x2e, 0x73, 0x77, 0x61, 0x72, 0x6d, 0x69, 0x6e, 0x67, 0x2e, 0x69, 0x6e,
	0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x73, 0x2e, 0x63, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x2e, 0x54,
	0x61, 0x73, 0x6b, 0x73, 0x43, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x48, 0x00, 0x52, 0x0b, 0x74, 0x61,
	0x73, 0x6b, 0x73, 0x43, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x42, 0x09, 0x0a, 0x07, 0x70, 0x61, 0x79,
	0x6c, 0x6f, 0x61, 0x64, 0x22, 0x26, 0x0a, 0x0c, 0x4f, 0x70, 0x61, 0x71, 0x75, 0x65, 0x43, 0x75,
	0x72, 0x73, 0x6f, 0x72, 0x12, 0x16, 0x0a, 0x06, 0x63, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x0c, 0x52, 0x06, 0x63, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x22, 0x2c, 0x0a, 0x0a,
	0x42, 0x6f, 0x74, 0x73, 0x43, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x12, 0x1e, 0x0a, 0x0b, 0x6c, 0x61,
	0x73, 0x74, 0x5f, 0x62, 0x6f, 0x74, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x09, 0x6c, 0x61, 0x73, 0x74, 0x42, 0x6f, 0x74, 0x49, 0x64, 0x22, 0x4b, 0x0a, 0x0b, 0x54, 0x61,
	0x73, 0x6b, 0x73, 0x43, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x12, 0x3c, 0x0a, 0x1b, 0x6c, 0x61, 0x73,
	0x74, 0x5f, 0x74, 0x61, 0x73, 0x6b, 0x5f, 0x72, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x5f, 0x65,
	0x6e, 0x74, 0x69, 0x74, 0x79, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x03, 0x52, 0x17,
	0x6c, 0x61, 0x73, 0x74, 0x54, 0x61, 0x73, 0x6b, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x45,
	0x6e, 0x74, 0x69, 0x74, 0x79, 0x49, 0x64, 0x2a, 0x9d, 0x01, 0x0a, 0x0b, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x4b, 0x69, 0x6e, 0x64, 0x12, 0x1c, 0x0a, 0x18, 0x52, 0x45, 0x51, 0x55, 0x45,
	0x53, 0x54, 0x5f, 0x4b, 0x49, 0x4e, 0x44, 0x5f, 0x55, 0x4e, 0x53, 0x50, 0x45, 0x43, 0x49, 0x46,
	0x49, 0x45, 0x44, 0x10, 0x00, 0x12, 0x13, 0x0a, 0x0f, 0x4c, 0x49, 0x53, 0x54, 0x5f, 0x42, 0x4f,
	0x54, 0x5f, 0x45, 0x56, 0x45, 0x4e, 0x54, 0x53, 0x10, 0x01, 0x12, 0x12, 0x0a, 0x0e, 0x4c, 0x49,
	0x53, 0x54, 0x5f, 0x42, 0x4f, 0x54, 0x5f, 0x54, 0x41, 0x53, 0x4b, 0x53, 0x10, 0x02, 0x12, 0x0d,
	0x0a, 0x09, 0x4c, 0x49, 0x53, 0x54, 0x5f, 0x42, 0x4f, 0x54, 0x53, 0x10, 0x03, 0x12, 0x0e, 0x0a,
	0x0a, 0x4c, 0x49, 0x53, 0x54, 0x5f, 0x54, 0x41, 0x53, 0x4b, 0x53, 0x10, 0x04, 0x12, 0x16, 0x0a,
	0x12, 0x4c, 0x49, 0x53, 0x54, 0x5f, 0x54, 0x41, 0x53, 0x4b, 0x5f, 0x52, 0x45, 0x51, 0x55, 0x45,
	0x53, 0x54, 0x53, 0x10, 0x05, 0x12, 0x10, 0x0a, 0x0c, 0x43, 0x41, 0x4e, 0x43, 0x45, 0x4c, 0x5f,
	0x54, 0x41, 0x53, 0x4b, 0x53, 0x10, 0x06, 0x42, 0x36, 0x5a, 0x34, 0x67, 0x6f, 0x2e, 0x63, 0x68,
	0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f,
	0x73, 0x77, 0x61, 0x72, 0x6d, 0x69, 0x6e, 0x67, 0x2f, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2f,
	0x63, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x2f, 0x63, 0x75, 0x72, 0x73, 0x6f, 0x72, 0x70, 0x62, 0x62,
	0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_rawDescData []byte
)

func file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_rawDesc), len(file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_rawDescData
}

var file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_goTypes = []any{
	(RequestKind)(0),              // 0: swarming.internals.cursor.RequestKind
	(*Cursor)(nil),                // 1: swarming.internals.cursor.Cursor
	(*OpaqueCursor)(nil),          // 2: swarming.internals.cursor.OpaqueCursor
	(*BotsCursor)(nil),            // 3: swarming.internals.cursor.BotsCursor
	(*TasksCursor)(nil),           // 4: swarming.internals.cursor.TasksCursor
	(*timestamppb.Timestamp)(nil), // 5: google.protobuf.Timestamp
}
var file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_depIdxs = []int32{
	0, // 0: swarming.internals.cursor.Cursor.request:type_name -> swarming.internals.cursor.RequestKind
	5, // 1: swarming.internals.cursor.Cursor.created:type_name -> google.protobuf.Timestamp
	2, // 2: swarming.internals.cursor.Cursor.opaque_cursor:type_name -> swarming.internals.cursor.OpaqueCursor
	3, // 3: swarming.internals.cursor.Cursor.bots_cursor:type_name -> swarming.internals.cursor.BotsCursor
	4, // 4: swarming.internals.cursor.Cursor.tasks_cursor:type_name -> swarming.internals.cursor.TasksCursor
	5, // [5:5] is the sub-list for method output_type
	5, // [5:5] is the sub-list for method input_type
	5, // [5:5] is the sub-list for extension type_name
	5, // [5:5] is the sub-list for extension extendee
	0, // [0:5] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_init() }
func file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_init() {
	if File_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto != nil {
		return
	}
	file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_msgTypes[0].OneofWrappers = []any{
		(*Cursor_OpaqueCursor)(nil),
		(*Cursor_BotsCursor)(nil),
		(*Cursor_TasksCursor)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_rawDesc), len(file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_rawDesc)),
			NumEnums:      1,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_depIdxs,
		EnumInfos:         file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_enumTypes,
		MessageInfos:      file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto = out.File
	file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_goTypes = nil
	file_go_chromium_org_luci_swarming_server_cursor_cursorpb_cursor_proto_depIdxs = nil
}
