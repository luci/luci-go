// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.5
// 	protoc        v6.30.2
// source: go.chromium.org/luci/logdog/api/logpb/butler.proto

package logpb

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

// This enumerates the possible contents of published Butler data.
type ButlerMetadata_ContentType int32

const (
	// An invalid content type. Do not use.
	ButlerMetadata_Invalid ButlerMetadata_ContentType = 0
	// The published data is a ButlerLogBundle protobuf message.
	ButlerMetadata_ButlerLogBundle ButlerMetadata_ContentType = 1
)

// Enum value maps for ButlerMetadata_ContentType.
var (
	ButlerMetadata_ContentType_name = map[int32]string{
		0: "Invalid",
		1: "ButlerLogBundle",
	}
	ButlerMetadata_ContentType_value = map[string]int32{
		"Invalid":         0,
		"ButlerLogBundle": 1,
	}
)

func (x ButlerMetadata_ContentType) Enum() *ButlerMetadata_ContentType {
	p := new(ButlerMetadata_ContentType)
	*p = x
	return p
}

func (x ButlerMetadata_ContentType) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (ButlerMetadata_ContentType) Descriptor() protoreflect.EnumDescriptor {
	return file_go_chromium_org_luci_logdog_api_logpb_butler_proto_enumTypes[0].Descriptor()
}

func (ButlerMetadata_ContentType) Type() protoreflect.EnumType {
	return &file_go_chromium_org_luci_logdog_api_logpb_butler_proto_enumTypes[0]
}

func (x ButlerMetadata_ContentType) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use ButlerMetadata_ContentType.Descriptor instead.
func (ButlerMetadata_ContentType) EnumDescriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_logdog_api_logpb_butler_proto_rawDescGZIP(), []int{0, 0}
}

// Compression scheme of attached data.
type ButlerMetadata_Compression int32

const (
	ButlerMetadata_NONE ButlerMetadata_Compression = 0
	ButlerMetadata_ZLIB ButlerMetadata_Compression = 1
)

// Enum value maps for ButlerMetadata_Compression.
var (
	ButlerMetadata_Compression_name = map[int32]string{
		0: "NONE",
		1: "ZLIB",
	}
	ButlerMetadata_Compression_value = map[string]int32{
		"NONE": 0,
		"ZLIB": 1,
	}
)

func (x ButlerMetadata_Compression) Enum() *ButlerMetadata_Compression {
	p := new(ButlerMetadata_Compression)
	*p = x
	return p
}

func (x ButlerMetadata_Compression) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (ButlerMetadata_Compression) Descriptor() protoreflect.EnumDescriptor {
	return file_go_chromium_org_luci_logdog_api_logpb_butler_proto_enumTypes[1].Descriptor()
}

func (ButlerMetadata_Compression) Type() protoreflect.EnumType {
	return &file_go_chromium_org_luci_logdog_api_logpb_butler_proto_enumTypes[1]
}

func (x ButlerMetadata_Compression) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use ButlerMetadata_Compression.Descriptor instead.
func (ButlerMetadata_Compression) EnumDescriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_logdog_api_logpb_butler_proto_rawDescGZIP(), []int{0, 1}
}

