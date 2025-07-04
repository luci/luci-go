// Copyright 2025 The LUCI Authors.
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
// 	protoc        v6.31.1
// source: go.chromium.org/luci/lucicfg/lockfilepb/lockfile.proto

package lockfilepb

import (
	_ "go.chromium.org/luci/common/proto"
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

// Lockfile captures the result of loading PACKAGE.star and traversing its
// dependencies.
//
// It is serialized as JSONPB and stored in PACKAGE.lock file. This happens as
// part of "lucicfg gen" execution.
//
// Lockfiles are used to quickly configure the interpreter for executing
// Starlark code (with dependencies) without reparsing PACKAGE.star and doing
// dependency traversal and version resolution again.
type Lockfile struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Version of lucicfg used to generate this lockfile.
	Lucicfg       string              `protobuf:"bytes,1,opt,name=lucicfg,proto3" json:"lucicfg,omitempty"`
	Packages      []*Lockfile_Package `protobuf:"bytes,2,rep,name=packages,proto3" json:"packages,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Lockfile) Reset() {
	*x = Lockfile{}
	mi := &file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Lockfile) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Lockfile) ProtoMessage() {}

func (x *Lockfile) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Lockfile.ProtoReflect.Descriptor instead.
func (*Lockfile) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_rawDescGZIP(), []int{0}
}

func (x *Lockfile) GetLucicfg() string {
	if x != nil {
		return x.Lucicfg
	}
	return ""
}

func (x *Lockfile) GetPackages() []*Lockfile_Package {
	if x != nil {
		return x.Packages
	}
	return nil
}

// Packages are the main package along with all its transitive dependencies.
//
// The main package comes first. All other packages are sorted alphabetically
// by their name.
type Lockfile_Package struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Name is the package name as "@name" string.
	Name   string                   `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Source *Lockfile_Package_Source `protobuf:"bytes,2,opt,name=source,proto3" json:"source,omitempty"`
	// Deps is a list of **direct** dependencies of this package.
	//
	// Each entry is a "@name" string, referencing one of the packages mentioned
	// in the `packages` section of the lockfile. Cycles are possible. Entries
	// in the list are sorted alphabetically.
	Deps []string `protobuf:"bytes,3,rep,name=deps,proto3" json:"deps,omitempty"`
	// Minimal required lucicfg version, matching pkg.declare(lucicfg=...) arg.
	Lucicfg string `protobuf:"bytes,4,opt,name=lucicfg,proto3" json:"lucicfg,omitempty"`
	// Resources is a list of resource patterns declared via pkg.resources(...).
	//
	// Sorted alphabetically.
	Resources []string `protobuf:"bytes,5,rep,name=resources,proto3" json:"resources,omitempty"`
	// Entrypoints is a list of entry points declared via pkg.entrypoint(...).
	//
	// Present only for the main package. Sorted alphabetically.
	Entrypoints   []string `protobuf:"bytes,6,rep,name=entrypoints,proto3" json:"entrypoints,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Lockfile_Package) Reset() {
	*x = Lockfile_Package{}
	mi := &file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Lockfile_Package) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Lockfile_Package) ProtoMessage() {}

func (x *Lockfile_Package) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Lockfile_Package.ProtoReflect.Descriptor instead.
func (*Lockfile_Package) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_rawDescGZIP(), []int{0, 0}
}

func (x *Lockfile_Package) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *Lockfile_Package) GetSource() *Lockfile_Package_Source {
	if x != nil {
		return x.Source
	}
	return nil
}

func (x *Lockfile_Package) GetDeps() []string {
	if x != nil {
		return x.Deps
	}
	return nil
}

func (x *Lockfile_Package) GetLucicfg() string {
	if x != nil {
		return x.Lucicfg
	}
	return ""
}

func (x *Lockfile_Package) GetResources() []string {
	if x != nil {
		return x.Resources
	}
	return nil
}

func (x *Lockfile_Package) GetEntrypoints() []string {
	if x != nil {
		return x.Entrypoints
	}
	return nil
}

// Source is where to look for this package source code.
//
// Unset for the main package (since the location of the lockfile defines
// where its sources are).
type Lockfile_Package_Source struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Repo specifies a git repository and its ref with the package code.
	//
	// It has the form "https://<host>.googlesource.com/<repo>/+/<ref>" to
	// make it friendly to code-search it and to copy-paste it.
	//
	// If empty, then the package is in the same repository as the lockfile.
	Repo string `protobuf:"bytes,1,opt,name=repo,proto3" json:"repo,omitempty"`
	// Revision is the git revision containing the package source.
	//
	// It is some git commit SHA (not a ref or a tag).
	//
	// Empty for packages that are in the same repository as the lockfile.
	Revision string `protobuf:"bytes,2,opt,name=revision,proto3" json:"revision,omitempty"`
	// Path is where to find the package source within the repository.
	//
	// If `repo` is empty, this is a slash-separated path relative to the
	// lockfile path, always starting with "./" or "../". Otherwise it is a
	// slash-separated path relative to the corresponding repository root (and
	// it never starts with "./" or "../", though it can be "." if the package
	// is at the root of the repository).
	//
	// When loading relative paths, the interpreter should verify they do not
	// go outside of repository directory that contains the lockfile.
	Path          string `protobuf:"bytes,3,opt,name=path,proto3" json:"path,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Lockfile_Package_Source) Reset() {
	*x = Lockfile_Package_Source{}
	mi := &file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Lockfile_Package_Source) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Lockfile_Package_Source) ProtoMessage() {}

func (x *Lockfile_Package_Source) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Lockfile_Package_Source.ProtoReflect.Descriptor instead.
func (*Lockfile_Package_Source) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_rawDescGZIP(), []int{0, 0, 0}
}

