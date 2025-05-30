// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.5
// 	protoc        v6.30.2
// source: go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1/state.proto

package logdog

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

// LogStreamState is a bidirectional state value used in UpdateStream calls.
//
// LogStreamState is embeddable in Endpoints request/response structs.
type LogStreamState struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// ProtoVersion is the protobuf version for this stream.
	ProtoVersion string `protobuf:"bytes,1,opt,name=proto_version,json=protoVersion,proto3" json:"proto_version,omitempty"`
	// The time when the log stream was registered with the Coordinator.
	Created *timestamppb.Timestamp `protobuf:"bytes,2,opt,name=created,proto3" json:"created,omitempty"`
	// The stream index of the log stream's terminal message. If the value is -1,
	// the log is still streaming.
	TerminalIndex int64 `protobuf:"varint,3,opt,name=terminal_index,json=terminalIndex,proto3" json:"terminal_index,omitempty"`
	// If non-nil, the log stream is archived, and this field contains archival
	// details.
	Archive *LogStreamState_ArchiveInfo `protobuf:"bytes,4,opt,name=archive,proto3" json:"archive,omitempty"`
	// Indicates the purged state of a log. A log that has been purged is only
	// acknowledged to administrative clients.
	Purged        bool `protobuf:"varint,5,opt,name=purged,proto3" json:"purged,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *LogStreamState) Reset() {
	*x = LogStreamState{}
	mi := &file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *LogStreamState) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*LogStreamState) ProtoMessage() {}

func (x *LogStreamState) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use LogStreamState.ProtoReflect.Descriptor instead.
func (*LogStreamState) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_rawDescGZIP(), []int{0}
}

func (x *LogStreamState) GetProtoVersion() string {
	if x != nil {
		return x.ProtoVersion
	}
	return ""
}

func (x *LogStreamState) GetCreated() *timestamppb.Timestamp {
	if x != nil {
		return x.Created
	}
	return nil
}

func (x *LogStreamState) GetTerminalIndex() int64 {
	if x != nil {
		return x.TerminalIndex
	}
	return 0
}

func (x *LogStreamState) GetArchive() *LogStreamState_ArchiveInfo {
	if x != nil {
		return x.Archive
	}
	return nil
}

func (x *LogStreamState) GetPurged() bool {
	if x != nil {
		return x.Purged
	}
	return false
}

// ArchiveInfo contains archive details for the log stream.
type LogStreamState_ArchiveInfo struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The Google Storage URL where the log stream's index is archived.
	IndexUrl string `protobuf:"bytes,1,opt,name=index_url,json=indexUrl,proto3" json:"index_url,omitempty"`
	// The Google Storage URL where the log stream's raw stream data is archived.
	StreamUrl string `protobuf:"bytes,2,opt,name=stream_url,json=streamUrl,proto3" json:"stream_url,omitempty"`
	// The Google Storage URL where the log stream's assembled data is archived.
	DataUrl string `protobuf:"bytes,3,opt,name=data_url,json=dataUrl,proto3" json:"data_url,omitempty"`
	// If true, all log entries between 0 and terminal_index were archived. If
	// false, this indicates that the log stream was not completely loaded into
	// intermediate storage when the archival interval expired.
	Complete bool `protobuf:"varint,4,opt,name=complete,proto3" json:"complete,omitempty"`
	// The number of log
	LogEntryCount int64 `protobuf:"varint,5,opt,name=log_entry_count,json=logEntryCount,proto3" json:"log_entry_count,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *LogStreamState_ArchiveInfo) Reset() {
	*x = LogStreamState_ArchiveInfo{}
	mi := &file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *LogStreamState_ArchiveInfo) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*LogStreamState_ArchiveInfo) ProtoMessage() {}

func (x *LogStreamState_ArchiveInfo) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use LogStreamState_ArchiveInfo.ProtoReflect.Descriptor instead.
func (*LogStreamState_ArchiveInfo) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_rawDescGZIP(), []int{0, 0}
}

func (x *LogStreamState_ArchiveInfo) GetIndexUrl() string {
	if x != nil {
		return x.IndexUrl
	}
	return ""
}

func (x *LogStreamState_ArchiveInfo) GetStreamUrl() string {
	if x != nil {
		return x.StreamUrl
	}
	return ""
}

func (x *LogStreamState_ArchiveInfo) GetDataUrl() string {
	if x != nil {
		return x.DataUrl
	}
	return ""
}

func (x *LogStreamState_ArchiveInfo) GetComplete() bool {
	if x != nil {
		return x.Complete
	}
	return false
}

func (x *LogStreamState_ArchiveInfo) GetLogEntryCount() int64 {
	if x != nil {
		return x.LogEntryCount
	}
	return 0
}

