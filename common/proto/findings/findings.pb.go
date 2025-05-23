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
// 	protoc        v6.30.2
// source: go.chromium.org/luci/common/proto/findings/findings.proto

package findings

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

type Finding_SeverityLevel int32

const (
	Finding_SEVERITY_LEVEL_UNSPECIFIED Finding_SeverityLevel = 0
	Finding_SEVERITY_LEVEL_INFO        Finding_SeverityLevel = 1
	Finding_SEVERITY_LEVEL_WARNING     Finding_SeverityLevel = 2
	Finding_SEVERITY_LEVEL_ERROR       Finding_SeverityLevel = 3
)

// Enum value maps for Finding_SeverityLevel.
var (
	Finding_SeverityLevel_name = map[int32]string{
		0: "SEVERITY_LEVEL_UNSPECIFIED",
		1: "SEVERITY_LEVEL_INFO",
		2: "SEVERITY_LEVEL_WARNING",
		3: "SEVERITY_LEVEL_ERROR",
	}
	Finding_SeverityLevel_value = map[string]int32{
		"SEVERITY_LEVEL_UNSPECIFIED": 0,
		"SEVERITY_LEVEL_INFO":        1,
		"SEVERITY_LEVEL_WARNING":     2,
		"SEVERITY_LEVEL_ERROR":       3,
	}
)

func (x Finding_SeverityLevel) Enum() *Finding_SeverityLevel {
	p := new(Finding_SeverityLevel)
	*p = x
	return p
}

func (x Finding_SeverityLevel) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Finding_SeverityLevel) Descriptor() protoreflect.EnumDescriptor {
	return file_go_chromium_org_luci_common_proto_findings_findings_proto_enumTypes[0].Descriptor()
}

func (Finding_SeverityLevel) Type() protoreflect.EnumType {
	return &file_go_chromium_org_luci_common_proto_findings_findings_proto_enumTypes[0]
}

func (x Finding_SeverityLevel) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Finding_SeverityLevel.Descriptor instead.
func (Finding_SeverityLevel) EnumDescriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_common_proto_findings_findings_proto_rawDescGZIP(), []int{1, 0}
}

// Findings are a collection of findings.
type Findings struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Findings      []*Finding             `protobuf:"bytes,1,rep,name=findings,proto3" json:"findings,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Findings) Reset() {
	*x = Findings{}
	mi := &file_go_chromium_org_luci_common_proto_findings_findings_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Findings) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Findings) ProtoMessage() {}

func (x *Findings) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_common_proto_findings_findings_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Findings.ProtoReflect.Descriptor instead.
func (*Findings) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_common_proto_findings_findings_proto_rawDescGZIP(), []int{0}
}

func (x *Findings) GetFindings() []*Finding {
	if x != nil {
		return x.Findings
	}
	return nil
}

// Finding represents a code finding, which can be a bug, vulnerability,
// style violation, or other issue identified in code.
type Finding struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Category of the code finding, e.g. "ClangTidy".
	Category string `protobuf:"bytes,1,opt,name=category,proto3" json:"category,omitempty"`
	// Location of the finding in the source code.
	Location *Location `protobuf:"bytes,2,opt,name=location,proto3" json:"location,omitempty"`
	// Human-readable message describing the finding.
	Message string `protobuf:"bytes,3,opt,name=message,proto3" json:"message,omitempty"`
	// Severity level of finding.
	//
	// In Gerrit, this controls what section of the checks UI the finding is
	// displayed under. Currently, the ERROR level findings won't block the CL
	// from submission. It will once go/long-term-cider-presubmits is implemented.
	SeverityLevel Finding_SeverityLevel `protobuf:"varint,4,opt,name=severity_level,json=severityLevel,proto3,enum=luci.findings.Finding_SeverityLevel" json:"severity_level,omitempty"`
	// Optional suggested fixes for the finding.
	//
	// If multiple fixes are present, they should be ordered by preference.
	Fixes []*Fix `protobuf:"bytes,5,rep,name=fixes,proto3" json:"fixes,omitempty"`
	// An optional URL that points to more detail about this finding.
	//
	// For example, the Buildbucket build that generates this finding.
	Url           string `protobuf:"bytes,6,opt,name=url,proto3" json:"url,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Finding) Reset() {
	*x = Finding{}
	mi := &file_go_chromium_org_luci_common_proto_findings_findings_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Finding) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Finding) ProtoMessage() {}

