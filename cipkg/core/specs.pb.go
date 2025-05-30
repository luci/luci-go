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
// source: go.chromium.org/luci/cipkg/core/specs.proto

package core

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

// HashAlgorithm includes all supported hash algorithms shared by actions.
type HashAlgorithm int32

const (
	HashAlgorithm_HASH_UNSPECIFIED HashAlgorithm = 0
	HashAlgorithm_HASH_MD5         HashAlgorithm = 1
	HashAlgorithm_HASH_SHA256      HashAlgorithm = 2
)

// Enum value maps for HashAlgorithm.
var (
	HashAlgorithm_name = map[int32]string{
		0: "HASH_UNSPECIFIED",
		1: "HASH_MD5",
		2: "HASH_SHA256",
	}
	HashAlgorithm_value = map[string]int32{
		"HASH_UNSPECIFIED": 0,
		"HASH_MD5":         1,
		"HASH_SHA256":      2,
	}
)

func (x HashAlgorithm) Enum() *HashAlgorithm {
	p := new(HashAlgorithm)
	*p = x
	return p
}

func (x HashAlgorithm) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (HashAlgorithm) Descriptor() protoreflect.EnumDescriptor {
	return file_go_chromium_org_luci_cipkg_core_specs_proto_enumTypes[0].Descriptor()
}

func (HashAlgorithm) Type() protoreflect.EnumType {
	return &file_go_chromium_org_luci_cipkg_core_specs_proto_enumTypes[0]
}

func (x HashAlgorithm) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use HashAlgorithm.Descriptor instead.
func (HashAlgorithm) EnumDescriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_cipkg_core_specs_proto_rawDescGZIP(), []int{0}
}

// ActionCommand executes the command directly.
// Template is supported in both args and env: {{.depName}} will be replaced by
// the output path of the action's dependency with depName.
// e.g. An ActionCommand with python3 as dependency:
//
//	&core.Action{
//	  Name: "some_script",
//	  Deps: []*core.Action_Dependency{Python3Action},
//	  Spec: &core.Action_Command{
//	    Command: &core.ActionCommand{
//	      Args: []string{"{{.python3}}/bin/python3", "something.py")},
//	    },
//	  }
//	}
//
// "{{.python3}}/bin/python3" will be replaced by the output path in the
// transformed derivation.
type ActionCommand struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Args          []string               `protobuf:"bytes,1,rep,name=args,proto3" json:"args,omitempty"`
	Env           []string               `protobuf:"bytes,2,rep,name=env,proto3" json:"env,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ActionCommand) Reset() {
	*x = ActionCommand{}
	mi := &file_go_chromium_org_luci_cipkg_core_specs_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ActionCommand) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ActionCommand) ProtoMessage() {}

func (x *ActionCommand) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_cipkg_core_specs_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ActionCommand.ProtoReflect.Descriptor instead.
func (*ActionCommand) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_cipkg_core_specs_proto_rawDescGZIP(), []int{0}
}

func (x *ActionCommand) GetArgs() []string {
	if x != nil {
		return x.Args
	}
	return nil
}

func (x *ActionCommand) GetEnv() []string {
	if x != nil {
		return x.Env
	}
	return nil
}

// ActionURLFetch downloads from url into output directory with name
// 'file'.
type ActionURLFetch struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// HTTP(s) url for the remote resource.
	Url string `protobuf:"bytes,1,opt,name=url,proto3" json:"url,omitempty"`
	// HashAlgorithm is the hash function used to calculate hash value.
	HashAlgorithm HashAlgorithm `protobuf:"varint,2,opt,name=hash_algorithm,json=hashAlgorithm,proto3,enum=HashAlgorithm" json:"hash_algorithm,omitempty"`
	// HashValue is the lower-case text representation for the hex value of the
	// hash sum.
	HashValue string `protobuf:"bytes,3,opt,name=hash_value,json=hashValue,proto3" json:"hash_value,omitempty"`
	// Name for the downloaded file. If empty, "file" will be used.
	Name string `protobuf:"bytes,4,opt,name=Name,proto3" json:"Name,omitempty"`
	// File mode for the downloaded resource. If empty, 0666 will be used.
	Mode          uint32 `protobuf:"varint,5,opt,name=mode,proto3" json:"mode,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ActionURLFetch) Reset() {
	*x = ActionURLFetch{}
	mi := &file_go_chromium_org_luci_cipkg_core_specs_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ActionURLFetch) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ActionURLFetch) ProtoMessage() {}

