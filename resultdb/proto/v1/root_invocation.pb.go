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
// source: go.chromium.org/luci/resultdb/proto/v1/root_invocation.proto

package resultpb

import (
	_ "google.golang.org/genproto/googleapis/api/annotations"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	structpb "google.golang.org/protobuf/types/known/structpb"
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

type RootInvocation_State int32

const (
	// The default value. This value is used if the state is omitted.
	RootInvocation_STATE_UNSPECIFIED RootInvocation_State = 0
	// The root invocation is mutable.
	RootInvocation_ACTIVE RootInvocation_State = 1
	// The root invocation is in the process of moving to the FINALIZED state.
	// This will happen automatically soon after all of its directly or
	// indirectly included invocations become inactive.
	//
	// In this state, the root invocation record itself is immutable, but its
	// contained work units may still be mutable.
	RootInvocation_FINALIZING RootInvocation_State = 2
	// The invocation is immutable and no longer accepts new results
	// directly or indirectly.
	RootInvocation_FINALIZED RootInvocation_State = 3
)

// Enum value maps for RootInvocation_State.
var (
	RootInvocation_State_name = map[int32]string{
		0: "STATE_UNSPECIFIED",
		1: "ACTIVE",
		2: "FINALIZING",
		3: "FINALIZED",
	}
	RootInvocation_State_value = map[string]int32{
		"STATE_UNSPECIFIED": 0,
		"ACTIVE":            1,
		"FINALIZING":        2,
		"FINALIZED":         3,
	}
)

func (x RootInvocation_State) Enum() *RootInvocation_State {
	p := new(RootInvocation_State)
	*p = x
	return p
}

func (x RootInvocation_State) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (RootInvocation_State) Descriptor() protoreflect.EnumDescriptor {
	return file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_enumTypes[0].Descriptor()
}

func (RootInvocation_State) Type() protoreflect.EnumType {
	return &file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_enumTypes[0]
}

func (x RootInvocation_State) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use RootInvocation_State.Descriptor instead.
func (RootInvocation_State) EnumDescriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_rawDescGZIP(), []int{0, 0}
}