func (x *Finding) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_common_proto_findings_findings_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Finding.ProtoReflect.Descriptor instead.
func (*Finding) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_common_proto_findings_findings_proto_rawDescGZIP(), []int{1}
}

func (x *Finding) GetCategory() string {
	if x != nil {
		return x.Category
	}
	return ""
}

func (x *Finding) GetLocation() *Location {
	if x != nil {
		return x.Location
	}
	return nil
}

func (x *Finding) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

func (x *Finding) GetSeverityLevel() Finding_SeverityLevel {
	if x != nil {
		return x.SeverityLevel
	}
	return Finding_SEVERITY_LEVEL_UNSPECIFIED
}

func (x *Finding) GetFixes() []*Fix {
	if x != nil {
		return x.Fixes
	}
	return nil
}

func (x *Finding) GetUrl() string {
	if x != nil {
		return x.Url
	}
	return ""
}

// Location describes a location in the source code.
type Location struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Types that are valid to be assigned to Source:
	//
	//	*Location_GerritChangeRef
	Source isLocation_Source `protobuf_oneof:"source"`
	// Path to the file where the finding is located in the source.
	//
	// For Gerrit Change, "/COMMIT_MSG" is a special file path indicating the
	// location is in commit message.
	FilePath      string          `protobuf:"bytes,2,opt,name=file_path,json=filePath,proto3" json:"file_path,omitempty"`
	Range         *Location_Range `protobuf:"bytes,3,opt,name=range,proto3" json:"range,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Location) Reset() {
	*x = Location{}
	mi := &file_go_chromium_org_luci_common_proto_findings_findings_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Location) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Location) ProtoMessage() {}

func (x *Location) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_common_proto_findings_findings_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Location.ProtoReflect.Descriptor instead.
func (*Location) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_common_proto_findings_findings_proto_rawDescGZIP(), []int{2}
}

func (x *Location) GetSource() isLocation_Source {
	if x != nil {
		return x.Source
	}
	return nil
}

func (x *Location) GetGerritChangeRef() *Location_GerritChangeReference {
	if x != nil {
		if x, ok := x.Source.(*Location_GerritChangeRef); ok {
			return x.GerritChangeRef
		}
	}
	return nil
}

func (x *Location) GetFilePath() string {
	if x != nil {
		return x.FilePath
	}
	return ""
}

func (x *Location) GetRange() *Location_Range {
	if x != nil {
		return x.Range
	}
	return nil
}

type isLocation_Source interface {
	isLocation_Source()
}

type Location_GerritChangeRef struct {
	// Source from a Gerrit CL.
	GerritChangeRef *Location_GerritChangeReference `protobuf:"bytes,1,opt,name=gerrit_change_ref,json=gerritChangeRef,proto3,oneof"`
}

func (*Location_GerritChangeRef) isLocation_Source() {}

// A suggested fix for the finding.
type Fix struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Optional human-readable description of the fix.
	Description string `protobuf:"bytes,1,opt,name=description,proto3" json:"description,omitempty"`
	// Replacements to be applied to fix the finding.
	Replacements  []*Fix_Replacement `protobuf:"bytes,2,rep,name=replacements,proto3" json:"replacements,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Fix) Reset() {
	*x = Fix{}
	mi := &file_go_chromium_org_luci_common_proto_findings_findings_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Fix) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Fix) ProtoMessage() {}

func (x *Fix) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_common_proto_findings_findings_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Fix.ProtoReflect.Descriptor instead.
func (*Fix) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_common_proto_findings_findings_proto_rawDescGZIP(), []int{3}
}

func (x *Fix) GetDescription() string {
	if x != nil {
		return x.Description
	}
	return ""
}

func (x *Fix) GetReplacements() []*Fix_Replacement {
	if x != nil {
		return x.Replacements
	}
	return nil
}