func (x *ActionURLFetch) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_cipkg_core_specs_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ActionURLFetch.ProtoReflect.Descriptor instead.
func (*ActionURLFetch) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_cipkg_core_specs_proto_rawDescGZIP(), []int{1}
}

func (x *ActionURLFetch) GetUrl() string {
	if x != nil {
		return x.Url
	}
	return ""
}

func (x *ActionURLFetch) GetHashAlgorithm() HashAlgorithm {
	if x != nil {
		return x.HashAlgorithm
	}
	return HashAlgorithm_HASH_UNSPECIFIED
}

func (x *ActionURLFetch) GetHashValue() string {
	if x != nil {
		return x.HashValue
	}
	return ""
}

func (x *ActionURLFetch) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *ActionURLFetch) GetMode() uint32 {
	if x != nil {
		return x.Mode
	}
	return 0
}

// ActionFilesCopy copies listed files into the output directory.
// TODO(fancl): Local, Embed, Output can be separated into different specs?
type ActionFilesCopy struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Files are the destination-source pairs which is the relative path
	// source files should be copied to.
	Files         map[string]*ActionFilesCopy_Source `protobuf:"bytes,1,rep,name=files,proto3" json:"files,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ActionFilesCopy) Reset() {
	*x = ActionFilesCopy{}
	mi := &file_go_chromium_org_luci_cipkg_core_specs_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ActionFilesCopy) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ActionFilesCopy) ProtoMessage() {}

func (x *ActionFilesCopy) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_cipkg_core_specs_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ActionFilesCopy.ProtoReflect.Descriptor instead.
func (*ActionFilesCopy) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_cipkg_core_specs_proto_rawDescGZIP(), []int{2}
}

func (x *ActionFilesCopy) GetFiles() map[string]*ActionFilesCopy_Source {
	if x != nil {
		return x.Files
	}
	return nil
}

// ActionCIPDExport exports cipd packages to the output directory.
// NOTE: this uses cipd export subcommand, which only includes CIPD package
// content without any cipd tracking metadata.
// TODO(fancl): use a protobuf ensure file instead.
type ActionCIPDExport struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Ensure_file is the serialized text ensure file for cipd.
	EnsureFile string `protobuf:"bytes,1,opt,name=ensure_file,json=ensureFile,proto3" json:"ensure_file,omitempty"`
	// Env is the extra environment variables passed to the cipd process.
	Env           []string `protobuf:"bytes,2,rep,name=env,proto3" json:"env,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ActionCIPDExport) Reset() {
	*x = ActionCIPDExport{}
	mi := &file_go_chromium_org_luci_cipkg_core_specs_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ActionCIPDExport) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ActionCIPDExport) ProtoMessage() {}

func (x *ActionCIPDExport) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_cipkg_core_specs_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ActionCIPDExport.ProtoReflect.Descriptor instead.
func (*ActionCIPDExport) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_cipkg_core_specs_proto_rawDescGZIP(), []int{3}
}

func (x *ActionCIPDExport) GetEnsureFile() string {
	if x != nil {
		return x.EnsureFile
	}
	return ""
}

func (x *ActionCIPDExport) GetEnv() []string {
	if x != nil {
		return x.Env
	}
	return nil
}