// ButlerMetadata appears as a frame at the beginning of Butler published data
// to describe the remainder of the contents.
type ButlerMetadata struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// This is the type of data in the subsequent frame.
	Type        ButlerMetadata_ContentType `protobuf:"varint,1,opt,name=type,proto3,enum=logpb.ButlerMetadata_ContentType" json:"type,omitempty"`
	Compression ButlerMetadata_Compression `protobuf:"varint,2,opt,name=compression,proto3,enum=logpb.ButlerMetadata_Compression" json:"compression,omitempty"`
	// The protobuf version string (see version.go).
	ProtoVersion  string `protobuf:"bytes,3,opt,name=proto_version,json=protoVersion,proto3" json:"proto_version,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ButlerMetadata) Reset() {
	*x = ButlerMetadata{}
	mi := &file_go_chromium_org_luci_logdog_api_logpb_butler_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ButlerMetadata) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ButlerMetadata) ProtoMessage() {}

func (x *ButlerMetadata) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_logdog_api_logpb_butler_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ButlerMetadata.ProtoReflect.Descriptor instead.
func (*ButlerMetadata) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_logdog_api_logpb_butler_proto_rawDescGZIP(), []int{0}
}

func (x *ButlerMetadata) GetType() ButlerMetadata_ContentType {
	if x != nil {
		return x.Type
	}
	return ButlerMetadata_Invalid
}

func (x *ButlerMetadata) GetCompression() ButlerMetadata_Compression {
	if x != nil {
		return x.Compression
	}
	return ButlerMetadata_NONE
}

func (x *ButlerMetadata) GetProtoVersion() string {
	if x != nil {
		return x.ProtoVersion
	}
	return ""
}

// A message containing log data in transit from the Butler.
//
// The Butler is capable of conserving bandwidth by bundling collected log
// messages together into this protocol buffer. Based on Butler bundling
// settings, this message can represent anything from a single LogRecord to
// multiple LogRecords belonging to several different streams.
//
// Entries in a Log Bundle are fully self-descriptive: no additional information
// is needed to fully associate the contained data with its proper place in
// the source log stream.
type ButlerLogBundle struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// (DEPRECATED) Stream source information. Now supplied during prefix
	// registration.
	DeprecatedSource string `protobuf:"bytes,1,opt,name=deprecated_source,json=deprecatedSource,proto3" json:"deprecated_source,omitempty"`
	// The timestamp when this bundle was generated.
	//
	// This field will be used for debugging and internal accounting.
	Timestamp *timestamppb.Timestamp `protobuf:"bytes,2,opt,name=timestamp,proto3" json:"timestamp,omitempty"`
	// *
	// Each Entry is an individual set of log records for a given log stream.
	Entries []*ButlerLogBundle_Entry `protobuf:"bytes,3,rep,name=entries,proto3" json:"entries,omitempty"`
	// * Project specifies which luci-config project this stream belongs to.
	Project string `protobuf:"bytes,4,opt,name=project,proto3" json:"project,omitempty"`
	// *
	// The log stream prefix that is shared by all bundled streams.
	//
	// This prefix is valid within the supplied project scope.
	Prefix string `protobuf:"bytes,5,opt,name=prefix,proto3" json:"prefix,omitempty"`
	// The log prefix's secret value (required).
	//
	// The secret is bound to all log streams that share the supplied Prefix, and
	// The Coordinator will record the secret associated with a given log Prefix,
	// but will not expose the secret to users.
	//
	// The Collector will check the secret prior to ingesting logs. If the
	// secret doesn't match the value recorded by the Coordinator, the log
	// will be discarded.
	//
	// This ensures that only the Butler instance that generated the log stream
	// can emit log data for that stream. It also ensures that only authenticated
	// users can write to a Prefix.
	Secret        []byte `protobuf:"bytes,6,opt,name=secret,proto3" json:"secret,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ButlerLogBundle) Reset() {
	*x = ButlerLogBundle{}
	mi := &file_go_chromium_org_luci_logdog_api_logpb_butler_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ButlerLogBundle) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ButlerLogBundle) ProtoMessage() {}

func (x *ButlerLogBundle) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_logdog_api_logpb_butler_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ButlerLogBundle.ProtoReflect.Descriptor instead.
func (*ButlerLogBundle) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_logdog_api_logpb_butler_proto_rawDescGZIP(), []int{1}
}

func (x *ButlerLogBundle) GetDeprecatedSource() string {
	if x != nil {
		return x.DeprecatedSource
	}
	return ""
}

func (x *ButlerLogBundle) GetTimestamp() *timestamppb.Timestamp {
	if x != nil {
		return x.Timestamp
	}
	return nil
}