var File_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_rawDesc = string([]byte{
	0x0a, 0x49, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x6c, 0x6f, 0x67, 0x64, 0x6f, 0x67, 0x2f, 0x61, 0x70,
	0x69, 0x2f, 0x65, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x73, 0x2f, 0x63, 0x6f, 0x6f, 0x72,
	0x64, 0x69, 0x6e, 0x61, 0x74, 0x6f, 0x72, 0x2f, 0x6c, 0x6f, 0x67, 0x73, 0x2f, 0x76, 0x31, 0x2f,
	0x73, 0x74, 0x61, 0x74, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x06, 0x6c, 0x6f, 0x67,
	0x64, 0x6f, 0x67, 0x1a, 0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x62, 0x75, 0x66, 0x2f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x22, 0x93, 0x03, 0x0a, 0x0e, 0x4c, 0x6f, 0x67, 0x53, 0x74, 0x72, 0x65,
	0x61, 0x6d, 0x53, 0x74, 0x61, 0x74, 0x65, 0x12, 0x23, 0x0a, 0x0d, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x5f, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0c,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x56, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x12, 0x34, 0x0a, 0x07,
	0x63, 0x72, 0x65, 0x61, 0x74, 0x65, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e,
	0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e,
	0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x52, 0x07, 0x63, 0x72, 0x65, 0x61, 0x74,
	0x65, 0x64, 0x12, 0x25, 0x0a, 0x0e, 0x74, 0x65, 0x72, 0x6d, 0x69, 0x6e, 0x61, 0x6c, 0x5f, 0x69,
	0x6e, 0x64, 0x65, 0x78, 0x18, 0x03, 0x20, 0x01, 0x28, 0x03, 0x52, 0x0d, 0x74, 0x65, 0x72, 0x6d,
	0x69, 0x6e, 0x61, 0x6c, 0x49, 0x6e, 0x64, 0x65, 0x78, 0x12, 0x3c, 0x0a, 0x07, 0x61, 0x72, 0x63,
	0x68, 0x69, 0x76, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x22, 0x2e, 0x6c, 0x6f, 0x67,
	0x64, 0x6f, 0x67, 0x2e, 0x4c, 0x6f, 0x67, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x53, 0x74, 0x61,
	0x74, 0x65, 0x2e, 0x41, 0x72, 0x63, 0x68, 0x69, 0x76, 0x65, 0x49, 0x6e, 0x66, 0x6f, 0x52, 0x07,
	0x61, 0x72, 0x63, 0x68, 0x69, 0x76, 0x65, 0x12, 0x16, 0x0a, 0x06, 0x70, 0x75, 0x72, 0x67, 0x65,
	0x64, 0x18, 0x05, 0x20, 0x01, 0x28, 0x08, 0x52, 0x06, 0x70, 0x75, 0x72, 0x67, 0x65, 0x64, 0x1a,
	0xa8, 0x01, 0x0a, 0x0b, 0x41, 0x72, 0x63, 0x68, 0x69, 0x76, 0x65, 0x49, 0x6e, 0x66, 0x6f, 0x12,
	0x1b, 0x0a, 0x09, 0x69, 0x6e, 0x64, 0x65, 0x78, 0x5f, 0x75, 0x72, 0x6c, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x08, 0x69, 0x6e, 0x64, 0x65, 0x78, 0x55, 0x72, 0x6c, 0x12, 0x1d, 0x0a, 0x0a,
	0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x5f, 0x75, 0x72, 0x6c, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x09, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x55, 0x72, 0x6c, 0x12, 0x19, 0x0a, 0x08, 0x64,
	0x61, 0x74, 0x61, 0x5f, 0x75, 0x72, 0x6c, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x64,
	0x61, 0x74, 0x61, 0x55, 0x72, 0x6c, 0x12, 0x1a, 0x0a, 0x08, 0x63, 0x6f, 0x6d, 0x70, 0x6c, 0x65,
	0x74, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x08, 0x52, 0x08, 0x63, 0x6f, 0x6d, 0x70, 0x6c, 0x65,
	0x74, 0x65, 0x12, 0x26, 0x0a, 0x0f, 0x6c, 0x6f, 0x67, 0x5f, 0x65, 0x6e, 0x74, 0x72, 0x79, 0x5f,
	0x63, 0x6f, 0x75, 0x6e, 0x74, 0x18, 0x05, 0x20, 0x01, 0x28, 0x03, 0x52, 0x0d, 0x6c, 0x6f, 0x67,
	0x45, 0x6e, 0x74, 0x72, 0x79, 0x43, 0x6f, 0x75, 0x6e, 0x74, 0x42, 0x46, 0x5a, 0x44, 0x67, 0x6f,
	0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75,
	0x63, 0x69, 0x2f, 0x6c, 0x6f, 0x67, 0x64, 0x6f, 0x67, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x65, 0x6e,
	0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x73, 0x2f, 0x63, 0x6f, 0x6f, 0x72, 0x64, 0x69, 0x6e, 0x61,
	0x74, 0x6f, 0x72, 0x2f, 0x6c, 0x6f, 0x67, 0x73, 0x2f, 0x76, 0x31, 0x3b, 0x6c, 0x6f, 0x67, 0x64,
	0x6f, 0x67, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_rawDescData []byte
)

func file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_rawDesc), len(file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_rawDescData
}

var file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_goTypes = []any{
	(*LogStreamState)(nil),             // 0: logdog.LogStreamState
	(*LogStreamState_ArchiveInfo)(nil), // 1: logdog.LogStreamState.ArchiveInfo
	(*timestamppb.Timestamp)(nil),      // 2: google.protobuf.Timestamp
}
var file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_depIdxs = []int32{
	2, // 0: logdog.LogStreamState.created:type_name -> google.protobuf.Timestamp
	1, // 1: logdog.LogStreamState.archive:type_name -> logdog.LogStreamState.ArchiveInfo
	2, // [2:2] is the sub-list for method output_type
	2, // [2:2] is the sub-list for method input_type
	2, // [2:2] is the sub-list for extension type_name
	2, // [2:2] is the sub-list for extension extendee
	0, // [0:2] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_init() }
func file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_init() {
	if File_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_rawDesc), len(file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto = out.File
	file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_goTypes = nil
	file_go_chromium_org_luci_logdog_api_endpoints_coordinator_logs_v1_state_proto_depIdxs = nil
}
