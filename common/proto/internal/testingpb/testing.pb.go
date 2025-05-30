// Copyright 2018 The Swarming Authors. All rights reserved.
// Use of this source code is governed by the Apache v2.0 license that can be
// found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.5
// 	protoc        v6.30.2
// source: go.chromium.org/luci/common/proto/internal/testingpb/testing.proto

package testingpb

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	fieldmaskpb "google.golang.org/protobuf/types/known/fieldmaskpb"
	structpb "google.golang.org/protobuf/types/known/structpb"
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

type Some struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	I             int64                  `protobuf:"varint,1,opt,name=i,proto3" json:"i,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Some) Reset() {
	*x = Some{}
	mi := &file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Some) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Some) ProtoMessage() {}

func (x *Some) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Some.ProtoReflect.Descriptor instead.
func (*Some) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_rawDescGZIP(), []int{0}
}

func (x *Some) GetI() int64 {
	if x != nil {
		return x.I
	}
	return 0
}

type Simple struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Id            int64                  `protobuf:"varint,1,opt,name=id,proto3" json:"id,omitempty"`
	Some          *Some                  `protobuf:"bytes,2,opt,name=some,proto3" json:"some,omitempty"`
	OtherSome     *Some                  `protobuf:"bytes,3,opt,name=other_some,json=otherSome,proto3" json:"other_some,omitempty"`
	OtherSomeJson *Some                  `protobuf:"bytes,4,opt,name=other_some_json,json=customJSON,proto3" json:"other_some_json,omitempty"`
	Fields        *fieldmaskpb.FieldMask `protobuf:"bytes,100,opt,name=fields,proto3" json:"fields,omitempty"`
	OtherFields   *fieldmaskpb.FieldMask `protobuf:"bytes,101,opt,name=other_fields,json=otherFields,proto3" json:"other_fields,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Simple) Reset() {
	*x = Simple{}
	mi := &file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Simple) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Simple) ProtoMessage() {}

func (x *Simple) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Simple.ProtoReflect.Descriptor instead.
func (*Simple) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_rawDescGZIP(), []int{1}
}

func (x *Simple) GetId() int64 {
	if x != nil {
		return x.Id
	}
	return 0
}

func (x *Simple) GetSome() *Some {
	if x != nil {
		return x.Some
	}
	return nil
}

func (x *Simple) GetOtherSome() *Some {
	if x != nil {
		return x.OtherSome
	}
	return nil
}

func (x *Simple) GetOtherSomeJson() *Some {
	if x != nil {
		return x.OtherSomeJson
	}
	return nil
}

func (x *Simple) GetFields() *fieldmaskpb.FieldMask {
	if x != nil {
		return x.Fields
	}
	return nil
}

func (x *Simple) GetOtherFields() *fieldmaskpb.FieldMask {
	if x != nil {
		return x.OtherFields
	}
	return nil
}

type Props struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Properties    *structpb.Struct       `protobuf:"bytes,6,opt,name=properties,proto3" json:"properties,omitempty"`
	Fields        *fieldmaskpb.FieldMask `protobuf:"bytes,100,opt,name=fields,proto3" json:"fields,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Props) Reset() {
	*x = Props{}
	mi := &file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Props) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Props) ProtoMessage() {}

func (x *Props) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Props.ProtoReflect.Descriptor instead.
func (*Props) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_rawDescGZIP(), []int{2}
}

func (x *Props) GetProperties() *structpb.Struct {
	if x != nil {
		return x.Properties
	}
	return nil
}

func (x *Props) GetFields() *fieldmaskpb.FieldMask {
	if x != nil {
		return x.Fields
	}
	return nil
}

type WithInner struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Msgs          []*WithInner_Inner     `protobuf:"bytes,1,rep,name=msgs,proto3" json:"msgs,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *WithInner) Reset() {
	*x = WithInner{}
	mi := &file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *WithInner) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*WithInner) ProtoMessage() {}

func (x *WithInner) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use WithInner.ProtoReflect.Descriptor instead.
func (*WithInner) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_rawDescGZIP(), []int{3}
}

func (x *WithInner) GetMsgs() []*WithInner_Inner {
	if x != nil {
		return x.Msgs
	}
	return nil
}