func (x *Lockfile_Package_Source) GetRepo() string {
	if x != nil {
		return x.Repo
	}
	return ""
}

func (x *Lockfile_Package_Source) GetRevision() string {
	if x != nil {
		return x.Revision
	}
	return ""
}

func (x *Lockfile_Package_Source) GetPath() string {
	if x != nil {
		return x.Path
	}
	return ""
}

var File_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_rawDesc = string([]byte{
	0x0a, 0x36, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x63, 0x66, 0x67, 0x2f, 0x6c,
	0x6f, 0x63, 0x6b, 0x66, 0x69, 0x6c, 0x65, 0x70, 0x62, 0x2f, 0x6c, 0x6f, 0x63, 0x6b, 0x66, 0x69,
	0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x07, 0x6c, 0x75, 0x63, 0x69, 0x63, 0x66,
	0x67, 0x1a, 0x2f, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f,
	0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x2f, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x6f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x22, 0xf7, 0x02, 0x0a, 0x08, 0x4c, 0x6f, 0x63, 0x6b, 0x66, 0x69, 0x6c, 0x65, 0x12,
	0x1e, 0x0a, 0x07, 0x6c, 0x75, 0x63, 0x69, 0x63, 0x66, 0x67, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09,
	0x42, 0x04, 0xb0, 0xfe, 0x23, 0x01, 0x52, 0x07, 0x6c, 0x75, 0x63, 0x69, 0x63, 0x66, 0x67, 0x12,
	0x35, 0x0a, 0x08, 0x70, 0x61, 0x63, 0x6b, 0x61, 0x67, 0x65, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28,
	0x0b, 0x32, 0x19, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x63, 0x66, 0x67, 0x2e, 0x4c, 0x6f, 0x63, 0x6b,
	0x66, 0x69, 0x6c, 0x65, 0x2e, 0x50, 0x61, 0x63, 0x6b, 0x61, 0x67, 0x65, 0x52, 0x08, 0x70, 0x61,
	0x63, 0x6b, 0x61, 0x67, 0x65, 0x73, 0x1a, 0x93, 0x02, 0x0a, 0x07, 0x50, 0x61, 0x63, 0x6b, 0x61,
	0x67, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x38, 0x0a, 0x06, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x20, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x63, 0x66, 0x67,
	0x2e, 0x4c, 0x6f, 0x63, 0x6b, 0x66, 0x69, 0x6c, 0x65, 0x2e, 0x50, 0x61, 0x63, 0x6b, 0x61, 0x67,
	0x65, 0x2e, 0x53, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x52, 0x06, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65,
	0x12, 0x12, 0x0a, 0x04, 0x64, 0x65, 0x70, 0x73, 0x18, 0x03, 0x20, 0x03, 0x28, 0x09, 0x52, 0x04,
	0x64, 0x65, 0x70, 0x73, 0x12, 0x18, 0x0a, 0x07, 0x6c, 0x75, 0x63, 0x69, 0x63, 0x66, 0x67, 0x18,
	0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6c, 0x75, 0x63, 0x69, 0x63, 0x66, 0x67, 0x12, 0x1c,
	0x0a, 0x09, 0x72, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x73, 0x18, 0x05, 0x20, 0x03, 0x28,
	0x09, 0x52, 0x09, 0x72, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x73, 0x12, 0x20, 0x0a, 0x0b,
	0x65, 0x6e, 0x74, 0x72, 0x79, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x73, 0x18, 0x06, 0x20, 0x03, 0x28,
	0x09, 0x52, 0x0b, 0x65, 0x6e, 0x74, 0x72, 0x79, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x73, 0x1a, 0x4c,
	0x0a, 0x06, 0x53, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x12, 0x12, 0x0a, 0x04, 0x72, 0x65, 0x70, 0x6f,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x72, 0x65, 0x70, 0x6f, 0x12, 0x1a, 0x0a, 0x08,
	0x72, 0x65, 0x76, 0x69, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08,
	0x72, 0x65, 0x76, 0x69, 0x73, 0x69, 0x6f, 0x6e, 0x12, 0x12, 0x0a, 0x04, 0x70, 0x61, 0x74, 0x68,
	0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x70, 0x61, 0x74, 0x68, 0x42, 0x29, 0x5a, 0x27,
	0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f,
	0x6c, 0x75, 0x63, 0x69, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x63, 0x66, 0x67, 0x2f, 0x6c, 0x6f, 0x63,
	0x6b, 0x66, 0x69, 0x6c, 0x65, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_rawDescData []byte
)

func file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_rawDesc), len(file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_rawDescData
}

var file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_goTypes = []any{
	(*Lockfile)(nil),                // 0: lucicfg.Lockfile
	(*Lockfile_Package)(nil),        // 1: lucicfg.Lockfile.Package
	(*Lockfile_Package_Source)(nil), // 2: lucicfg.Lockfile.Package.Source
}
var file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_depIdxs = []int32{
	1, // 0: lucicfg.Lockfile.packages:type_name -> lucicfg.Lockfile.Package
	2, // 1: lucicfg.Lockfile.Package.source:type_name -> lucicfg.Lockfile.Package.Source
	2, // [2:2] is the sub-list for method output_type
	2, // [2:2] is the sub-list for method input_type
	2, // [2:2] is the sub-list for extension type_name
	2, // [2:2] is the sub-list for extension extendee
	0, // [0:2] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_init() }
func file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_init() {
	if File_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_rawDesc), len(file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto = out.File
	file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_goTypes = nil
	file_go_chromium_org_luci_lucicfg_lockfilepb_lockfile_proto_depIdxs = nil
}