type Location_GerritChangeReference struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Gerrit hostname, e.g. "chromium-review.googlesource.com".
	Host string `protobuf:"bytes,1,opt,name=host,proto3" json:"host,omitempty"`
	// Gerrit project, e.g. "chromium/src".
	Project string `protobuf:"bytes,2,opt,name=project,proto3" json:"project,omitempty"`
	// Change number, e.g. 12345.
	Change int64 `protobuf:"varint,3,opt,name=change,proto3" json:"change,omitempty"`
	// Patch set number, e.g. 1.
	Patchset      int64 `protobuf:"varint,4,opt,name=patchset,proto3" json:"patchset,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Location_GerritChangeReference) Reset() {
	*x = Location_GerritChangeReference{}
	mi := &file_go_chromium_org_luci_common_proto_findings_findings_proto_msgTypes[4]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Location_GerritChangeReference) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Location_GerritChangeReference) ProtoMessage() {}

func (x *Location_GerritChangeReference) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_common_proto_findings_findings_proto_msgTypes[4]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Location_GerritChangeReference.ProtoReflect.Descriptor instead.
func (*Location_GerritChangeReference) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_common_proto_findings_findings_proto_rawDescGZIP(), []int{2, 0}
}

func (x *Location_GerritChangeReference) GetHost() string {
	if x != nil {
		return x.Host
	}
	return ""
}

func (x *Location_GerritChangeReference) GetProject() string {
	if x != nil {
		return x.Project
	}
	return ""
}

func (x *Location_GerritChangeReference) GetChange() int64 {
	if x != nil {
		return x.Change
	}
	return 0
}

func (x *Location_GerritChangeReference) GetPatchset() int64 {
	if x != nil {
		return x.Patchset
	}
	return 0
}

// Range within the file where the finding is located.
//
// The semantic is the same as
// https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#comment-range
type Location_Range struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Start line of the range (1-based).
	StartLine int32 `protobuf:"varint,1,opt,name=start_line,json=startLine,proto3" json:"start_line,omitempty"`
	// Start column of the range (0-based).
	StartColumn int32 `protobuf:"varint,2,opt,name=start_column,json=startColumn,proto3" json:"start_column,omitempty"`
	// End line of the range (1-based).
	EndLine int32 `protobuf:"varint,3,opt,name=end_line,json=endLine,proto3" json:"end_line,omitempty"`
	// End column of the range (0-based).
	EndColumn     int32 `protobuf:"varint,4,opt,name=end_column,json=endColumn,proto3" json:"end_column,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Location_Range) Reset() {
	*x = Location_Range{}
	mi := &file_go_chromium_org_luci_common_proto_findings_findings_proto_msgTypes[5]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Location_Range) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Location_Range) ProtoMessage() {}

func (x *Location_Range) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_common_proto_findings_findings_proto_msgTypes[5]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Location_Range.ProtoReflect.Descriptor instead.
func (*Location_Range) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_common_proto_findings_findings_proto_rawDescGZIP(), []int{2, 1}
}

func (x *Location_Range) GetStartLine() int32 {
	if x != nil {
		return x.StartLine
	}
	return 0
}

func (x *Location_Range) GetStartColumn() int32 {
	if x != nil {
		return x.StartColumn
	}
	return 0
}

func (x *Location_Range) GetEndLine() int32 {
	if x != nil {
		return x.EndLine
	}
	return 0
}

func (x *Location_Range) GetEndColumn() int32 {
	if x != nil {
		return x.EndColumn
	}
	return 0
}