type Full struct {
	state          protoimpl.MessageState `protogen:"open.v1"`
	I32            int32                  `protobuf:"varint,1,opt,name=i32,proto3" json:"i32,omitempty"`
	I64            int64                  `protobuf:"varint,2,opt,name=i64,proto3" json:"i64,omitempty"`
	U32            uint32                 `protobuf:"varint,3,opt,name=u32,proto3" json:"u32,omitempty"`
	U64            uint64                 `protobuf:"varint,4,opt,name=u64,proto3" json:"u64,omitempty"`
	F32            float32                `protobuf:"fixed32,5,opt,name=f32,proto3" json:"f32,omitempty"`
	F64            float64                `protobuf:"fixed64,6,opt,name=f64,proto3" json:"f64,omitempty"`
	Boolean        bool                   `protobuf:"varint,7,opt,name=boolean,proto3" json:"boolean,omitempty"`
	Num            int32                  `protobuf:"varint,8,opt,name=num,proto3" json:"num,omitempty"`
	Nums           []int32                `protobuf:"varint,9,rep,packed,name=nums,proto3" json:"nums,omitempty"`
	Str            string                 `protobuf:"bytes,10,opt,name=str,proto3" json:"str,omitempty"`
	Strs           []string               `protobuf:"bytes,11,rep,name=strs,proto3" json:"strs,omitempty"`
	Msg            *Full                  `protobuf:"bytes,12,opt,name=msg,proto3" json:"msg,omitempty"`
	Msgs           []*Full                `protobuf:"bytes,13,rep,name=msgs,proto3" json:"msgs,omitempty"`
	MapStrNum      map[string]int32       `protobuf:"bytes,14,rep,name=map_str_num,json=mapStrNum,proto3" json:"map_str_num,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"varint,2,opt,name=value"`
	MapNumStr      map[int32]string       `protobuf:"bytes,15,rep,name=map_num_str,json=mapNumStr,proto3" json:"map_num_str,omitempty" protobuf_key:"varint,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	MapBoolStr     map[bool]string        `protobuf:"bytes,16,rep,name=map_bool_str,json=mapBoolStr,proto3" json:"map_bool_str,omitempty" protobuf_key:"varint,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	MapStrMsg      map[string]*Full       `protobuf:"bytes,17,rep,name=map_str_msg,json=mapStrMsg,proto3" json:"map_str_msg,omitempty" protobuf_key:"bytes,1,opt,name=key" protobuf_val:"bytes,2,opt,name=value"`
	JsonName       string                 `protobuf:"bytes,18,opt,name=json_name,json=jsonName,proto3" json:"json_name,omitempty"`
	JsonNameOption string                 `protobuf:"bytes,19,opt,name=json_name_option,json=another_json_name,proto3" json:"json_name_option,omitempty"`
	unknownFields  protoimpl.UnknownFields
	sizeCache      protoimpl.SizeCache
}

func (x *Full) Reset() {
	*x = Full{}
	mi := &file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_msgTypes[4]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Full) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Full) ProtoMessage() {}

func (x *Full) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_msgTypes[4]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Full.ProtoReflect.Descriptor instead.
func (*Full) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_rawDescGZIP(), []int{4}
}

func (x *Full) GetI32() int32 {
	if x != nil {
		return x.I32
	}
	return 0
}

func (x *Full) GetI64() int64 {
	if x != nil {
		return x.I64
	}
	return 0
}

func (x *Full) GetU32() uint32 {
	if x != nil {
		return x.U32
	}
	return 0
}

func (x *Full) GetU64() uint64 {
	if x != nil {
		return x.U64
	}
	return 0
}

func (x *Full) GetF32() float32 {
	if x != nil {
		return x.F32
	}
	return 0
}

func (x *Full) GetF64() float64 {
	if x != nil {
		return x.F64
	}
	return 0
}

func (x *Full) GetBoolean() bool {
	if x != nil {
		return x.Boolean
	}
	return false
}

func (x *Full) GetNum() int32 {
	if x != nil {
		return x.Num
	}
	return 0
}

func (x *Full) GetNums() []int32 {
	if x != nil {
		return x.Nums
	}
	return nil
}

func (x *Full) GetStr() string {
	if x != nil {
		return x.Str
	}
	return ""
}

func (x *Full) GetStrs() []string {
	if x != nil {
		return x.Strs
	}
	return nil
}