// A top-level container of test results.
type RootInvocation struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The resource name of this root invocation.
	// Format: `rootInvocations/{ROOT_INVOCATION_ID}`
	// See also https://aip.dev/122.
	//
	// Output only.
	Name string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	// The root invocation ID.
	//
	// Output only.
	RootInvocationId string `protobuf:"bytes,2,opt,name=root_invocation_id,json=rootInvocationId,proto3" json:"root_invocation_id,omitempty"`
	// Current state of the root invocation.
	//
	// At creation time, this can be set to ACTIVE or FINALIZING (if all fields
	// are known at creation time). When updating or via the FinalizeRootInvocation
	// RPC, the state can also be updated from ACTIVE to FINALIZING.
	//
	// In all other cases, this field should be treated as output only. ResultDB
	// will automatically transition the invocation to FINALIZING when the provided
	// `deadline` expires (if the invocation is not already in FINALIZING state).
	// FINALIZING invocations will transition onward to FINALIZED when all included
	// work units are FINALIZED.
	State RootInvocation_State `protobuf:"varint,3,opt,name=state,proto3,enum=luci.resultdb.v1.RootInvocation_State" json:"state,omitempty"`
	// The realm of the root invocation. This controls the ACLs that apply to the
	// root invocation and its contents.
	//
	// For example, 'chromium:try'.
	//
	// See go/luci-authorization for more information.
	Realm string `protobuf:"bytes,4,opt,name=realm,proto3" json:"realm,omitempty"`
	// When the invocation was created.
	// Output only.
	CreateTime *timestamppb.Timestamp `protobuf:"bytes,5,opt,name=create_time,json=createTime,proto3" json:"create_time,omitempty"`
	// LUCI identity (e.g. "user:<email>") who created the invocation.
	// Typically, a LUCI service account (e.g.
	// "user:cr-buildbucket@appspot.gserviceaccount.com"), but can also be a user
	// (e.g. "user:johndoe@example.com").
	//
	// Output only.
	Creator string `protobuf:"bytes,6,opt,name=creator,proto3" json:"creator,omitempty"`
	// When the invocation started to finalize, i.e. transitioned to FINALIZING
	// state. This means the invocation is immutable but directly or indirectly
	// included invocations may not be.
	//
	// Output only.
	FinalizeStartTime *timestamppb.Timestamp `protobuf:"bytes,7,opt,name=finalize_start_time,json=finalizeStartTime,proto3" json:"finalize_start_time,omitempty"`
	// When the invocation was finalized, i.e. transitioned to FINALIZED state.
	// If this field is set, implies that the invocation is finalized. This
	// means the invocation and directly or indirectly included invocations
	// are immutable.
	//
	// Output only.
	FinalizeTime *timestamppb.Timestamp `protobuf:"bytes,8,opt,name=finalize_time,json=finalizeTime,proto3" json:"finalize_time,omitempty"`
	// Timestamp when the invocation will be forcefully finalized.
	// Can be extended with UpdateRootInvocation until finalized.
	Deadline *timestamppb.Timestamp `protobuf:"bytes,9,opt,name=deadline,proto3" json:"deadline,omitempty"`
	// Full name of the resource that produced results in this root invocation.
	// See also https://aip.dev/122#full-resource-names
	// Typical examples:
	// - Swarming task: "//chromium-swarm.appspot.com/tasks/deadbeef"
	// - Buildbucket build: "//cr-buildbucket.appspot.com/builds/1234567890".
	//
	// Total length limited to 2000 bytes. Resource names must be in Unicode
	// normalization form C.
	ProducerResource string `protobuf:"bytes,10,opt,name=producer_resource,json=producerResource,proto3" json:"producer_resource,omitempty"`
	// The code sources which were tested by this root invocation.
	// This is used to index test results for test history, and for
	// related analyses (e.g. culprit analysis / changepoint analyses).
	Sources *Sources `protobuf:"bytes,11,opt,name=sources,proto3" json:"sources,omitempty"`
	// Whether the code sources specified by sources are final (immutable).
	//
	// To facilitate rapid export of invocations inheriting sources from this
	// root invocation, this property should be set to true as soon as possible
	// after the root invocation's sources are fixed. In most cases, clients
	// will want to set this property to true at the same time as they set
	// sources.
	//
	// This field is client owned. Consistent with https://google.aip.dev/129,
	// it will not be forced to true when the invocation starts to finalize, even
	// if its effective value will always be true at that point.
	SourcesFinal bool `protobuf:"varint,12,opt,name=sources_final,json=sourcesFinal,proto3" json:"sources_final,omitempty"`
	// Root invocation-level string key-value pairs.
	// A key can be repeated.
	//
	// Total size (as measured by proto.Size()) must be <= 16 KB.
	Tags []*StringPair `protobuf:"bytes,13,rep,name=tags,proto3" json:"tags,omitempty"`
	// Arbitrary JSON object that contains structured, domain-specific properties
	// of the root invocation.
	//
	// The value must contain a field "@type" which is a URL/resource name that
	// uniquely identifies the type of the source protocol buffer message that
	// defines the schema of these properties. This string must contain at least
	// one "/" character. The last segment of the URL's path must represent the
	// fully qualified name of the type (e.g. foo.com/x/some.package.MyMessage).
	// See google.protobuf.Any for more information.
	//
	// N.B. We do not use google.protobuf.Any here to remove a requirement for
	// ResultDB to know the schema of customer-defined protos. We do however use
	// a format equivalent to google.protobuf.Any's JSON representation.
	//
	// The serialized size must be <= 16 KB.
	Properties *structpb.Struct `protobuf:"bytes,14,opt,name=properties,proto3" json:"properties,omitempty"`
	// The test baseline that this root invocation should contribute to.
	//
	// This is a user-specified identifier. Typically, this identifier is generated
	// from the name of the source that generated the test result, such as the
	// builder name for Chromium. For example, `try:linux-rel`.
	//
	// The supported syntax for a baseline identifier is
	// ^[a-z0-9\-_.]{1,100}:[a-zA-Z0-9\-_.\(\) ]{1,128}$. This syntax was selected
	// to allow <buildbucket bucket name>:<buildbucket builder name> as a valid
	// baseline ID.
	//
	// Baselines are used to identify new tests; subtracting from the tests in the
	// root invocation the set of test variants in the baseline yields the new
	// tests run in the invocation. Those tests can then be e.g. subject to additional
	// presubmit checks, such as to validate they are not flaky.
	BaselineId    string `protobuf:"bytes,15,opt,name=baseline_id,json=baselineId,proto3" json:"baseline_id,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *RootInvocation) Reset() {
	*x = RootInvocation{}
	mi := &file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *RootInvocation) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RootInvocation) ProtoMessage() {}

func (x *RootInvocation) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RootInvocation.ProtoReflect.Descriptor instead.
func (*RootInvocation) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_rawDescGZIP(), []int{0}
}

func (x *RootInvocation) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *RootInvocation) GetRootInvocationId() string {
	if x != nil {
		return x.RootInvocationId
	}
	return ""
}

func (x *RootInvocation) GetState() RootInvocation_State {
	if x != nil {
		return x.State
	}
	return RootInvocation_STATE_UNSPECIFIED
}

func (x *RootInvocation) GetRealm() string {
	if x != nil {
		return x.Realm
	}
	return ""
}

func (x *RootInvocation) GetCreateTime() *timestamppb.Timestamp {
	if x != nil {
		return x.CreateTime
	}
	return nil
}

func (x *RootInvocation) GetCreator() string {
	if x != nil {
		return x.Creator
	}
	return ""
}

func (x *RootInvocation) GetFinalizeStartTime() *timestamppb.Timestamp {
	if x != nil {
		return x.FinalizeStartTime
	}
	return nil
}

func (x *RootInvocation) GetFinalizeTime() *timestamppb.Timestamp {
	if x != nil {
		return x.FinalizeTime
	}
	return nil
}

func (x *RootInvocation) GetDeadline() *timestamppb.Timestamp {
	if x != nil {
		return x.Deadline
	}
	return nil
}

func (x *RootInvocation) GetProducerResource() string {
	if x != nil {
		return x.ProducerResource
	}
	return ""
}

func (x *RootInvocation) GetSources() *Sources {
	if x != nil {
		return x.Sources
	}
	return nil
}

func (x *RootInvocation) GetSourcesFinal() bool {
	if x != nil {
		return x.SourcesFinal
	}
	return false
}

func (x *RootInvocation) GetTags() []*StringPair {
	if x != nil {
		return x.Tags
	}
	return nil
}

func (x *RootInvocation) GetProperties() *structpb.Struct {
	if x != nil {
		return x.Properties
	}
	return nil
}

func (x *RootInvocation) GetBaselineId() string {
	if x != nil {
		return x.BaselineId
	}
	return ""
}

var File_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_rawDesc = string([]byte{
	0x0a, 0x3c, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2f,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x76, 0x31, 0x2f, 0x72, 0x6f, 0x6f, 0x74, 0x5f, 0x69, 0x6e,
	0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x10,
	0x6c, 0x75, 0x63, 0x69, 0x2e, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2e, 0x76, 0x31,
	0x1a, 0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x66, 0x69, 0x65,
	0x6c, 0x64, 0x5f, 0x62, 0x65, 0x68, 0x61, 0x76, 0x69, 0x6f, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x1a, 0x1c, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62,
	0x75, 0x66, 0x2f, 0x73, 0x74, 0x72, 0x75, 0x63, 0x74, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a,
	0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66,
	0x2f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x1a, 0x33, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2f,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x76, 0x31, 0x2f, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0xd2, 0x06, 0x0a, 0x0e, 0x52, 0x6f, 0x6f, 0x74, 0x49, 0x6e,
	0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x1a, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x42, 0x06, 0xe0, 0x41, 0x03, 0xe0, 0x41, 0x05, 0x52, 0x04,
	0x6e, 0x61, 0x6d, 0x65, 0x12, 0x34, 0x0a, 0x12, 0x72, 0x6f, 0x6f, 0x74, 0x5f, 0x69, 0x6e, 0x76,
	0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09,
	0x42, 0x06, 0xe0, 0x41, 0x03, 0xe0, 0x41, 0x05, 0x52, 0x10, 0x72, 0x6f, 0x6f, 0x74, 0x49, 0x6e,
	0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x49, 0x64, 0x12, 0x3c, 0x0a, 0x05, 0x73, 0x74,
	0x61, 0x74, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x26, 0x2e, 0x6c, 0x75, 0x63, 0x69,
	0x2e, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2e, 0x76, 0x31, 0x2e, 0x52, 0x6f, 0x6f,
	0x74, 0x49, 0x6e, 0x76, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2e, 0x53, 0x74, 0x61, 0x74,
	0x65, 0x52, 0x05, 0x73, 0x74, 0x61, 0x74, 0x65, 0x12, 0x1c, 0x0a, 0x05, 0x72, 0x65, 0x61, 0x6c,
	0x6d, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x42, 0x06, 0xe0, 0x41, 0x03, 0xe0, 0x41, 0x05, 0x52,
	0x05, 0x72, 0x65, 0x61, 0x6c, 0x6d, 0x12, 0x43, 0x0a, 0x0b, 0x63, 0x72, 0x65, 0x61, 0x74, 0x65,
	0x5f, 0x74, 0x69, 0x6d, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f,
	0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69,
	0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x42, 0x06, 0xe0, 0x41, 0x03, 0xe0, 0x41, 0x05, 0x52,
	0x0a, 0x63, 0x72, 0x65, 0x61, 0x74, 0x65, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x20, 0x0a, 0x07, 0x63,
	0x72, 0x65, 0x61, 0x74, 0x6f, 0x72, 0x18, 0x06, 0x20, 0x01, 0x28, 0x09, 0x42, 0x06, 0xe0, 0x41,
	0x03, 0xe0, 0x41, 0x05, 0x52, 0x07, 0x63, 0x72, 0x65, 0x61, 0x74, 0x6f, 0x72, 0x12, 0x4f, 0x0a,
	0x13, 0x66, 0x69, 0x6e, 0x61, 0x6c, 0x69, 0x7a, 0x65, 0x5f, 0x73, 0x74, 0x61, 0x72, 0x74, 0x5f,
	0x74, 0x69, 0x6d, 0x65, 0x18, 0x07, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f,
	0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d,
	0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x42, 0x03, 0xe0, 0x41, 0x03, 0x52, 0x11, 0x66, 0x69, 0x6e,
	0x61, 0x6c, 0x69, 0x7a, 0x65, 0x53, 0x74, 0x61, 0x72, 0x74, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x44,
	0x0a, 0x0d, 0x66, 0x69, 0x6e, 0x61, 0x6c, 0x69, 0x7a, 0x65, 0x5f, 0x74, 0x69, 0x6d, 0x65, 0x18,
	0x08, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d,
	0x70, 0x42, 0x03, 0xe0, 0x41, 0x03, 0x52, 0x0c, 0x66, 0x69, 0x6e, 0x61, 0x6c, 0x69, 0x7a, 0x65,
	0x54, 0x69, 0x6d, 0x65, 0x12, 0x36, 0x0a, 0x08, 0x64, 0x65, 0x61, 0x64, 0x6c, 0x69, 0x6e, 0x65,
	0x18, 0x09, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61,
	0x6d, 0x70, 0x52, 0x08, 0x64, 0x65, 0x61, 0x64, 0x6c, 0x69, 0x6e, 0x65, 0x12, 0x2b, 0x0a, 0x11,
	0x70, 0x72, 0x6f, 0x64, 0x75, 0x63, 0x65, 0x72, 0x5f, 0x72, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63,
	0x65, 0x18, 0x0a, 0x20, 0x01, 0x28, 0x09, 0x52, 0x10, 0x70, 0x72, 0x6f, 0x64, 0x75, 0x63, 0x65,
	0x72, 0x52, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x12, 0x33, 0x0a, 0x07, 0x73, 0x6f, 0x75,
	0x72, 0x63, 0x65, 0x73, 0x18, 0x0b, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x6c, 0x75, 0x63,
	0x69, 0x2e, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2e, 0x76, 0x31, 0x2e, 0x53, 0x6f,
	0x75, 0x72, 0x63, 0x65, 0x73, 0x52, 0x07, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x73, 0x12, 0x23,
	0x0a, 0x0d, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x73, 0x5f, 0x66, 0x69, 0x6e, 0x61, 0x6c, 0x18,
	0x0c, 0x20, 0x01, 0x28, 0x08, 0x52, 0x0c, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x73, 0x46, 0x69,
	0x6e, 0x61, 0x6c, 0x12, 0x30, 0x0a, 0x04, 0x74, 0x61, 0x67, 0x73, 0x18, 0x0d, 0x20, 0x03, 0x28,
	0x0b, 0x32, 0x1c, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64,
	0x62, 0x2e, 0x76, 0x31, 0x2e, 0x53, 0x74, 0x72, 0x69, 0x6e, 0x67, 0x50, 0x61, 0x69, 0x72, 0x52,
	0x04, 0x74, 0x61, 0x67, 0x73, 0x12, 0x37, 0x0a, 0x0a, 0x70, 0x72, 0x6f, 0x70, 0x65, 0x72, 0x74,
	0x69, 0x65, 0x73, 0x18, 0x0e, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x17, 0x2e, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x53, 0x74, 0x72, 0x75,
	0x63, 0x74, 0x52, 0x0a, 0x70, 0x72, 0x6f, 0x70, 0x65, 0x72, 0x74, 0x69, 0x65, 0x73, 0x12, 0x1f,
	0x0a, 0x0b, 0x62, 0x61, 0x73, 0x65, 0x6c, 0x69, 0x6e, 0x65, 0x5f, 0x69, 0x64, 0x18, 0x0f, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x0a, 0x62, 0x61, 0x73, 0x65, 0x6c, 0x69, 0x6e, 0x65, 0x49, 0x64, 0x22,
	0x49, 0x0a, 0x05, 0x53, 0x74, 0x61, 0x74, 0x65, 0x12, 0x15, 0x0a, 0x11, 0x53, 0x54, 0x41, 0x54,
	0x45, 0x5f, 0x55, 0x4e, 0x53, 0x50, 0x45, 0x43, 0x49, 0x46, 0x49, 0x45, 0x44, 0x10, 0x00, 0x12,
	0x0a, 0x0a, 0x06, 0x41, 0x43, 0x54, 0x49, 0x56, 0x45, 0x10, 0x01, 0x12, 0x0e, 0x0a, 0x0a, 0x46,
	0x49, 0x4e, 0x41, 0x4c, 0x49, 0x5a, 0x49, 0x4e, 0x47, 0x10, 0x02, 0x12, 0x0d, 0x0a, 0x09, 0x46,
	0x49, 0x4e, 0x41, 0x4c, 0x49, 0x5a, 0x45, 0x44, 0x10, 0x03, 0x42, 0x50, 0x0a, 0x1b, 0x63, 0x6f,
	0x6d, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x72, 0x65,
	0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2e, 0x76, 0x31, 0x50, 0x01, 0x5a, 0x2f, 0x67, 0x6f, 0x2e,
	0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63,
	0x69, 0x2f, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x64, 0x62, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x2f, 0x76, 0x31, 0x3b, 0x72, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_rawDescData []byte
)

func file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_rawDesc), len(file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_rawDescData
}

var file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_goTypes = []any{
	(RootInvocation_State)(0),     // 0: luci.resultdb.v1.RootInvocation.State
	(*RootInvocation)(nil),        // 1: luci.resultdb.v1.RootInvocation
	(*timestamppb.Timestamp)(nil), // 2: google.protobuf.Timestamp
	(*Sources)(nil),               // 3: luci.resultdb.v1.Sources
	(*StringPair)(nil),            // 4: luci.resultdb.v1.StringPair
	(*structpb.Struct)(nil),       // 5: google.protobuf.Struct
}
var file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_depIdxs = []int32{
	0, // 0: luci.resultdb.v1.RootInvocation.state:type_name -> luci.resultdb.v1.RootInvocation.State
	2, // 1: luci.resultdb.v1.RootInvocation.create_time:type_name -> google.protobuf.Timestamp
	2, // 2: luci.resultdb.v1.RootInvocation.finalize_start_time:type_name -> google.protobuf.Timestamp
	2, // 3: luci.resultdb.v1.RootInvocation.finalize_time:type_name -> google.protobuf.Timestamp
	2, // 4: luci.resultdb.v1.RootInvocation.deadline:type_name -> google.protobuf.Timestamp
	3, // 5: luci.resultdb.v1.RootInvocation.sources:type_name -> luci.resultdb.v1.Sources
	4, // 6: luci.resultdb.v1.RootInvocation.tags:type_name -> luci.resultdb.v1.StringPair
	5, // 7: luci.resultdb.v1.RootInvocation.properties:type_name -> google.protobuf.Struct
	8, // [8:8] is the sub-list for method output_type
	8, // [8:8] is the sub-list for method input_type
	8, // [8:8] is the sub-list for extension type_name
	8, // [8:8] is the sub-list for extension extendee
	0, // [0:8] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_init() }
func file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_init() {
	if File_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto != nil {
		return
	}
	file_go_chromium_org_luci_resultdb_proto_v1_common_proto_init()
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_rawDesc), len(file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_rawDesc)),
			NumEnums:      1,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_depIdxs,
		EnumInfos:         file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_enumTypes,
		MessageInfos:      file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto = out.File
	file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_goTypes = nil
	file_go_chromium_org_luci_resultdb_proto_v1_root_invocation_proto_depIdxs = nil
}