// Source defines the source file we want to copied from.
type ActionFilesCopy_Source struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Mode is unix file mode.
	Mode uint32 `protobuf:"varint,1,opt,name=mode,proto3" json:"mode,omitempty"`
	// Win_attrs is windows file attributes.
	WinAttrs uint32 `protobuf:"varint,7,opt,name=win_attrs,json=winAttrs,proto3" json:"win_attrs,omitempty"`
	// Version is the version string for the file.
	Version string `protobuf:"bytes,6,opt,name=version,proto3" json:"version,omitempty"`
	// Types that are valid to be assigned to Content:
	//
	//	*ActionFilesCopy_Source_Raw
	//	*ActionFilesCopy_Source_Local_
	//	*ActionFilesCopy_Source_Embed_
	//	*ActionFilesCopy_Source_Output_
	Content       isActionFilesCopy_Source_Content `protobuf_oneof:"content"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ActionFilesCopy_Source) Reset() {
	*x = ActionFilesCopy_Source{}
	mi := &file_go_chromium_org_luci_cipkg_core_specs_proto_msgTypes[5]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ActionFilesCopy_Source) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ActionFilesCopy_Source) ProtoMessage() {}

func (x *ActionFilesCopy_Source) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_cipkg_core_specs_proto_msgTypes[5]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ActionFilesCopy_Source.ProtoReflect.Descriptor instead.
func (*ActionFilesCopy_Source) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_cipkg_core_specs_proto_rawDescGZIP(), []int{2, 1}
}

func (x *ActionFilesCopy_Source) GetMode() uint32 {
	if x != nil {
		return x.Mode
	}
	return 0
}

func (x *ActionFilesCopy_Source) GetWinAttrs() uint32 {
	if x != nil {
		return x.WinAttrs
	}
	return 0
}

func (x *ActionFilesCopy_Source) GetVersion() string {
	if x != nil {
		return x.Version
	}
	return ""
}

func (x *ActionFilesCopy_Source) GetContent() isActionFilesCopy_Source_Content {
	if x != nil {
		return x.Content
	}
	return nil
}

func (x *ActionFilesCopy_Source) GetRaw() []byte {
	if x != nil {
		if x, ok := x.Content.(*ActionFilesCopy_Source_Raw); ok {
			return x.Raw
		}
	}
	return nil
}

func (x *ActionFilesCopy_Source) GetLocal() *ActionFilesCopy_Source_Local {
	if x != nil {
		if x, ok := x.Content.(*ActionFilesCopy_Source_Local_); ok {
			return x.Local
		}
	}
	return nil
}

func (x *ActionFilesCopy_Source) GetEmbed() *ActionFilesCopy_Source_Embed {
	if x != nil {
		if x, ok := x.Content.(*ActionFilesCopy_Source_Embed_); ok {
			return x.Embed
		}
	}
	return nil
}

func (x *ActionFilesCopy_Source) GetOutput() *ActionFilesCopy_Source_Output {
	if x != nil {
		if x, ok := x.Content.(*ActionFilesCopy_Source_Output_); ok {
			return x.Output
		}
	}
	return nil
}

type isActionFilesCopy_Source_Content interface {
	isActionFilesCopy_Source_Content()
}

type ActionFilesCopy_Source_Raw struct {
	// Raw contains the literal content of the file.
	Raw []byte `protobuf:"bytes,2,opt,name=raw,proto3,oneof"`
}

type ActionFilesCopy_Source_Local_ struct {
	// Local refers to the local file.
	Local *ActionFilesCopy_Source_Local `protobuf:"bytes,3,opt,name=local,proto3,oneof"`
}

type ActionFilesCopy_Source_Embed_ struct {
	// Embed refers to the embedded file in the go binary.
	Embed *ActionFilesCopy_Source_Embed `protobuf:"bytes,4,opt,name=embed,proto3,oneof"`
}

type ActionFilesCopy_Source_Output_ struct {
	// Output refers to the output file from other derivations.
	Output *ActionFilesCopy_Source_Output `protobuf:"bytes,5,opt,name=output,proto3,oneof"`
}

func (*ActionFilesCopy_Source_Raw) isActionFilesCopy_Source_Content() {}

func (*ActionFilesCopy_Source_Local_) isActionFilesCopy_Source_Content() {}

func (*ActionFilesCopy_Source_Embed_) isActionFilesCopy_Source_Content() {}

func (*ActionFilesCopy_Source_Output_) isActionFilesCopy_Source_Content() {}

type ActionFilesCopy_Source_Local struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Path is the local filesystem absolute path to the file.
	Path string `protobuf:"bytes,1,opt,name=path,proto3" json:"path,omitempty"`
	// Follow_symlinks, if set to true, will follow all the symlinks in the
	// directories while copying.
	FollowSymlinks bool `protobuf:"varint,3,opt,name=follow_symlinks,json=followSymlinks,proto3" json:"follow_symlinks,omitempty"`
	unknownFields  protoimpl.UnknownFields
	sizeCache      protoimpl.SizeCache
}

func (x *ActionFilesCopy_Source_Local) Reset() {
	*x = ActionFilesCopy_Source_Local{}
	mi := &file_go_chromium_org_luci_cipkg_core_specs_proto_msgTypes[6]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ActionFilesCopy_Source_Local) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ActionFilesCopy_Source_Local) ProtoMessage() {}

func (x *ActionFilesCopy_Source_Local) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_cipkg_core_specs_proto_msgTypes[6]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ActionFilesCopy_Source_Local.ProtoReflect.Descriptor instead.
func (*ActionFilesCopy_Source_Local) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_cipkg_core_specs_proto_rawDescGZIP(), []int{2, 1, 0}
}

func (x *ActionFilesCopy_Source_Local) GetPath() string {
	if x != nil {
		return x.Path
	}
	return ""
}

func (x *ActionFilesCopy_Source_Local) GetFollowSymlinks() bool {
	if x != nil {
		return x.FollowSymlinks
	}
	return false
}

type ActionFilesCopy_Source_Embed struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Ref is the reference to the embed.FS.
	Ref string `protobuf:"bytes,1,opt,name=ref,proto3" json:"ref,omitempty"`
	// Path is the relative path to the file inside the embedded filesystem.
	Path          string `protobuf:"bytes,2,opt,name=path,proto3" json:"path,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ActionFilesCopy_Source_Embed) Reset() {
	*x = ActionFilesCopy_Source_Embed{}
	mi := &file_go_chromium_org_luci_cipkg_core_specs_proto_msgTypes[7]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ActionFilesCopy_Source_Embed) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ActionFilesCopy_Source_Embed) ProtoMessage() {}

