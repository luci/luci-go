// Copyright 2022 The LUCI Authors.
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
// source: go.chromium.org/luci/analysis/proto/analyzedtestvariant/predicate.proto

package atvpb

import (
	v1 "go.chromium.org/luci/analysis/proto/v1"
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

// Deprecated. Retained only for config compatibility.
// Can be deleted once chromium-m120 / chrome-m120 and all prior
// versions have had their LUCI configs deleted.
type Status int32

const (
	Status_STATUS_UNSPECIFIED      Status = 0
	Status_HAS_UNEXPECTED_RESULTS  Status = 5
	Status_FLAKY                   Status = 10
	Status_CONSISTENTLY_UNEXPECTED Status = 20
	Status_CONSISTENTLY_EXPECTED   Status = 30
	Status_NO_NEW_RESULTS          Status = 40
)

// Enum value maps for Status.
var (
	Status_name = map[int32]string{
		0:  "STATUS_UNSPECIFIED",
		5:  "HAS_UNEXPECTED_RESULTS",
		10: "FLAKY",
		20: "CONSISTENTLY_UNEXPECTED",
		30: "CONSISTENTLY_EXPECTED",
		40: "NO_NEW_RESULTS",
	}
	Status_value = map[string]int32{
		"STATUS_UNSPECIFIED":      0,
		"HAS_UNEXPECTED_RESULTS":  5,
		"FLAKY":                   10,
		"CONSISTENTLY_UNEXPECTED": 20,
		"CONSISTENTLY_EXPECTED":   30,
		"NO_NEW_RESULTS":          40,
	}
)

func (x Status) Enum() *Status {
	p := new(Status)
	*p = x
	return p
}

func (x Status) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Status) Descriptor() protoreflect.EnumDescriptor {
	return file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_enumTypes[0].Descriptor()
}

func (Status) Type() protoreflect.EnumType {
	return &file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_enumTypes[0]
}

func (x Status) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Status.Descriptor instead.
func (Status) EnumDescriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_rawDescGZIP(), []int{0}
}