func (x *Full) GetMsg() *Full {
	if x != nil {
		return x.Msg
	}
	return nil
}

func (x *Full) GetMsgs() []*Full {
	if x != nil {
		return x.Msgs
	}
	return nil
}

func (x *Full) GetMapStrNum() map[string]int32 {
	if x != nil {
		return x.MapStrNum
	}
	return nil
}

func (x *Full) GetMapNumStr() map[int32]string {
	if x != nil {
		return x.MapNumStr
	}
	return nil
}

func (x *Full) GetMapBoolStr() map[bool]string {
	if x != nil {
		return x.MapBoolStr
	}
	return nil
}

func (x *Full) GetMapStrMsg() map[string]*Full {
	if x != nil {
		return x.MapStrMsg
	}
	return nil
}

func (x *Full) GetJsonName() string {
	if x != nil {
		return x.JsonName
	}
	return ""
}

func (x *Full) GetJsonNameOption() string {
	if x != nil {
		return x.JsonNameOption
	}
	return ""
}

type WithInner_Inner struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Types that are valid to be assigned to Msg:
	//
	//	*WithInner_Inner_Simple
	//	*WithInner_Inner_Props
	Msg           isWithInner_Inner_Msg `protobuf_oneof:"msg"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *WithInner_Inner) Reset() {
	*x = WithInner_Inner{}
	mi := &file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_msgTypes[5]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *WithInner_Inner) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*WithInner_Inner) ProtoMessage() {}

func (x *WithInner_Inner) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_msgTypes[5]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use WithInner_Inner.ProtoReflect.Descriptor instead.
func (*WithInner_Inner) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_rawDescGZIP(), []int{3, 0}
}

func (x *WithInner_Inner) GetMsg() isWithInner_Inner_Msg {
	if x != nil {
		return x.Msg
	}
	return nil
}

func (x *WithInner_Inner) GetSimple() *Simple {
	if x != nil {
		if x, ok := x.Msg.(*WithInner_Inner_Simple); ok {
			return x.Simple
		}
	}
	return nil
}

func (x *WithInner_Inner) GetProps() *Props {
	if x != nil {
		if x, ok := x.Msg.(*WithInner_Inner_Props); ok {
			return x.Props
		}
	}
	return nil
}

type isWithInner_Inner_Msg interface {
	isWithInner_Inner_Msg()
}

type WithInner_Inner_Simple struct {
	Simple *Simple `protobuf:"bytes,1,opt,name=simple,proto3,oneof"`
}

type WithInner_Inner_Props struct {
	Props *Props `protobuf:"bytes,2,opt,name=props,proto3,oneof"`
}

func (*WithInner_Inner_Simple) isWithInner_Inner_Msg() {}

func (*WithInner_Inner_Props) isWithInner_Inner_Msg() {}

var File_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_rawDesc = string([]byte{
	0x0a, 0x42, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x2f, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x74, 0x65, 0x73,
	0x74, 0x69, 0x6e, 0x67, 0x70, 0x62, 0x2f, 0x74, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x67, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x12, 0x10, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x74,
	0x65, 0x73, 0x74, 0x69, 0x6e, 0x67, 0x1a, 0x20, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x66, 0x69, 0x65, 0x6c, 0x64, 0x5f, 0x6d, 0x61,
	0x73, 0x6b, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1c, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x73, 0x74, 0x72, 0x75, 0x63, 0x74,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x14, 0x0a, 0x04, 0x53, 0x6f, 0x6d, 0x65, 0x12, 0x0c,
	0x0a, 0x01, 0x69, 0x18, 0x01, 0x20, 0x01, 0x28, 0x03, 0x52, 0x01, 0x69, 0x22, 0xab, 0x02, 0x0a,
	0x06, 0x53, 0x69, 0x6d, 0x70, 0x6c, 0x65, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x03, 0x52, 0x02, 0x69, 0x64, 0x12, 0x2a, 0x0a, 0x04, 0x73, 0x6f, 0x6d, 0x65, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x16, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c,
	0x2e, 0x74, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x67, 0x2e, 0x53, 0x6f, 0x6d, 0x65, 0x52, 0x04, 0x73,
	0x6f, 0x6d, 0x65, 0x12, 0x35, 0x0a, 0x0a, 0x6f, 0x74, 0x68, 0x65, 0x72, 0x5f, 0x73, 0x6f, 0x6d,
	0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x16, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e,
	0x61, 0x6c, 0x2e, 0x74, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x67, 0x2e, 0x53, 0x6f, 0x6d, 0x65, 0x52,
	0x09, 0x6f, 0x74, 0x68, 0x65, 0x72, 0x53, 0x6f, 0x6d, 0x65, 0x12, 0x3b, 0x0a, 0x0f, 0x6f, 0x74,
	0x68, 0x65, 0x72, 0x5f, 0x73, 0x6f, 0x6d, 0x65, 0x5f, 0x6a, 0x73, 0x6f, 0x6e, 0x18, 0x04, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x16, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x74,
	0x65, 0x73, 0x74, 0x69, 0x6e, 0x67, 0x2e, 0x53, 0x6f, 0x6d, 0x65, 0x52, 0x0a, 0x63, 0x75, 0x73,
	0x74, 0x6f, 0x6d, 0x4a, 0x53, 0x4f, 0x4e, 0x12, 0x32, 0x0a, 0x06, 0x66, 0x69, 0x65, 0x6c, 0x64,
	0x73, 0x18, 0x64, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x46, 0x69, 0x65, 0x6c, 0x64, 0x4d,
	0x61, 0x73, 0x6b, 0x52, 0x06, 0x66, 0x69, 0x65, 0x6c, 0x64, 0x73, 0x12, 0x3d, 0x0a, 0x0c, 0x6f,
	0x74, 0x68, 0x65, 0x72, 0x5f, 0x66, 0x69, 0x65, 0x6c, 0x64, 0x73, 0x18, 0x65, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x62, 0x75, 0x66, 0x2e, 0x46, 0x69, 0x65, 0x6c, 0x64, 0x4d, 0x61, 0x73, 0x6b, 0x52, 0x0b, 0x6f,
	0x74, 0x68, 0x65, 0x72, 0x46, 0x69, 0x65, 0x6c, 0x64, 0x73, 0x22, 0x74, 0x0a, 0x05, 0x50, 0x72,
	0x6f, 0x70, 0x73, 0x12, 0x37, 0x0a, 0x0a, 0x70, 0x72, 0x6f, 0x70, 0x65, 0x72, 0x74, 0x69, 0x65,
	0x73, 0x18, 0x06, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x17, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x53, 0x74, 0x72, 0x75, 0x63, 0x74,
	0x52, 0x0a, 0x70, 0x72, 0x6f, 0x70, 0x65, 0x72, 0x74, 0x69, 0x65, 0x73, 0x12, 0x32, 0x0a, 0x06,
	0x66, 0x69, 0x65, 0x6c, 0x64, 0x73, 0x18, 0x64, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x46,
	0x69, 0x65, 0x6c, 0x64, 0x4d, 0x61, 0x73, 0x6b, 0x52, 0x06, 0x66, 0x69, 0x65, 0x6c, 0x64, 0x73,
	0x22, 0xb7, 0x01, 0x0a, 0x09, 0x57, 0x69, 0x74, 0x68, 0x49, 0x6e, 0x6e, 0x65, 0x72, 0x12, 0x35,
	0x0a, 0x04, 0x6d, 0x73, 0x67, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x21, 0x2e, 0x69,
	0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x74, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x67, 0x2e,
	0x57, 0x69, 0x74, 0x68, 0x49, 0x6e, 0x6e, 0x65, 0x72, 0x2e, 0x49, 0x6e, 0x6e, 0x65, 0x72, 0x52,
	0x04, 0x6d, 0x73, 0x67, 0x73, 0x1a, 0x73, 0x0a, 0x05, 0x49, 0x6e, 0x6e, 0x65, 0x72, 0x12, 0x32,
	0x0a, 0x06, 0x73, 0x69, 0x6d, 0x70, 0x6c, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x18,
	0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x74, 0x65, 0x73, 0x74, 0x69, 0x6e,
	0x67, 0x2e, 0x53, 0x69, 0x6d, 0x70, 0x6c, 0x65, 0x48, 0x00, 0x52, 0x06, 0x73, 0x69, 0x6d, 0x70,
	0x6c, 0x65, 0x12, 0x2f, 0x0a, 0x05, 0x70, 0x72, 0x6f, 0x70, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x17, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x74, 0x65, 0x73,
	0x74, 0x69, 0x6e, 0x67, 0x2e, 0x50, 0x72, 0x6f, 0x70, 0x73, 0x48, 0x00, 0x52, 0x05, 0x70, 0x72,
	0x6f, 0x70, 0x73, 0x42, 0x05, 0x0a, 0x03, 0x6d, 0x73, 0x67, 0x22, 0xa8, 0x07, 0x0a, 0x04, 0x46,
	0x75, 0x6c, 0x6c, 0x12, 0x10, 0x0a, 0x03, 0x69, 0x33, 0x32, 0x18, 0x01, 0x20, 0x01, 0x28, 0x05,
	0x52, 0x03, 0x69, 0x33, 0x32, 0x12, 0x10, 0x0a, 0x03, 0x69, 0x36, 0x34, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x03, 0x52, 0x03, 0x69, 0x36, 0x34, 0x12, 0x10, 0x0a, 0x03, 0x75, 0x33, 0x32, 0x18, 0x03,
	0x20, 0x01, 0x28, 0x0d, 0x52, 0x03, 0x75, 0x33, 0x32, 0x12, 0x10, 0x0a, 0x03, 0x75, 0x36, 0x34,
	0x18, 0x04, 0x20, 0x01, 0x28, 0x04, 0x52, 0x03, 0x75, 0x36, 0x34, 0x12, 0x10, 0x0a, 0x03, 0x66,
	0x33, 0x32, 0x18, 0x05, 0x20, 0x01, 0x28, 0x02, 0x52, 0x03, 0x66, 0x33, 0x32, 0x12, 0x10, 0x0a,
	0x03, 0x66, 0x36, 0x34, 0x18, 0x06, 0x20, 0x01, 0x28, 0x01, 0x52, 0x03, 0x66, 0x36, 0x34, 0x12,
	0x18, 0x0a, 0x07, 0x62, 0x6f, 0x6f, 0x6c, 0x65, 0x61, 0x6e, 0x18, 0x07, 0x20, 0x01, 0x28, 0x08,
	0x52, 0x07, 0x62, 0x6f, 0x6f, 0x6c, 0x65, 0x61, 0x6e, 0x12, 0x10, 0x0a, 0x03, 0x6e, 0x75, 0x6d,
	0x18, 0x08, 0x20, 0x01, 0x28, 0x05, 0x52, 0x03, 0x6e, 0x75, 0x6d, 0x12, 0x12, 0x0a, 0x04, 0x6e,
	0x75, 0x6d, 0x73, 0x18, 0x09, 0x20, 0x03, 0x28, 0x05, 0x52, 0x04, 0x6e, 0x75, 0x6d, 0x73, 0x12,
	0x10, 0x0a, 0x03, 0x73, 0x74, 0x72, 0x18, 0x0a, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x73, 0x74,
	0x72, 0x12, 0x12, 0x0a, 0x04, 0x73, 0x74, 0x72, 0x73, 0x18, 0x0b, 0x20, 0x03, 0x28, 0x09, 0x52,
	0x04, 0x73, 0x74, 0x72, 0x73, 0x12, 0x28, 0x0a, 0x03, 0x6d, 0x73, 0x67, 0x18, 0x0c, 0x20, 0x01,
	0x28, 0x0b, 0x32, 0x16, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x74, 0x65,
	0x73, 0x74, 0x69, 0x6e, 0x67, 0x2e, 0x46, 0x75, 0x6c, 0x6c, 0x52, 0x03, 0x6d, 0x73, 0x67, 0x12,
	0x2a, 0x0a, 0x04, 0x6d, 0x73, 0x67, 0x73, 0x18, 0x0d, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x16, 0x2e,
	0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x74, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x67,
	0x2e, 0x46, 0x75, 0x6c, 0x6c, 0x52, 0x04, 0x6d, 0x73, 0x67, 0x73, 0x12, 0x45, 0x0a, 0x0b, 0x6d,
	0x61, 0x70, 0x5f, 0x73, 0x74, 0x72, 0x5f, 0x6e, 0x75, 0x6d, 0x18, 0x0e, 0x20, 0x03, 0x28, 0x0b,
	0x32, 0x25, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x74, 0x65, 0x73, 0x74,
	0x69, 0x6e, 0x67, 0x2e, 0x46, 0x75, 0x6c, 0x6c, 0x2e, 0x4d, 0x61, 0x70, 0x53, 0x74, 0x72, 0x4e,
	0x75, 0x6d, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x09, 0x6d, 0x61, 0x70, 0x53, 0x74, 0x72, 0x4e,
	0x75, 0x6d, 0x12, 0x45, 0x0a, 0x0b, 0x6d, 0x61, 0x70, 0x5f, 0x6e, 0x75, 0x6d, 0x5f, 0x73, 0x74,
	0x72, 0x18, 0x0f, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x25, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e,
	0x61, 0x6c, 0x2e, 0x74, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x67, 0x2e, 0x46, 0x75, 0x6c, 0x6c, 0x2e,
	0x4d, 0x61, 0x70, 0x4e, 0x75, 0x6d, 0x53, 0x74, 0x72, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x09,
	0x6d, 0x61, 0x70, 0x4e, 0x75, 0x6d, 0x53, 0x74, 0x72, 0x12, 0x48, 0x0a, 0x0c, 0x6d, 0x61, 0x70,
	0x5f, 0x62, 0x6f, 0x6f, 0x6c, 0x5f, 0x73, 0x74, 0x72, 0x18, 0x10, 0x20, 0x03, 0x28, 0x0b, 0x32,
	0x26, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x74, 0x65, 0x73, 0x74, 0x69,
	0x6e, 0x67, 0x2e, 0x46, 0x75, 0x6c, 0x6c, 0x2e, 0x4d, 0x61, 0x70, 0x42, 0x6f, 0x6f, 0x6c, 0x53,
	0x74, 0x72, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x0a, 0x6d, 0x61, 0x70, 0x42, 0x6f, 0x6f, 0x6c,
	0x53, 0x74, 0x72, 0x12, 0x45, 0x0a, 0x0b, 0x6d, 0x61, 0x70, 0x5f, 0x73, 0x74, 0x72, 0x5f, 0x6d,
	0x73, 0x67, 0x18, 0x11, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x25, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72,
	0x6e, 0x61, 0x6c, 0x2e, 0x74, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x67, 0x2e, 0x46, 0x75, 0x6c, 0x6c,
	0x2e, 0x4d, 0x61, 0x70, 0x53, 0x74, 0x72, 0x4d, 0x73, 0x67, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52,
	0x09, 0x6d, 0x61, 0x70, 0x53, 0x74, 0x72, 0x4d, 0x73, 0x67, 0x12, 0x1b, 0x0a, 0x09, 0x6a, 0x73,
	0x6f, 0x6e, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x12, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x6a,
	0x73, 0x6f, 0x6e, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x2b, 0x0a, 0x10, 0x6a, 0x73, 0x6f, 0x6e, 0x5f,
	0x6e, 0x61, 0x6d, 0x65, 0x5f, 0x6f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x13, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x11, 0x61, 0x6e, 0x6f, 0x74, 0x68, 0x65, 0x72, 0x5f, 0x6a, 0x73, 0x6f, 0x6e, 0x5f,
	0x6e, 0x61, 0x6d, 0x65, 0x1a, 0x3c, 0x0a, 0x0e, 0x4d, 0x61, 0x70, 0x53, 0x74, 0x72, 0x4e, 0x75,
	0x6d, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75,
	0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x05, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x3a, 0x02,
	0x38, 0x01, 0x1a, 0x3c, 0x0a, 0x0e, 0x4d, 0x61, 0x70, 0x4e, 0x75, 0x6d, 0x53, 0x74, 0x72, 0x45,
	0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x05, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x3a, 0x02, 0x38, 0x01,
	0x1a, 0x3d, 0x0a, 0x0f, 0x4d, 0x61, 0x70, 0x42, 0x6f, 0x6f, 0x6c, 0x53, 0x74, 0x72, 0x45, 0x6e,
	0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x08,
	0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x3a, 0x02, 0x38, 0x01, 0x1a,
	0x54, 0x0a, 0x0e, 0x4d, 0x61, 0x70, 0x53, 0x74, 0x72, 0x4d, 0x73, 0x67, 0x45, 0x6e, 0x74, 0x72,
	0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03,
	0x6b, 0x65, 0x79, 0x12, 0x2c, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x0b, 0x32, 0x16, 0x2e, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x74, 0x65,
	0x73, 0x74, 0x69, 0x6e, 0x67, 0x2e, 0x46, 0x75, 0x6c, 0x6c, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75,
	0x65, 0x3a, 0x02, 0x38, 0x01, 0x42, 0x40, 0x5a, 0x3e, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f,
	0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x63, 0x6f,
	0x6d, 0x6d, 0x6f, 0x6e, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72,
	0x6e, 0x61, 0x6c, 0x2f, 0x74, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x67, 0x70, 0x62, 0x3b, 0x74, 0x65,
	0x73, 0x74, 0x69, 0x6e, 0x67, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_rawDescData []byte
)

func file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_rawDesc), len(file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_rawDescData
}

var file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_msgTypes = make([]protoimpl.MessageInfo, 10)
var file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_goTypes = []any{
	(*Some)(nil),                  // 0: internal.testing.Some
	(*Simple)(nil),                // 1: internal.testing.Simple
	(*Props)(nil),                 // 2: internal.testing.Props
	(*WithInner)(nil),             // 3: internal.testing.WithInner
	(*Full)(nil),                  // 4: internal.testing.Full
	(*WithInner_Inner)(nil),       // 5: internal.testing.WithInner.Inner
	nil,                           // 6: internal.testing.Full.MapStrNumEntry
	nil,                           // 7: internal.testing.Full.MapNumStrEntry
	nil,                           // 8: internal.testing.Full.MapBoolStrEntry
	nil,                           // 9: internal.testing.Full.MapStrMsgEntry
	(*fieldmaskpb.FieldMask)(nil), // 10: google.protobuf.FieldMask
	(*structpb.Struct)(nil),       // 11: google.protobuf.Struct
}
var file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_depIdxs = []int32{
	0,  // 0: internal.testing.Simple.some:type_name -> internal.testing.Some
	0,  // 1: internal.testing.Simple.other_some:type_name -> internal.testing.Some
	0,  // 2: internal.testing.Simple.other_some_json:type_name -> internal.testing.Some
	10, // 3: internal.testing.Simple.fields:type_name -> google.protobuf.FieldMask
	10, // 4: internal.testing.Simple.other_fields:type_name -> google.protobuf.FieldMask
	11, // 5: internal.testing.Props.properties:type_name -> google.protobuf.Struct
	10, // 6: internal.testing.Props.fields:type_name -> google.protobuf.FieldMask
	5,  // 7: internal.testing.WithInner.msgs:type_name -> internal.testing.WithInner.Inner
	4,  // 8: internal.testing.Full.msg:type_name -> internal.testing.Full
	4,  // 9: internal.testing.Full.msgs:type_name -> internal.testing.Full
	6,  // 10: internal.testing.Full.map_str_num:type_name -> internal.testing.Full.MapStrNumEntry
	7,  // 11: internal.testing.Full.map_num_str:type_name -> internal.testing.Full.MapNumStrEntry
	8,  // 12: internal.testing.Full.map_bool_str:type_name -> internal.testing.Full.MapBoolStrEntry
	9,  // 13: internal.testing.Full.map_str_msg:type_name -> internal.testing.Full.MapStrMsgEntry
	1,  // 14: internal.testing.WithInner.Inner.simple:type_name -> internal.testing.Simple
	2,  // 15: internal.testing.WithInner.Inner.props:type_name -> internal.testing.Props
	4,  // 16: internal.testing.Full.MapStrMsgEntry.value:type_name -> internal.testing.Full
	17, // [17:17] is the sub-list for method output_type
	17, // [17:17] is the sub-list for method input_type
	17, // [17:17] is the sub-list for extension type_name
	17, // [17:17] is the sub-list for extension extendee
	0,  // [0:17] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_init() }
func file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_init() {
	if File_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto != nil {
		return
	}
	file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_msgTypes[5].OneofWrappers = []any{
		(*WithInner_Inner_Simple)(nil),
		(*WithInner_Inner_Props)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_rawDesc), len(file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   10,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto = out.File
	file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_goTypes = nil
	file_go_chromium_org_luci_common_proto_internal_testingpb_testing_proto_depIdxs = nil
}