func (x *ActionFilesCopy_Source_Embed) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_cipkg_core_specs_proto_msgTypes[7]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ActionFilesCopy_Source_Embed.ProtoReflect.Descriptor instead.
func (*ActionFilesCopy_Source_Embed) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_cipkg_core_specs_proto_rawDescGZIP(), []int{2, 1, 1}
}

func (x *ActionFilesCopy_Source_Embed) GetRef() string {
	if x != nil {
		return x.Ref
	}
	return ""
}

func (x *ActionFilesCopy_Source_Embed) GetPath() string {
	if x != nil {
		return x.Path
	}
	return ""
}

type ActionFilesCopy_Source_Output struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Name is the output action's Metadata.Name
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Path is the relative path to the file inside the output.
	Path          string `protobuf:"bytes,2,opt,name=path,proto3" json:"path,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *ActionFilesCopy_Source_Output) Reset() {
	*x = ActionFilesCopy_Source_Output{}
	mi := &file_go_chromium_org_luci_cipkg_core_specs_proto_msgTypes[8]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ActionFilesCopy_Source_Output) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ActionFilesCopy_Source_Output) ProtoMessage() {}

func (x *ActionFilesCopy_Source_Output) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_cipkg_core_specs_proto_msgTypes[8]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ActionFilesCopy_Source_Output.ProtoReflect.Descriptor instead.
func (*ActionFilesCopy_Source_Output) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_cipkg_core_specs_proto_rawDescGZIP(), []int{2, 1, 2}
}

func (x *ActionFilesCopy_Source_Output) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *ActionFilesCopy_Source_Output) GetPath() string {
	if x != nil {
		return x.Path
	}
	return ""
}

var File_go_chromium_org_luci_cipkg_core_specs_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_cipkg_core_specs_proto_rawDesc = string([]byte{
	0x0a, 0x2b, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x63, 0x69, 0x70, 0x6b, 0x67, 0x2f, 0x63, 0x6f, 0x72,
	0x65, 0x2f, 0x73, 0x70, 0x65, 0x63, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x35, 0x0a,
	0x0d, 0x41, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x43, 0x6f, 0x6d, 0x6d, 0x61, 0x6e, 0x64, 0x12, 0x12,
	0x0a, 0x04, 0x61, 0x72, 0x67, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x09, 0x52, 0x04, 0x61, 0x72,
	0x67, 0x73, 0x12, 0x10, 0x0a, 0x03, 0x65, 0x6e, 0x76, 0x18, 0x02, 0x20, 0x03, 0x28, 0x09, 0x52,
	0x03, 0x65, 0x6e, 0x76, 0x22, 0xa0, 0x01, 0x0a, 0x0e, 0x41, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x55,
	0x52, 0x4c, 0x46, 0x65, 0x74, 0x63, 0x68, 0x12, 0x10, 0x0a, 0x03, 0x75, 0x72, 0x6c, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x75, 0x72, 0x6c, 0x12, 0x35, 0x0a, 0x0e, 0x68, 0x61, 0x73,
	0x68, 0x5f, 0x61, 0x6c, 0x67, 0x6f, 0x72, 0x69, 0x74, 0x68, 0x6d, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x0e, 0x32, 0x0e, 0x2e, 0x48, 0x61, 0x73, 0x68, 0x41, 0x6c, 0x67, 0x6f, 0x72, 0x69, 0x74, 0x68,
	0x6d, 0x52, 0x0d, 0x68, 0x61, 0x73, 0x68, 0x41, 0x6c, 0x67, 0x6f, 0x72, 0x69, 0x74, 0x68, 0x6d,
	0x12, 0x1d, 0x0a, 0x0a, 0x68, 0x61, 0x73, 0x68, 0x5f, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x03,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x68, 0x61, 0x73, 0x68, 0x56, 0x61, 0x6c, 0x75, 0x65, 0x12,
	0x12, 0x0a, 0x04, 0x4e, 0x61, 0x6d, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x4e,
	0x61, 0x6d, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x6d, 0x6f, 0x64, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28,
	0x0d, 0x52, 0x04, 0x6d, 0x6f, 0x64, 0x65, 0x22, 0xdb, 0x04, 0x0a, 0x0f, 0x41, 0x63, 0x74, 0x69,
	0x6f, 0x6e, 0x46, 0x69, 0x6c, 0x65, 0x73, 0x43, 0x6f, 0x70, 0x79, 0x12, 0x31, 0x0a, 0x05, 0x66,
	0x69, 0x6c, 0x65, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x1b, 0x2e, 0x41, 0x63, 0x74,
	0x69, 0x6f, 0x6e, 0x46, 0x69, 0x6c, 0x65, 0x73, 0x43, 0x6f, 0x70, 0x79, 0x2e, 0x46, 0x69, 0x6c,
	0x65, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x05, 0x66, 0x69, 0x6c, 0x65, 0x73, 0x1a, 0x51,
	0x0a, 0x0a, 0x46, 0x69, 0x6c, 0x65, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03,
	0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x2d,
	0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x17, 0x2e,
	0x41, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x46, 0x69, 0x6c, 0x65, 0x73, 0x43, 0x6f, 0x70, 0x79, 0x2e,
	0x53, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x3a, 0x02, 0x38,
	0x01, 0x1a, 0xc1, 0x03, 0x0a, 0x06, 0x53, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x12, 0x12, 0x0a, 0x04,
	0x6d, 0x6f, 0x64, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x04, 0x6d, 0x6f, 0x64, 0x65,
	0x12, 0x1b, 0x0a, 0x09, 0x77, 0x69, 0x6e, 0x5f, 0x61, 0x74, 0x74, 0x72, 0x73, 0x18, 0x07, 0x20,
	0x01, 0x28, 0x0d, 0x52, 0x08, 0x77, 0x69, 0x6e, 0x41, 0x74, 0x74, 0x72, 0x73, 0x12, 0x18, 0x0a,
	0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x06, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07,
	0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x12, 0x12, 0x0a, 0x03, 0x72, 0x61, 0x77, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x0c, 0x48, 0x00, 0x52, 0x03, 0x72, 0x61, 0x77, 0x12, 0x35, 0x0a, 0x05, 0x6c,
	0x6f, 0x63, 0x61, 0x6c, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1d, 0x2e, 0x41, 0x63, 0x74,
	0x69, 0x6f, 0x6e, 0x46, 0x69, 0x6c, 0x65, 0x73, 0x43, 0x6f, 0x70, 0x79, 0x2e, 0x53, 0x6f, 0x75,
	0x72, 0x63, 0x65, 0x2e, 0x4c, 0x6f, 0x63, 0x61, 0x6c, 0x48, 0x00, 0x52, 0x05, 0x6c, 0x6f, 0x63,
	0x61, 0x6c, 0x12, 0x35, 0x0a, 0x05, 0x65, 0x6d, 0x62, 0x65, 0x64, 0x18, 0x04, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x1d, 0x2e, 0x41, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x46, 0x69, 0x6c, 0x65, 0x73, 0x43,
	0x6f, 0x70, 0x79, 0x2e, 0x53, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x2e, 0x45, 0x6d, 0x62, 0x65, 0x64,
	0x48, 0x00, 0x52, 0x05, 0x65, 0x6d, 0x62, 0x65, 0x64, 0x12, 0x38, 0x0a, 0x06, 0x6f, 0x75, 0x74,
	0x70, 0x75, 0x74, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1e, 0x2e, 0x41, 0x63, 0x74, 0x69,
	0x6f, 0x6e, 0x46, 0x69, 0x6c, 0x65, 0x73, 0x43, 0x6f, 0x70, 0x79, 0x2e, 0x53, 0x6f, 0x75, 0x72,
	0x63, 0x65, 0x2e, 0x4f, 0x75, 0x74, 0x70, 0x75, 0x74, 0x48, 0x00, 0x52, 0x06, 0x6f, 0x75, 0x74,
	0x70, 0x75, 0x74, 0x1a, 0x44, 0x0a, 0x05, 0x4c, 0x6f, 0x63, 0x61, 0x6c, 0x12, 0x12, 0x0a, 0x04,
	0x70, 0x61, 0x74, 0x68, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x70, 0x61, 0x74, 0x68,
	0x12, 0x27, 0x0a, 0x0f, 0x66, 0x6f, 0x6c, 0x6c, 0x6f, 0x77, 0x5f, 0x73, 0x79, 0x6d, 0x6c, 0x69,
	0x6e, 0x6b, 0x73, 0x18, 0x03, 0x20, 0x01, 0x28, 0x08, 0x52, 0x0e, 0x66, 0x6f, 0x6c, 0x6c, 0x6f,
	0x77, 0x53, 0x79, 0x6d, 0x6c, 0x69, 0x6e, 0x6b, 0x73, 0x1a, 0x2d, 0x0a, 0x05, 0x45, 0x6d, 0x62,
	0x65, 0x64, 0x12, 0x10, 0x0a, 0x03, 0x72, 0x65, 0x66, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x03, 0x72, 0x65, 0x66, 0x12, 0x12, 0x0a, 0x04, 0x70, 0x61, 0x74, 0x68, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x04, 0x70, 0x61, 0x74, 0x68, 0x1a, 0x30, 0x0a, 0x06, 0x4f, 0x75, 0x74, 0x70,
	0x75, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x70, 0x61, 0x74, 0x68, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x70, 0x61, 0x74, 0x68, 0x42, 0x09, 0x0a, 0x07, 0x63, 0x6f,
	0x6e, 0x74, 0x65, 0x6e, 0x74, 0x22, 0x45, 0x0a, 0x10, 0x41, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x43,
	0x49, 0x50, 0x44, 0x45, 0x78, 0x70, 0x6f, 0x72, 0x74, 0x12, 0x1f, 0x0a, 0x0b, 0x65, 0x6e, 0x73,
	0x75, 0x72, 0x65, 0x5f, 0x66, 0x69, 0x6c, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a,
	0x65, 0x6e, 0x73, 0x75, 0x72, 0x65, 0x46, 0x69, 0x6c, 0x65, 0x12, 0x10, 0x0a, 0x03, 0x65, 0x6e,
	0x76, 0x18, 0x02, 0x20, 0x03, 0x28, 0x09, 0x52, 0x03, 0x65, 0x6e, 0x76, 0x2a, 0x44, 0x0a, 0x0d,
	0x48, 0x61, 0x73, 0x68, 0x41, 0x6c, 0x67, 0x6f, 0x72, 0x69, 0x74, 0x68, 0x6d, 0x12, 0x14, 0x0a,
	0x10, 0x48, 0x41, 0x53, 0x48, 0x5f, 0x55, 0x4e, 0x53, 0x50, 0x45, 0x43, 0x49, 0x46, 0x49, 0x45,
	0x44, 0x10, 0x00, 0x12, 0x0c, 0x0a, 0x08, 0x48, 0x41, 0x53, 0x48, 0x5f, 0x4d, 0x44, 0x35, 0x10,
	0x01, 0x12, 0x0f, 0x0a, 0x0b, 0x48, 0x41, 0x53, 0x48, 0x5f, 0x53, 0x48, 0x41, 0x32, 0x35, 0x36,
	0x10, 0x02, 0x42, 0x21, 0x5a, 0x1f, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75,
	0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x63, 0x69, 0x70, 0x6b, 0x67,
	0x2f, 0x63, 0x6f, 0x72, 0x65, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_cipkg_core_specs_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_cipkg_core_specs_proto_rawDescData []byte
)

func file_go_chromium_org_luci_cipkg_core_specs_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_cipkg_core_specs_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_cipkg_core_specs_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_cipkg_core_specs_proto_rawDesc), len(file_go_chromium_org_luci_cipkg_core_specs_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_cipkg_core_specs_proto_rawDescData
}

var file_go_chromium_org_luci_cipkg_core_specs_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_go_chromium_org_luci_cipkg_core_specs_proto_msgTypes = make([]protoimpl.MessageInfo, 9)
var file_go_chromium_org_luci_cipkg_core_specs_proto_goTypes = []any{
	(HashAlgorithm)(0),                    // 0: HashAlgorithm
	(*ActionCommand)(nil),                 // 1: ActionCommand
	(*ActionURLFetch)(nil),                // 2: ActionURLFetch
	(*ActionFilesCopy)(nil),               // 3: ActionFilesCopy
	(*ActionCIPDExport)(nil),              // 4: ActionCIPDExport
	nil,                                   // 5: ActionFilesCopy.FilesEntry
	(*ActionFilesCopy_Source)(nil),        // 6: ActionFilesCopy.Source
	(*ActionFilesCopy_Source_Local)(nil),  // 7: ActionFilesCopy.Source.Local
	(*ActionFilesCopy_Source_Embed)(nil),  // 8: ActionFilesCopy.Source.Embed
	(*ActionFilesCopy_Source_Output)(nil), // 9: ActionFilesCopy.Source.Output
}
var file_go_chromium_org_luci_cipkg_core_specs_proto_depIdxs = []int32{
	0, // 0: ActionURLFetch.hash_algorithm:type_name -> HashAlgorithm
	5, // 1: ActionFilesCopy.files:type_name -> ActionFilesCopy.FilesEntry
	6, // 2: ActionFilesCopy.FilesEntry.value:type_name -> ActionFilesCopy.Source
	7, // 3: ActionFilesCopy.Source.local:type_name -> ActionFilesCopy.Source.Local
	8, // 4: ActionFilesCopy.Source.embed:type_name -> ActionFilesCopy.Source.Embed
	9, // 5: ActionFilesCopy.Source.output:type_name -> ActionFilesCopy.Source.Output
	6, // [6:6] is the sub-list for method output_type
	6, // [6:6] is the sub-list for method input_type
	6, // [6:6] is the sub-list for extension type_name
	6, // [6:6] is the sub-list for extension extendee
	0, // [0:6] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_cipkg_core_specs_proto_init() }
func file_go_chromium_org_luci_cipkg_core_specs_proto_init() {
	if File_go_chromium_org_luci_cipkg_core_specs_proto != nil {
		return
	}
	file_go_chromium_org_luci_cipkg_core_specs_proto_msgTypes[5].OneofWrappers = []any{
		(*ActionFilesCopy_Source_Raw)(nil),
		(*ActionFilesCopy_Source_Local_)(nil),
		(*ActionFilesCopy_Source_Embed_)(nil),
		(*ActionFilesCopy_Source_Output_)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_cipkg_core_specs_proto_rawDesc), len(file_go_chromium_org_luci_cipkg_core_specs_proto_rawDesc)),
			NumEnums:      1,
			NumMessages:   9,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_cipkg_core_specs_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_cipkg_core_specs_proto_depIdxs,
		EnumInfos:         file_go_chromium_org_luci_cipkg_core_specs_proto_enumTypes,
		MessageInfos:      file_go_chromium_org_luci_cipkg_core_specs_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_cipkg_core_specs_proto = out.File
	file_go_chromium_org_luci_cipkg_core_specs_proto_goTypes = nil
	file_go_chromium_org_luci_cipkg_core_specs_proto_depIdxs = nil
}