type Fix_Replacement struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Location of the content to be replaced.
	Location *Location `protobuf:"bytes,1,opt,name=location,proto3" json:"location,omitempty"`
	// New content to replace the old content.
	NewContent    string `protobuf:"bytes,2,opt,name=new_content,json=newContent,proto3" json:"new_content,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Fix_Replacement) Reset() {
	*x = Fix_Replacement{}
	mi := &file_go_chromium_org_luci_common_proto_findings_findings_proto_msgTypes[6]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Fix_Replacement) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Fix_Replacement) ProtoMessage() {}

func (x *Fix_Replacement) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_common_proto_findings_findings_proto_msgTypes[6]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Fix_Replacement.ProtoReflect.Descriptor instead.
func (*Fix_Replacement) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_common_proto_findings_findings_proto_rawDescGZIP(), []int{3, 0}
}

func (x *Fix_Replacement) GetLocation() *Location {
	if x != nil {
		return x.Location
	}
	return nil
}

func (x *Fix_Replacement) GetNewContent() string {
	if x != nil {
		return x.NewContent
	}
	return ""
}

var File_go_chromium_org_luci_common_proto_findings_findings_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_common_proto_findings_findings_proto_rawDesc = string([]byte{
	0x0a, 0x39, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x2f, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x2f, 0x66, 0x69, 0x6e, 0x64, 0x69, 0x6e, 0x67, 0x73, 0x2f, 0x66, 0x69, 0x6e,
	0x64, 0x69, 0x6e, 0x67, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0d, 0x6c, 0x75, 0x63,
	0x69, 0x2e, 0x66, 0x69, 0x6e, 0x64, 0x69, 0x6e, 0x67, 0x73, 0x22, 0x3e, 0x0a, 0x08, 0x46, 0x69,
	0x6e, 0x64, 0x69, 0x6e, 0x67, 0x73, 0x12, 0x32, 0x0a, 0x08, 0x66, 0x69, 0x6e, 0x64, 0x69, 0x6e,
	0x67, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x16, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e,
	0x66, 0x69, 0x6e, 0x64, 0x69, 0x6e, 0x67, 0x73, 0x2e, 0x46, 0x69, 0x6e, 0x64, 0x69, 0x6e, 0x67,
	0x52, 0x08, 0x66, 0x69, 0x6e, 0x64, 0x69, 0x6e, 0x67, 0x73, 0x22, 0xfd, 0x02, 0x0a, 0x07, 0x46,
	0x69, 0x6e, 0x64, 0x69, 0x6e, 0x67, 0x12, 0x1a, 0x0a, 0x08, 0x63, 0x61, 0x74, 0x65, 0x67, 0x6f,
	0x72, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x63, 0x61, 0x74, 0x65, 0x67, 0x6f,
	0x72, 0x79, 0x12, 0x33, 0x0a, 0x08, 0x6c, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x0b, 0x32, 0x17, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x66, 0x69, 0x6e, 0x64,
	0x69, 0x6e, 0x67, 0x73, 0x2e, 0x4c, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x08, 0x6c,
	0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x18, 0x0a, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61,
	0x67, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67,
	0x65, 0x12, 0x4b, 0x0a, 0x0e, 0x73, 0x65, 0x76, 0x65, 0x72, 0x69, 0x74, 0x79, 0x5f, 0x6c, 0x65,
	0x76, 0x65, 0x6c, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x24, 0x2e, 0x6c, 0x75, 0x63, 0x69,
	0x2e, 0x66, 0x69, 0x6e, 0x64, 0x69, 0x6e, 0x67, 0x73, 0x2e, 0x46, 0x69, 0x6e, 0x64, 0x69, 0x6e,
	0x67, 0x2e, 0x53, 0x65, 0x76, 0x65, 0x72, 0x69, 0x74, 0x79, 0x4c, 0x65, 0x76, 0x65, 0x6c, 0x52,
	0x0d, 0x73, 0x65, 0x76, 0x65, 0x72, 0x69, 0x74, 0x79, 0x4c, 0x65, 0x76, 0x65, 0x6c, 0x12, 0x28,
	0x0a, 0x05, 0x66, 0x69, 0x78, 0x65, 0x73, 0x18, 0x05, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x12, 0x2e,
	0x6c, 0x75, 0x63, 0x69, 0x2e, 0x66, 0x69, 0x6e, 0x64, 0x69, 0x6e, 0x67, 0x73, 0x2e, 0x46, 0x69,
	0x78, 0x52, 0x05, 0x66, 0x69, 0x78, 0x65, 0x73, 0x12, 0x10, 0x0a, 0x03, 0x75, 0x72, 0x6c, 0x18,
	0x06, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x75, 0x72, 0x6c, 0x22, 0x7e, 0x0a, 0x0d, 0x53, 0x65,
	0x76, 0x65, 0x72, 0x69, 0x74, 0x79, 0x4c, 0x65, 0x76, 0x65, 0x6c, 0x12, 0x1e, 0x0a, 0x1a, 0x53,
	0x45, 0x56, 0x45, 0x52, 0x49, 0x54, 0x59, 0x5f, 0x4c, 0x45, 0x56, 0x45, 0x4c, 0x5f, 0x55, 0x4e,
	0x53, 0x50, 0x45, 0x43, 0x49, 0x46, 0x49, 0x45, 0x44, 0x10, 0x00, 0x12, 0x17, 0x0a, 0x13, 0x53,
	0x45, 0x56, 0x45, 0x52, 0x49, 0x54, 0x59, 0x5f, 0x4c, 0x45, 0x56, 0x45, 0x4c, 0x5f, 0x49, 0x4e,
	0x46, 0x4f, 0x10, 0x01, 0x12, 0x1a, 0x0a, 0x16, 0x53, 0x45, 0x56, 0x45, 0x52, 0x49, 0x54, 0x59,
	0x5f, 0x4c, 0x45, 0x56, 0x45, 0x4c, 0x5f, 0x57, 0x41, 0x52, 0x4e, 0x49, 0x4e, 0x47, 0x10, 0x02,
	0x12, 0x18, 0x0a, 0x14, 0x53, 0x45, 0x56, 0x45, 0x52, 0x49, 0x54, 0x59, 0x5f, 0x4c, 0x45, 0x56,
	0x45, 0x4c, 0x5f, 0x45, 0x52, 0x52, 0x4f, 0x52, 0x10, 0x03, 0x22, 0xc4, 0x03, 0x0a, 0x08, 0x4c,
	0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x5b, 0x0a, 0x11, 0x67, 0x65, 0x72, 0x72, 0x69,
	0x74, 0x5f, 0x63, 0x68, 0x61, 0x6e, 0x67, 0x65, 0x5f, 0x72, 0x65, 0x66, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x0b, 0x32, 0x2d, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x66, 0x69, 0x6e, 0x64, 0x69, 0x6e,
	0x67, 0x73, 0x2e, 0x4c, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2e, 0x47, 0x65, 0x72, 0x72,
	0x69, 0x74, 0x43, 0x68, 0x61, 0x6e, 0x67, 0x65, 0x52, 0x65, 0x66, 0x65, 0x72, 0x65, 0x6e, 0x63,
	0x65, 0x48, 0x00, 0x52, 0x0f, 0x67, 0x65, 0x72, 0x72, 0x69, 0x74, 0x43, 0x68, 0x61, 0x6e, 0x67,
	0x65, 0x52, 0x65, 0x66, 0x12, 0x1b, 0x0a, 0x09, 0x66, 0x69, 0x6c, 0x65, 0x5f, 0x70, 0x61, 0x74,
	0x68, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x66, 0x69, 0x6c, 0x65, 0x50, 0x61, 0x74,
	0x68, 0x12, 0x33, 0x0a, 0x05, 0x72, 0x61, 0x6e, 0x67, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x1d, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x66, 0x69, 0x6e, 0x64, 0x69, 0x6e, 0x67, 0x73,
	0x2e, 0x4c, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2e, 0x52, 0x61, 0x6e, 0x67, 0x65, 0x52,
	0x05, 0x72, 0x61, 0x6e, 0x67, 0x65, 0x1a, 0x79, 0x0a, 0x15, 0x47, 0x65, 0x72, 0x72, 0x69, 0x74,
	0x43, 0x68, 0x61, 0x6e, 0x67, 0x65, 0x52, 0x65, 0x66, 0x65, 0x72, 0x65, 0x6e, 0x63, 0x65, 0x12,
	0x12, 0x0a, 0x04, 0x68, 0x6f, 0x73, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x68,
	0x6f, 0x73, 0x74, 0x12, 0x18, 0x0a, 0x07, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x70, 0x72, 0x6f, 0x6a, 0x65, 0x63, 0x74, 0x12, 0x16, 0x0a,
	0x06, 0x63, 0x68, 0x61, 0x6e, 0x67, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x03, 0x52, 0x06, 0x63,
	0x68, 0x61, 0x6e, 0x67, 0x65, 0x12, 0x1a, 0x0a, 0x08, 0x70, 0x61, 0x74, 0x63, 0x68, 0x73, 0x65,
	0x74, 0x18, 0x04, 0x20, 0x01, 0x28, 0x03, 0x52, 0x08, 0x70, 0x61, 0x74, 0x63, 0x68, 0x73, 0x65,
	0x74, 0x1a, 0x83, 0x01, 0x0a, 0x05, 0x52, 0x61, 0x6e, 0x67, 0x65, 0x12, 0x1d, 0x0a, 0x0a, 0x73,
	0x74, 0x61, 0x72, 0x74, 0x5f, 0x6c, 0x69, 0x6e, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x05, 0x52,
	0x09, 0x73, 0x74, 0x61, 0x72, 0x74, 0x4c, 0x69, 0x6e, 0x65, 0x12, 0x21, 0x0a, 0x0c, 0x73, 0x74,
	0x61, 0x72, 0x74, 0x5f, 0x63, 0x6f, 0x6c, 0x75, 0x6d, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x05,
	0x52, 0x0b, 0x73, 0x74, 0x61, 0x72, 0x74, 0x43, 0x6f, 0x6c, 0x75, 0x6d, 0x6e, 0x12, 0x19, 0x0a,
	0x08, 0x65, 0x6e, 0x64, 0x5f, 0x6c, 0x69, 0x6e, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x05, 0x52,
	0x07, 0x65, 0x6e, 0x64, 0x4c, 0x69, 0x6e, 0x65, 0x12, 0x1d, 0x0a, 0x0a, 0x65, 0x6e, 0x64, 0x5f,
	0x63, 0x6f, 0x6c, 0x75, 0x6d, 0x6e, 0x18, 0x04, 0x20, 0x01, 0x28, 0x05, 0x52, 0x09, 0x65, 0x6e,
	0x64, 0x43, 0x6f, 0x6c, 0x75, 0x6d, 0x6e, 0x42, 0x08, 0x0a, 0x06, 0x73, 0x6f, 0x75, 0x72, 0x63,
	0x65, 0x22, 0xd0, 0x01, 0x0a, 0x03, 0x46, 0x69, 0x78, 0x12, 0x20, 0x0a, 0x0b, 0x64, 0x65, 0x73,
	0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b,
	0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x42, 0x0a, 0x0c, 0x72,
	0x65, 0x70, 0x6c, 0x61, 0x63, 0x65, 0x6d, 0x65, 0x6e, 0x74, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28,
	0x0b, 0x32, 0x1e, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x66, 0x69, 0x6e, 0x64, 0x69, 0x6e, 0x67,
	0x73, 0x2e, 0x46, 0x69, 0x78, 0x2e, 0x52, 0x65, 0x70, 0x6c, 0x61, 0x63, 0x65, 0x6d, 0x65, 0x6e,
	0x74, 0x52, 0x0c, 0x72, 0x65, 0x70, 0x6c, 0x61, 0x63, 0x65, 0x6d, 0x65, 0x6e, 0x74, 0x73, 0x1a,
	0x63, 0x0a, 0x0b, 0x52, 0x65, 0x70, 0x6c, 0x61, 0x63, 0x65, 0x6d, 0x65, 0x6e, 0x74, 0x12, 0x33,
	0x0a, 0x08, 0x6c, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x17, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x66, 0x69, 0x6e, 0x64, 0x69, 0x6e, 0x67, 0x73,
	0x2e, 0x4c, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x08, 0x6c, 0x6f, 0x63, 0x61, 0x74,
	0x69, 0x6f, 0x6e, 0x12, 0x1f, 0x0a, 0x0b, 0x6e, 0x65, 0x77, 0x5f, 0x63, 0x6f, 0x6e, 0x74, 0x65,
	0x6e, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x6e, 0x65, 0x77, 0x43, 0x6f, 0x6e,
	0x74, 0x65, 0x6e, 0x74, 0x42, 0x35, 0x5a, 0x33, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d,
	0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x63, 0x6f, 0x6d,
	0x6d, 0x6f, 0x6e, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x66, 0x69, 0x6e, 0x64, 0x69, 0x6e,
	0x67, 0x73, 0x3b, 0x66, 0x69, 0x6e, 0x64, 0x69, 0x6e, 0x67, 0x73, 0x62, 0x06, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_common_proto_findings_findings_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_common_proto_findings_findings_proto_rawDescData []byte
)

func file_go_chromium_org_luci_common_proto_findings_findings_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_common_proto_findings_findings_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_common_proto_findings_findings_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_common_proto_findings_findings_proto_rawDesc), len(file_go_chromium_org_luci_common_proto_findings_findings_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_common_proto_findings_findings_proto_rawDescData
}

var file_go_chromium_org_luci_common_proto_findings_findings_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_go_chromium_org_luci_common_proto_findings_findings_proto_msgTypes = make([]protoimpl.MessageInfo, 7)
var file_go_chromium_org_luci_common_proto_findings_findings_proto_goTypes = []any{
	(Finding_SeverityLevel)(0),             // 0: luci.findings.Finding.SeverityLevel
	(*Findings)(nil),                       // 1: luci.findings.Findings
	(*Finding)(nil),                        // 2: luci.findings.Finding
	(*Location)(nil),                       // 3: luci.findings.Location
	(*Fix)(nil),                            // 4: luci.findings.Fix
	(*Location_GerritChangeReference)(nil), // 5: luci.findings.Location.GerritChangeReference
	(*Location_Range)(nil),                 // 6: luci.findings.Location.Range
	(*Fix_Replacement)(nil),                // 7: luci.findings.Fix.Replacement
}
var file_go_chromium_org_luci_common_proto_findings_findings_proto_depIdxs = []int32{
	2, // 0: luci.findings.Findings.findings:type_name -> luci.findings.Finding
	3, // 1: luci.findings.Finding.location:type_name -> luci.findings.Location
	0, // 2: luci.findings.Finding.severity_level:type_name -> luci.findings.Finding.SeverityLevel
	4, // 3: luci.findings.Finding.fixes:type_name -> luci.findings.Fix
	5, // 4: luci.findings.Location.gerrit_change_ref:type_name -> luci.findings.Location.GerritChangeReference
	6, // 5: luci.findings.Location.range:type_name -> luci.findings.Location.Range
	7, // 6: luci.findings.Fix.replacements:type_name -> luci.findings.Fix.Replacement
	3, // 7: luci.findings.Fix.Replacement.location:type_name -> luci.findings.Location
	8, // [8:8] is the sub-list for method output_type
	8, // [8:8] is the sub-list for method input_type
	8, // [8:8] is the sub-list for extension type_name
	8, // [8:8] is the sub-list for extension extendee
	0, // [0:8] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_common_proto_findings_findings_proto_init() }
func file_go_chromium_org_luci_common_proto_findings_findings_proto_init() {
	if File_go_chromium_org_luci_common_proto_findings_findings_proto != nil {
		return
	}
	file_go_chromium_org_luci_common_proto_findings_findings_proto_msgTypes[2].OneofWrappers = []any{
		(*Location_GerritChangeRef)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_common_proto_findings_findings_proto_rawDesc), len(file_go_chromium_org_luci_common_proto_findings_findings_proto_rawDesc)),
			NumEnums:      1,
			NumMessages:   7,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_common_proto_findings_findings_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_common_proto_findings_findings_proto_depIdxs,
		EnumInfos:         file_go_chromium_org_luci_common_proto_findings_findings_proto_enumTypes,
		MessageInfos:      file_go_chromium_org_luci_common_proto_findings_findings_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_common_proto_findings_findings_proto = out.File
	file_go_chromium_org_luci_common_proto_findings_findings_proto_goTypes = nil
	file_go_chromium_org_luci_common_proto_findings_findings_proto_depIdxs = nil
}
