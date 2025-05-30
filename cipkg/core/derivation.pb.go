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
// source: go.chromium.org/luci/cipkg/core/derivation.proto

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

// Derivation is the atomic step transformed from different types of actions. It
// should contain all information used during the execution in its definition.
// NOTE: ${out} enviroment variable is not part of the derivation. We can't
// determine the output directory before we have a deterministic derivation so
// it has to be excluded from derivation to avoid self-reference.
type Derivation struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Name is the name of the derivation's output and serves ONLY as an
	// indicator of its output.
	// The name shouldn't include version of the package and should represents
	// its content (e.g. cpython3, curl, ninja), NOT the action taken place
	// (e.g. build_cpython3, build_curl, build_ninja).
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// Platform is a textual description of the platform in which this Derivation
	// should be performed, and serves ONLY as an indicator of implicit
	// environmental contamination of the output of the derivation.
	// Actions should relies on cipkg/base/actions.ActionProcessor to populate
	// this field appropriately.
	Platform string `protobuf:"bytes,2,opt,name=platform,proto3" json:"platform,omitempty"`
	// Args are the `argv` vector of the derivation when executed.
	Args []string `protobuf:"bytes,3,rep,name=args,proto3" json:"args,omitempty"`
	// Env includes all the environment variables for the execution isolated from
	// host.
	// NOTE: ${out} is not included here but will be presented in the environment
	// during execution.
	Env []string `protobuf:"bytes,4,rep,name=env,proto3" json:"env,omitempty"`
	// Inputs are ids of all packages referred by this derivation.
	// It depends on the package manager to ensure packages represented by the
	// derivation IDs will be available before execution.
	// Ideally derivation should only be able to access derivations listed in the
	// inputs. Executor may lock down the runtime environment to prevent the
	// derivation from accessing any resource other than those listed in the
	// future.
	Inputs []string `protobuf:"bytes,5,rep,name=inputs,proto3" json:"inputs,omitempty"`
	// fixed_output, if set, represents the content of the output. ID will be
	// generated based on fixed_output exclusively.
	// WARNING: Using fixed_output means shifting away the responsibility for
	// detecting any change from derivation. This should be rarely touched and
	// most of its use cases have a builtin implementation to take care of the
	// generated fixed_output value. Any use of it outside the builtin modules
	// are strongly discouraged. YOU HAVE BEEN WARNED.
	FixedOutput   string `protobuf:"bytes,6,opt,name=fixed_output,json=fixedOutput,proto3" json:"fixed_output,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Derivation) Reset() {
	*x = Derivation{}
	mi := &file_go_chromium_org_luci_cipkg_core_derivation_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Derivation) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Derivation) ProtoMessage() {}

func (x *Derivation) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_cipkg_core_derivation_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Derivation.ProtoReflect.Descriptor instead.
func (*Derivation) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_cipkg_core_derivation_proto_rawDescGZIP(), []int{0}
}

func (x *Derivation) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *Derivation) GetPlatform() string {
	if x != nil {
		return x.Platform
	}
	return ""
}

func (x *Derivation) GetArgs() []string {
	if x != nil {
		return x.Args
	}
	return nil
}

func (x *Derivation) GetEnv() []string {
	if x != nil {
		return x.Env
	}
	return nil
}

func (x *Derivation) GetInputs() []string {
	if x != nil {
		return x.Inputs
	}
	return nil
}

func (x *Derivation) GetFixedOutput() string {
	if x != nil {
		return x.FixedOutput
	}
	return ""
}

var File_go_chromium_org_luci_cipkg_core_derivation_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_cipkg_core_derivation_proto_rawDesc = string([]byte{
	0x0a, 0x30, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x63, 0x69, 0x70, 0x6b, 0x67, 0x2f, 0x63, 0x6f, 0x72,
	0x65, 0x2f, 0x64, 0x65, 0x72, 0x69, 0x76, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x22, 0x9d, 0x01, 0x0a, 0x0a, 0x44, 0x65, 0x72, 0x69, 0x76, 0x61, 0x74, 0x69, 0x6f,
	0x6e, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x1a, 0x0a, 0x08, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72,
	0x6d, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x70, 0x6c, 0x61, 0x74, 0x66, 0x6f, 0x72,
	0x6d, 0x12, 0x12, 0x0a, 0x04, 0x61, 0x72, 0x67, 0x73, 0x18, 0x03, 0x20, 0x03, 0x28, 0x09, 0x52,
	0x04, 0x61, 0x72, 0x67, 0x73, 0x12, 0x10, 0x0a, 0x03, 0x65, 0x6e, 0x76, 0x18, 0x04, 0x20, 0x03,
	0x28, 0x09, 0x52, 0x03, 0x65, 0x6e, 0x76, 0x12, 0x16, 0x0a, 0x06, 0x69, 0x6e, 0x70, 0x75, 0x74,
	0x73, 0x18, 0x05, 0x20, 0x03, 0x28, 0x09, 0x52, 0x06, 0x69, 0x6e, 0x70, 0x75, 0x74, 0x73, 0x12,
	0x21, 0x0a, 0x0c, 0x66, 0x69, 0x78, 0x65, 0x64, 0x5f, 0x6f, 0x75, 0x74, 0x70, 0x75, 0x74, 0x18,
	0x06, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b, 0x66, 0x69, 0x78, 0x65, 0x64, 0x4f, 0x75, 0x74, 0x70,
	0x75, 0x74, 0x42, 0x21, 0x5a, 0x1f, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75,
	0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x63, 0x69, 0x70, 0x6b, 0x67,
	0x2f, 0x63, 0x6f, 0x72, 0x65, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_cipkg_core_derivation_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_cipkg_core_derivation_proto_rawDescData []byte
)

func file_go_chromium_org_luci_cipkg_core_derivation_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_cipkg_core_derivation_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_cipkg_core_derivation_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_cipkg_core_derivation_proto_rawDesc), len(file_go_chromium_org_luci_cipkg_core_derivation_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_cipkg_core_derivation_proto_rawDescData
}

var file_go_chromium_org_luci_cipkg_core_derivation_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_go_chromium_org_luci_cipkg_core_derivation_proto_goTypes = []any{
	(*Derivation)(nil), // 0: Derivation
}
var file_go_chromium_org_luci_cipkg_core_derivation_proto_depIdxs = []int32{
	0, // [0:0] is the sub-list for method output_type
	0, // [0:0] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_cipkg_core_derivation_proto_init() }
func file_go_chromium_org_luci_cipkg_core_derivation_proto_init() {
	if File_go_chromium_org_luci_cipkg_core_derivation_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_cipkg_core_derivation_proto_rawDesc), len(file_go_chromium_org_luci_cipkg_core_derivation_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_cipkg_core_derivation_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_cipkg_core_derivation_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_cipkg_core_derivation_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_cipkg_core_derivation_proto = out.File
	file_go_chromium_org_luci_cipkg_core_derivation_proto_goTypes = nil
	file_go_chromium_org_luci_cipkg_core_derivation_proto_depIdxs = nil
}