// Deprecated. Retained only for config compatibility.
// Can be deleted once chromium-m120 / chrome-m120 and all prior
// versions have had their LUCI configs deleted.
type Predicate struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	TestIdRegexp  string                 `protobuf:"bytes,1,opt,name=test_id_regexp,json=testIdRegexp,proto3" json:"test_id_regexp,omitempty"`
	Variant       *v1.VariantPredicate   `protobuf:"bytes,2,opt,name=variant,proto3" json:"variant,omitempty"`
	Status        Status                 `protobuf:"varint,3,opt,name=status,proto3,enum=luci.analysis.analyzedtestvariant.Status" json:"status,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Predicate) Reset() {
	*x = Predicate{}
	mi := &file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Predicate) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Predicate) ProtoMessage() {}

func (x *Predicate) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Predicate.ProtoReflect.Descriptor instead.
func (*Predicate) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_rawDescGZIP(), []int{0}
}

func (x *Predicate) GetTestIdRegexp() string {
	if x != nil {
		return x.TestIdRegexp
	}
	return ""
}

func (x *Predicate) GetVariant() *v1.VariantPredicate {
	if x != nil {
		return x.Variant
	}
	return nil
}

func (x *Predicate) GetStatus() Status {
	if x != nil {
		return x.Status
	}
	return Status_STATUS_UNSPECIFIED
}

var File_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_rawDesc = string([]byte{
	0x0a, 0x47, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2f,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x7a, 0x65, 0x64, 0x74, 0x65,
	0x73, 0x74, 0x76, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x2f, 0x70, 0x72, 0x65, 0x64, 0x69, 0x63,
	0x61, 0x74, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x21, 0x6c, 0x75, 0x63, 0x69, 0x2e,
	0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2e, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x7a, 0x65,
	0x64, 0x74, 0x65, 0x73, 0x74, 0x76, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x1a, 0x36, 0x67, 0x6f,
	0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75,
	0x63, 0x69, 0x2f, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2f, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x2f, 0x76, 0x31, 0x2f, 0x70, 0x72, 0x65, 0x64, 0x69, 0x63, 0x61, 0x74, 0x65, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x22, 0xb2, 0x01, 0x0a, 0x09, 0x50, 0x72, 0x65, 0x64, 0x69, 0x63, 0x61,
	0x74, 0x65, 0x12, 0x24, 0x0a, 0x0e, 0x74, 0x65, 0x73, 0x74, 0x5f, 0x69, 0x64, 0x5f, 0x72, 0x65,
	0x67, 0x65, 0x78, 0x70, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0c, 0x74, 0x65, 0x73, 0x74,
	0x49, 0x64, 0x52, 0x65, 0x67, 0x65, 0x78, 0x70, 0x12, 0x3c, 0x0a, 0x07, 0x76, 0x61, 0x72, 0x69,
	0x61, 0x6e, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x22, 0x2e, 0x6c, 0x75, 0x63, 0x69,
	0x2e, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x56, 0x61, 0x72,
	0x69, 0x61, 0x6e, 0x74, 0x50, 0x72, 0x65, 0x64, 0x69, 0x63, 0x61, 0x74, 0x65, 0x52, 0x07, 0x76,
	0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x12, 0x41, 0x0a, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73,
	0x18, 0x03, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x29, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x61, 0x6e,
	0x61, 0x6c, 0x79, 0x73, 0x69, 0x73, 0x2e, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x7a, 0x65, 0x64, 0x74,
	0x65, 0x73, 0x74, 0x76, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x2e, 0x53, 0x74, 0x61, 0x74, 0x75,
	0x73, 0x52, 0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x2a, 0x93, 0x01, 0x0a, 0x06, 0x53, 0x74,
	0x61, 0x74, 0x75, 0x73, 0x12, 0x16, 0x0a, 0x12, 0x53, 0x54, 0x41, 0x54, 0x55, 0x53, 0x5f, 0x55,
	0x4e, 0x53, 0x50, 0x45, 0x43, 0x49, 0x46, 0x49, 0x45, 0x44, 0x10, 0x00, 0x12, 0x1a, 0x0a, 0x16,
	0x48, 0x41, 0x53, 0x5f, 0x55, 0x4e, 0x45, 0x58, 0x50, 0x45, 0x43, 0x54, 0x45, 0x44, 0x5f, 0x52,
	0x45, 0x53, 0x55, 0x4c, 0x54, 0x53, 0x10, 0x05, 0x12, 0x09, 0x0a, 0x05, 0x46, 0x4c, 0x41, 0x4b,
	0x59, 0x10, 0x0a, 0x12, 0x1b, 0x0a, 0x17, 0x43, 0x4f, 0x4e, 0x53, 0x49, 0x53, 0x54, 0x45, 0x4e,
	0x54, 0x4c, 0x59, 0x5f, 0x55, 0x4e, 0x45, 0x58, 0x50, 0x45, 0x43, 0x54, 0x45, 0x44, 0x10, 0x14,
	0x12, 0x19, 0x0a, 0x15, 0x43, 0x4f, 0x4e, 0x53, 0x49, 0x53, 0x54, 0x45, 0x4e, 0x54, 0x4c, 0x59,
	0x5f, 0x45, 0x58, 0x50, 0x45, 0x43, 0x54, 0x45, 0x44, 0x10, 0x1e, 0x12, 0x12, 0x0a, 0x0e, 0x4e,
	0x4f, 0x5f, 0x4e, 0x45, 0x57, 0x5f, 0x52, 0x45, 0x53, 0x55, 0x4c, 0x54, 0x53, 0x10, 0x28, 0x42,
	0x3f, 0x5a, 0x3d, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f,
	0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x73, 0x69, 0x73,
	0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x61, 0x6e, 0x61, 0x6c, 0x79, 0x7a, 0x65, 0x64, 0x74,
	0x65, 0x73, 0x74, 0x76, 0x61, 0x72, 0x69, 0x61, 0x6e, 0x74, 0x3b, 0x61, 0x74, 0x76, 0x70, 0x62,
	0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_rawDescData []byte
)

func file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_rawDesc), len(file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_rawDescData
}

var file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_goTypes = []any{
	(Status)(0),                 // 0: luci.analysis.analyzedtestvariant.Status
	(*Predicate)(nil),           // 1: luci.analysis.analyzedtestvariant.Predicate
	(*v1.VariantPredicate)(nil), // 2: luci.analysis.v1.VariantPredicate
}
var file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_depIdxs = []int32{
	2, // 0: luci.analysis.analyzedtestvariant.Predicate.variant:type_name -> luci.analysis.v1.VariantPredicate
	0, // 1: luci.analysis.analyzedtestvariant.Predicate.status:type_name -> luci.analysis.analyzedtestvariant.Status
	2, // [2:2] is the sub-list for method output_type
	2, // [2:2] is the sub-list for method input_type
	2, // [2:2] is the sub-list for extension type_name
	2, // [2:2] is the sub-list for extension extendee
	0, // [0:2] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_init() }
func file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_init() {
	if File_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_rawDesc), len(file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_rawDesc)),
			NumEnums:      1,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_depIdxs,
		EnumInfos:         file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_enumTypes,
		MessageInfos:      file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto = out.File
	file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_goTypes = nil
	file_go_chromium_org_luci_analysis_proto_analyzedtestvariant_predicate_proto_depIdxs = nil
}