func (x *ButlerLogBundle) GetEntries() []*ButlerLogBundle_Entry {
	if x != nil {
		return x.Entries
	}
	return nil
}

func (x *ButlerLogBundle) GetProject() string {
	if x != nil {
		return x.Project
	}
	return ""
}

func (x *ButlerLogBundle) GetPrefix() string {
	if x != nil {
		return x.Prefix
	}
	return ""
}

func (x *ButlerLogBundle) GetSecret() []byte {
	if x != nil {
		return x.Secret
	}
	return nil
}

// A bundle Entry describes a set of LogEntry messages originating from the
// same log stream.
type ButlerLogBundle_Entry struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The descriptor for this entry's log stream.
	//
	// Each LogEntry in the "logs" field is shares this common descriptor.
	Desc *LogStreamDescriptor `protobuf:"bytes,1,opt,name=desc,proto3" json:"desc,omitempty"`
	// (DEPRECATED) Per-entry secret replaced with Butler-wide secret.
	DeprecatedEntrySecret []byte `protobuf:"bytes,2,opt,name=deprecated_entry_secret,json=deprecatedEntrySecret,proto3" json:"deprecated_entry_secret,omitempty"`
	// Whether this log entry terminates its stream.
	//
	// If present and "true", this field declares that this Entry is the last
	// such entry in the stream. This fact is recorded by the Collector and
	// registered with the Coordinator. The largest stream prefix in this Entry
	// will be bound the stream's LogEntry records to [0:largest_prefix]. Once
	// all messages in that range have been received, the log may be archived.
	//
	// Further log entries belonging to this stream with stream indices
	// exceeding the terminal log's index will be discarded.
	Terminal bool `protobuf:"varint,3,opt,name=terminal,proto3" json:"terminal,omitempty"`
	// If terminal is true, this is the terminal stream index; that is, the last
	// message index in the stream.
	TerminalIndex uint64 `protobuf:"varint,4,opt,name=terminal_index,json=terminalIndex,proto3" json:"terminal_index,omitempty"`
	// Log entries attached to this record. These MUST be sequential.
	//
	// This is the main log entry content.
	Logs          []*LogEntry `protobuf:"bytes,5,rep,name=logs,proto3" json:"logs,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ButlerLogBundle_Entry) Reset() {
	*x = ButlerLogBundle_Entry{}
	mi := &file_go_chromium_org_luci_logdog_api_logpb_butler_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ButlerLogBundle_Entry) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ButlerLogBundle_Entry) ProtoMessage() {}

func (x *ButlerLogBundle_Entry) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_logdog_api_logpb_butler_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ButlerLogBundle_Entry.ProtoReflect.Descriptor instead.
func (*ButlerLogBundle_Entry) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_logdog_api_logpb_butler_proto_rawDescGZIP(), []int{1, 0}
}

func (x *ButlerLogBundle_Entry) GetDesc() *LogStreamDescriptor {
	if x != nil {
		return x.Desc
	}
	return nil
}

func (x *ButlerLogBundle_Entry) GetDeprecatedEntrySecret() []byte {
	if x != nil {
		return x.DeprecatedEntrySecret
	}
	return nil
}

func (x *ButlerLogBundle_Entry) GetTerminal() bool {
	if x != nil {
		return x.Terminal
	}
	return false
}

func (x *ButlerLogBundle_Entry) GetTerminalIndex() uint64 {
	if x != nil {
		return x.TerminalIndex
	}
	return 0
}

func (x *ButlerLogBundle_Entry) GetLogs() []*LogEntry {
	if x != nil {
		return x.Logs
	}
	return nil
}

var File_go_chromium_org_luci_logdog_api_logpb_butler_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_logdog_api_logpb_butler_proto_rawDesc = string([]byte{
	0x0a, 0x32, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x6c, 0x6f, 0x67, 0x64, 0x6f, 0x67, 0x2f, 0x61, 0x70,
	0x69, 0x2f, 0x6c, 0x6f, 0x67, 0x70, 0x62, 0x2f, 0x62, 0x75, 0x74, 0x6c, 0x65, 0x72, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x12, 0x05, 0x6c, 0x6f, 0x67, 0x70, 0x62, 0x1a, 0x2f, 0x67, 0x6f, 0x2e,
	0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63,
	0x69, 0x2f, 0x6c, 0x6f, 0x67, 0x64, 0x6f, 0x67, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x6c, 0x6f, 0x67,
	0x70, 0x62, 0x2f, 0x6c, 0x6f, 0x67, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1f, 0x67, 0x6f,
	0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x74, 0x69,
	0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x85, 0x02,
	0x0a, 0x0e, 0x42, 0x75, 0x74, 0x6c, 0x65, 0x72, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61,
	0x12, 0x35, 0x0a, 0x04, 0x74, 0x79, 0x70, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x21,
	0x2e, 0x6c, 0x6f, 0x67, 0x70, 0x62, 0x2e, 0x42, 0x75, 0x74, 0x6c, 0x65, 0x72, 0x4d, 0x65, 0x74,
	0x61, 0x64, 0x61, 0x74, 0x61, 0x2e, 0x43, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x54, 0x79, 0x70,
	0x65, 0x52, 0x04, 0x74, 0x79, 0x70, 0x65, 0x12, 0x43, 0x0a, 0x0b, 0x63, 0x6f, 0x6d, 0x70, 0x72,
	0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x21, 0x2e, 0x6c,
	0x6f, 0x67, 0x70, 0x62, 0x2e, 0x42, 0x75, 0x74, 0x6c, 0x65, 0x72, 0x4d, 0x65, 0x74, 0x61, 0x64,
	0x61, 0x74, 0x61, 0x2e, 0x43, 0x6f, 0x6d, 0x70, 0x72, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x52,
	0x0b, 0x63, 0x6f, 0x6d, 0x70, 0x72, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x12, 0x23, 0x0a, 0x0d,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x5f, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x03, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x0c, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x56, 0x65, 0x72, 0x73, 0x69, 0x6f,
	0x6e, 0x22, 0x2f, 0x0a, 0x0b, 0x43, 0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x54, 0x79, 0x70, 0x65,
	0x12, 0x0b, 0x0a, 0x07, 0x49, 0x6e, 0x76, 0x61, 0x6c, 0x69, 0x64, 0x10, 0x00, 0x12, 0x13, 0x0a,
	0x0f, 0x42, 0x75, 0x74, 0x6c, 0x65, 0x72, 0x4c, 0x6f, 0x67, 0x42, 0x75, 0x6e, 0x64, 0x6c, 0x65,
	0x10, 0x01, 0x22, 0x21, 0x0a, 0x0b, 0x43, 0x6f, 0x6d, 0x70, 0x72, 0x65, 0x73, 0x73, 0x69, 0x6f,
	0x6e, 0x12, 0x08, 0x0a, 0x04, 0x4e, 0x4f, 0x4e, 0x45, 0x10, 0x00, 0x12, 0x08, 0x0a, 0x04, 0x5a,
	0x4c, 0x49, 0x42, 0x10, 0x01, 0x22, 0xd4, 0x03, 0x0a, 0x0f, 0x42, 0x75, 0x74, 0x6c, 0x65, 0x72,
	0x4c, 0x6f, 0x67, 0x42, 0x75, 0x6e, 0x64, 0x6c, 0x65, 0x12, 0x2b, 0x0a, 0x11, 0x64, 0x65, 0x70,
	0x72, 0x65, 0x63, 0x61, 0x74, 0x65, 0x64, 0x5f, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x10, 0x64, 0x65, 0x70, 0x72, 0x65, 0x63, 0x61, 0x74, 0x65, 0x64,
	0x53, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x12, 0x38, 0x0a, 0x09, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74,
	0x61, 0x6d, 0x70, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65,
	0x73, 0x74, 0x61, 0x6d, 0x70, 0x52, 0x09, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70,
	0x12, 0x36, 0x0a, 0x07, 0x65, 0x6e, 0x74, 0x72, 0x69, 0x65, 0x73, 0x18, 0x03, 0x20, 0x03, 0x28,
	0x0b, 0x32, 0x1c, 0x2e, 0x6c, 0x6f, 0x67, 0x70, 0x62, 0x2e, 0x42, 0x75, 0x74, 0x6c, 0x65, 0x72,
	0x4c, 0x6f, 0x67, 0x42, 0x75, 0x6e, 0x64, 0x6c, 0x65, 0x2e, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52,
	0x07, 0x65, 0x6e, 0x74, 0x72, 0x69, 0x65, 0x73, 0x12, 0x18, 0x0a, 0x07, 0x70, 0x72, 0x6f, 0x6a,
	0x65, 0x63, 0x74, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x70, 0x72, 0x6f, 0x6a, 0x65,
	0x63, 0x74, 0x12, 0x16, 0x0a, 0x06, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x18, 0x05, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x06, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x12, 0x16, 0x0a, 0x06, 0x73, 0x65,
	0x63, 0x72, 0x65, 0x74, 0x18, 0x06, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x06, 0x73, 0x65, 0x63, 0x72,
	0x65, 0x74, 0x1a, 0xd7, 0x01, 0x0a, 0x05, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x2e, 0x0a, 0x04,
	0x64, 0x65, 0x73, 0x63, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x6c, 0x6f, 0x67,
	0x70, 0x62, 0x2e, 0x4c, 0x6f, 0x67, 0x53, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x44, 0x65, 0x73, 0x63,
	0x72, 0x69, 0x70, 0x74, 0x6f, 0x72, 0x52, 0x04, 0x64, 0x65, 0x73, 0x63, 0x12, 0x36, 0x0a, 0x17,
	0x64, 0x65, 0x70, 0x72, 0x65, 0x63, 0x61, 0x74, 0x65, 0x64, 0x5f, 0x65, 0x6e, 0x74, 0x72, 0x79,
	0x5f, 0x73, 0x65, 0x63, 0x72, 0x65, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x15, 0x64,
	0x65, 0x70, 0x72, 0x65, 0x63, 0x61, 0x74, 0x65, 0x64, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x53, 0x65,
	0x63, 0x72, 0x65, 0x74, 0x12, 0x1a, 0x0a, 0x08, 0x74, 0x65, 0x72, 0x6d, 0x69, 0x6e, 0x61, 0x6c,
	0x18, 0x03, 0x20, 0x01, 0x28, 0x08, 0x52, 0x08, 0x74, 0x65, 0x72, 0x6d, 0x69, 0x6e, 0x61, 0x6c,
	0x12, 0x25, 0x0a, 0x0e, 0x74, 0x65, 0x72, 0x6d, 0x69, 0x6e, 0x61, 0x6c, 0x5f, 0x69, 0x6e, 0x64,
	0x65, 0x78, 0x18, 0x04, 0x20, 0x01, 0x28, 0x04, 0x52, 0x0d, 0x74, 0x65, 0x72, 0x6d, 0x69, 0x6e,
	0x61, 0x6c, 0x49, 0x6e, 0x64, 0x65, 0x78, 0x12, 0x23, 0x0a, 0x04, 0x6c, 0x6f, 0x67, 0x73, 0x18,
	0x05, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x0f, 0x2e, 0x6c, 0x6f, 0x67, 0x70, 0x62, 0x2e, 0x4c, 0x6f,
	0x67, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x04, 0x6c, 0x6f, 0x67, 0x73, 0x42, 0x27, 0x5a, 0x25,
	0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f,
	0x6c, 0x75, 0x63, 0x69, 0x2f, 0x6c, 0x6f, 0x67, 0x64, 0x6f, 0x67, 0x2f, 0x61, 0x70, 0x69, 0x2f,
	0x6c, 0x6f, 0x67, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_logdog_api_logpb_butler_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_logdog_api_logpb_butler_proto_rawDescData []byte
)

func file_go_chromium_org_luci_logdog_api_logpb_butler_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_logdog_api_logpb_butler_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_logdog_api_logpb_butler_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_logdog_api_logpb_butler_proto_rawDesc), len(file_go_chromium_org_luci_logdog_api_logpb_butler_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_logdog_api_logpb_butler_proto_rawDescData
}

var file_go_chromium_org_luci_logdog_api_logpb_butler_proto_enumTypes = make([]protoimpl.EnumInfo, 2)
var file_go_chromium_org_luci_logdog_api_logpb_butler_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_go_chromium_org_luci_logdog_api_logpb_butler_proto_goTypes = []any{
	(ButlerMetadata_ContentType)(0), // 0: logpb.ButlerMetadata.ContentType
	(ButlerMetadata_Compression)(0), // 1: logpb.ButlerMetadata.Compression
	(*ButlerMetadata)(nil),          // 2: logpb.ButlerMetadata
	(*ButlerLogBundle)(nil),         // 3: logpb.ButlerLogBundle
	(*ButlerLogBundle_Entry)(nil),   // 4: logpb.ButlerLogBundle.Entry
	(*timestamppb.Timestamp)(nil),   // 5: google.protobuf.Timestamp
	(*LogStreamDescriptor)(nil),     // 6: logpb.LogStreamDescriptor
	(*LogEntry)(nil),                // 7: logpb.LogEntry
}
var file_go_chromium_org_luci_logdog_api_logpb_butler_proto_depIdxs = []int32{
	0, // 0: logpb.ButlerMetadata.type:type_name -> logpb.ButlerMetadata.ContentType
	1, // 1: logpb.ButlerMetadata.compression:type_name -> logpb.ButlerMetadata.Compression
	5, // 2: logpb.ButlerLogBundle.timestamp:type_name -> google.protobuf.Timestamp
	4, // 3: logpb.ButlerLogBundle.entries:type_name -> logpb.ButlerLogBundle.Entry
	6, // 4: logpb.ButlerLogBundle.Entry.desc:type_name -> logpb.LogStreamDescriptor
	7, // 5: logpb.ButlerLogBundle.Entry.logs:type_name -> logpb.LogEntry
	6, // [6:6] is the sub-list for method output_type
	6, // [6:6] is the sub-list for method input_type
	6, // [6:6] is the sub-list for extension type_name
	6, // [6:6] is the sub-list for extension extendee
	0, // [0:6] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_logdog_api_logpb_butler_proto_init() }
func file_go_chromium_org_luci_logdog_api_logpb_butler_proto_init() {
	if File_go_chromium_org_luci_logdog_api_logpb_butler_proto != nil {
		return
	}
	file_go_chromium_org_luci_logdog_api_logpb_log_proto_init()
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_logdog_api_logpb_butler_proto_rawDesc), len(file_go_chromium_org_luci_logdog_api_logpb_butler_proto_rawDesc)),
			NumEnums:      2,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_logdog_api_logpb_butler_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_logdog_api_logpb_butler_proto_depIdxs,
		EnumInfos:         file_go_chromium_org_luci_logdog_api_logpb_butler_proto_enumTypes,
		MessageInfos:      file_go_chromium_org_luci_logdog_api_logpb_butler_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_logdog_api_logpb_butler_proto = out.File
	file_go_chromium_org_luci_logdog_api_logpb_butler_proto_goTypes = nil
	file_go_chromium_org_luci_logdog_api_logpb_butler_proto_depIdxs = nil
}
