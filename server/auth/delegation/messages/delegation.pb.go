// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// This file is copied from luci-py.git:
// appengine/components/components/auth/proto/delegation.proto
// Commit: f615e7592619691fe9fa64997880e2490072db21
//
// Changes: renamed package to 'messages'.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.5
// 	protoc        v6.30.2
// source: go.chromium.org/luci/server/auth/delegation/messages/delegation.proto

package messages

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

type Subtoken_Kind int32

const (
	// This is to catch old tokens that don't have 'kind' field yet.
	//
	// Tokens of this kind are interpreted as 'BEARER_DELEGATION_TOKEN' for now,
	// for compatibility. But eventually (when all backends are updated), they
	// will become invalid (and there will be no way to generate them). This is
	// needed to avoid old servers accidentally interpret tokens of kind != 0 as
	// BEARER_DELEGATION_TOKEN tokens.
	Subtoken_UNKNOWN_KIND Subtoken_Kind = 0
	// The token of this kind can be sent in X-Delegation-Token-V1 HTTP header.
	// The services will check all restrictions of the token, and will
	// authenticate requests as coming from 'delegated_identity'.
	Subtoken_BEARER_DELEGATION_TOKEN Subtoken_Kind = 1
)

// Enum value maps for Subtoken_Kind.
var (
	Subtoken_Kind_name = map[int32]string{
		0: "UNKNOWN_KIND",
		1: "BEARER_DELEGATION_TOKEN",
	}
	Subtoken_Kind_value = map[string]int32{
		"UNKNOWN_KIND":            0,
		"BEARER_DELEGATION_TOKEN": 1,
	}
)

func (x Subtoken_Kind) Enum() *Subtoken_Kind {
	p := new(Subtoken_Kind)
	*p = x
	return p
}

func (x Subtoken_Kind) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (Subtoken_Kind) Descriptor() protoreflect.EnumDescriptor {
	return file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_enumTypes[0].Descriptor()
}

func (Subtoken_Kind) Type() protoreflect.EnumType {
	return &file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_enumTypes[0]
}

func (x Subtoken_Kind) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use Subtoken_Kind.Descriptor instead.
func (Subtoken_Kind) EnumDescriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_rawDescGZIP(), []int{1, 0}
}

// Signed serialized Subtoken.
//
// This message is just an envelope that carries the serialized Subtoken message
// and its signature.
//
// Next ID: 6.
type DelegationToken struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Identity of a service that signed this token.
	//
	// It can be a 'service:<app-id>' string or 'user:<service-account-email>'
	// string.
	//
	// In both cases the appropriate certificate store will be queried (via SSL)
	// for the public key to use for signature verification.
	SignerId string `protobuf:"bytes,2,opt,name=signer_id,json=signerId,proto3" json:"signer_id,omitempty"`
	// ID of a key used for making the signature.
	//
	// There can be multiple active keys at any moment in time: one used for new
	// signatures, and one being rotated out (but still valid for verification).
	//
	// The lifetime of the token indirectly depends on the lifetime of the signing
	// key, which is 24h. So delegation tokens can't live longer than 24h.
	SigningKeyId string `protobuf:"bytes,3,opt,name=signing_key_id,json=signingKeyId,proto3" json:"signing_key_id,omitempty"`
	// The signature: PKCS1_v1_5+SHA256(serialized_subtoken, signing_key_id).
	Pkcs1Sha256Sig []byte `protobuf:"bytes,4,opt,name=pkcs1_sha256_sig,json=pkcs1Sha256Sig,proto3" json:"pkcs1_sha256_sig,omitempty"`
	// Serialized Subtoken message. It's signature is stored in pkcs1_sha256_sig.
	SerializedSubtoken []byte `protobuf:"bytes,5,opt,name=serialized_subtoken,json=serializedSubtoken,proto3" json:"serialized_subtoken,omitempty"`
	unknownFields      protoimpl.UnknownFields
	sizeCache          protoimpl.SizeCache
}

func (x *DelegationToken) Reset() {
	*x = DelegationToken{}
	mi := &file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *DelegationToken) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DelegationToken) ProtoMessage() {}

func (x *DelegationToken) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DelegationToken.ProtoReflect.Descriptor instead.
func (*DelegationToken) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_rawDescGZIP(), []int{0}
}

func (x *DelegationToken) GetSignerId() string {
	if x != nil {
		return x.SignerId
	}
	return ""
}

func (x *DelegationToken) GetSigningKeyId() string {
	if x != nil {
		return x.SigningKeyId
	}
	return ""
}

func (x *DelegationToken) GetPkcs1Sha256Sig() []byte {
	if x != nil {
		return x.Pkcs1Sha256Sig
	}
	return nil
}

func (x *DelegationToken) GetSerializedSubtoken() []byte {
	if x != nil {
		return x.SerializedSubtoken
	}
	return nil
}

// Identifies who delegates what authority to whom where.
//
// Next ID: 10.
type Subtoken struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// What kind of token is this.
	//
	// Defines how it can be used. See comments for Kind enum.
	Kind Subtoken_Kind `protobuf:"varint,8,opt,name=kind,proto3,enum=messages.Subtoken_Kind" json:"kind,omitempty"`
	// Identifier of this subtoken as generated by the token server.
	//
	// Used for logging and tracking purposes.
	SubtokenId int64 `protobuf:"varint,4,opt,name=subtoken_id,json=subtokenId,proto3" json:"subtoken_id,omitempty"`
	// Identity whose authority is delegated.
	//
	// A string of the form "user:<email>".
	DelegatedIdentity string `protobuf:"bytes,1,opt,name=delegated_identity,json=delegatedIdentity,proto3" json:"delegated_identity,omitempty"`
	// Who requested this token.
	//
	// This can match delegated_identity if the user is delegating their own
	// identity or it can be a different id if the token is actually
	// an impersonation token.
	RequestorIdentity string `protobuf:"bytes,7,opt,name=requestor_identity,json=requestorIdentity,proto3" json:"requestor_identity,omitempty"`
	// When the token was generated (and when it becomes valid).
	//
	// Number of seconds since epoch (Unix timestamp).
	CreationTime int64 `protobuf:"varint,2,opt,name=creation_time,json=creationTime,proto3" json:"creation_time,omitempty"`
	// How long the token is considered valid (in seconds).
	ValidityDuration int32 `protobuf:"varint,3,opt,name=validity_duration,json=validityDuration,proto3" json:"validity_duration,omitempty"`
	// Who can present this token.
	//
	// Each item can be an identity string (e.g. "user:<email>"), a "group:<name>"
	// string, or special "*" string which means "Any bearer can use the token".
	Audience []string `protobuf:"bytes,5,rep,name=audience,proto3" json:"audience,omitempty"`
	// What services should accept this token.
	//
	// List of services (specified as service identities, e.g. "service:app-id")
	// that should accept this token. May also contain special "*" string, which
	// means "All services".
	Services []string `protobuf:"bytes,6,rep,name=services,proto3" json:"services,omitempty"`
	// Arbitrary key:value pairs embedded into the token by whoever requested it.
	// Convey circumstance of why the token is created.
	//
	// Services that accept the token may use them for additional authorization
	// decisions. Please use extremely carefully, only when you control both sides
	// of the delegation link and can guarantee that services involved understand
	// the tags.
	Tags          []string `protobuf:"bytes,9,rep,name=tags,proto3" json:"tags,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *Subtoken) Reset() {
	*x = Subtoken{}
	mi := &file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Subtoken) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Subtoken) ProtoMessage() {}

func (x *Subtoken) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Subtoken.ProtoReflect.Descriptor instead.
func (*Subtoken) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_rawDescGZIP(), []int{1}
}

func (x *Subtoken) GetKind() Subtoken_Kind {
	if x != nil {
		return x.Kind
	}
	return Subtoken_UNKNOWN_KIND
}

func (x *Subtoken) GetSubtokenId() int64 {
	if x != nil {
		return x.SubtokenId
	}
	return 0
}

func (x *Subtoken) GetDelegatedIdentity() string {
	if x != nil {
		return x.DelegatedIdentity
	}
	return ""
}

func (x *Subtoken) GetRequestorIdentity() string {
	if x != nil {
		return x.RequestorIdentity
	}
	return ""
}

func (x *Subtoken) GetCreationTime() int64 {
	if x != nil {
		return x.CreationTime
	}
	return 0
}

func (x *Subtoken) GetValidityDuration() int32 {
	if x != nil {
		return x.ValidityDuration
	}
	return 0
}

func (x *Subtoken) GetAudience() []string {
	if x != nil {
		return x.Audience
	}
	return nil
}

func (x *Subtoken) GetServices() []string {
	if x != nil {
		return x.Services
	}
	return nil
}

func (x *Subtoken) GetTags() []string {
	if x != nil {
		return x.Tags
	}
	return nil
}

var File_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_rawDesc = string([]byte{
	0x0a, 0x45, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2f, 0x61, 0x75,
	0x74, 0x68, 0x2f, 0x64, 0x65, 0x6c, 0x65, 0x67, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2f, 0x6d, 0x65,
	0x73, 0x73, 0x61, 0x67, 0x65, 0x73, 0x2f, 0x64, 0x65, 0x6c, 0x65, 0x67, 0x61, 0x74, 0x69, 0x6f,
	0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x08, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65,
	0x73, 0x22, 0xb5, 0x01, 0x0a, 0x0f, 0x44, 0x65, 0x6c, 0x65, 0x67, 0x61, 0x74, 0x69, 0x6f, 0x6e,
	0x54, 0x6f, 0x6b, 0x65, 0x6e, 0x12, 0x1b, 0x0a, 0x09, 0x73, 0x69, 0x67, 0x6e, 0x65, 0x72, 0x5f,
	0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x73, 0x69, 0x67, 0x6e, 0x65, 0x72,
	0x49, 0x64, 0x12, 0x24, 0x0a, 0x0e, 0x73, 0x69, 0x67, 0x6e, 0x69, 0x6e, 0x67, 0x5f, 0x6b, 0x65,
	0x79, 0x5f, 0x69, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0c, 0x73, 0x69, 0x67, 0x6e,
	0x69, 0x6e, 0x67, 0x4b, 0x65, 0x79, 0x49, 0x64, 0x12, 0x28, 0x0a, 0x10, 0x70, 0x6b, 0x63, 0x73,
	0x31, 0x5f, 0x73, 0x68, 0x61, 0x32, 0x35, 0x36, 0x5f, 0x73, 0x69, 0x67, 0x18, 0x04, 0x20, 0x01,
	0x28, 0x0c, 0x52, 0x0e, 0x70, 0x6b, 0x63, 0x73, 0x31, 0x53, 0x68, 0x61, 0x32, 0x35, 0x36, 0x53,
	0x69, 0x67, 0x12, 0x2f, 0x0a, 0x13, 0x73, 0x65, 0x72, 0x69, 0x61, 0x6c, 0x69, 0x7a, 0x65, 0x64,
	0x5f, 0x73, 0x75, 0x62, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0c, 0x52,
	0x12, 0x73, 0x65, 0x72, 0x69, 0x61, 0x6c, 0x69, 0x7a, 0x65, 0x64, 0x53, 0x75, 0x62, 0x74, 0x6f,
	0x6b, 0x65, 0x6e, 0x4a, 0x04, 0x08, 0x01, 0x10, 0x02, 0x22, 0x8b, 0x03, 0x0a, 0x08, 0x53, 0x75,
	0x62, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x12, 0x2b, 0x0a, 0x04, 0x6b, 0x69, 0x6e, 0x64, 0x18, 0x08,
	0x20, 0x01, 0x28, 0x0e, 0x32, 0x17, 0x2e, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x73, 0x2e,
	0x53, 0x75, 0x62, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x2e, 0x4b, 0x69, 0x6e, 0x64, 0x52, 0x04, 0x6b,
	0x69, 0x6e, 0x64, 0x12, 0x1f, 0x0a, 0x0b, 0x73, 0x75, 0x62, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x5f,
	0x69, 0x64, 0x18, 0x04, 0x20, 0x01, 0x28, 0x03, 0x52, 0x0a, 0x73, 0x75, 0x62, 0x74, 0x6f, 0x6b,
	0x65, 0x6e, 0x49, 0x64, 0x12, 0x2d, 0x0a, 0x12, 0x64, 0x65, 0x6c, 0x65, 0x67, 0x61, 0x74, 0x65,
	0x64, 0x5f, 0x69, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x74, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x11, 0x64, 0x65, 0x6c, 0x65, 0x67, 0x61, 0x74, 0x65, 0x64, 0x49, 0x64, 0x65, 0x6e, 0x74,
	0x69, 0x74, 0x79, 0x12, 0x2d, 0x0a, 0x12, 0x72, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x6f, 0x72,
	0x5f, 0x69, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x74, 0x79, 0x18, 0x07, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x11, 0x72, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x6f, 0x72, 0x49, 0x64, 0x65, 0x6e, 0x74, 0x69,
	0x74, 0x79, 0x12, 0x23, 0x0a, 0x0d, 0x63, 0x72, 0x65, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x74,
	0x69, 0x6d, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x03, 0x52, 0x0c, 0x63, 0x72, 0x65, 0x61, 0x74,
	0x69, 0x6f, 0x6e, 0x54, 0x69, 0x6d, 0x65, 0x12, 0x2b, 0x0a, 0x11, 0x76, 0x61, 0x6c, 0x69, 0x64,
	0x69, 0x74, 0x79, 0x5f, 0x64, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x03, 0x20, 0x01,
	0x28, 0x05, 0x52, 0x10, 0x76, 0x61, 0x6c, 0x69, 0x64, 0x69, 0x74, 0x79, 0x44, 0x75, 0x72, 0x61,
	0x74, 0x69, 0x6f, 0x6e, 0x12, 0x1a, 0x0a, 0x08, 0x61, 0x75, 0x64, 0x69, 0x65, 0x6e, 0x63, 0x65,
	0x18, 0x05, 0x20, 0x03, 0x28, 0x09, 0x52, 0x08, 0x61, 0x75, 0x64, 0x69, 0x65, 0x6e, 0x63, 0x65,
	0x12, 0x1a, 0x0a, 0x08, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x73, 0x18, 0x06, 0x20, 0x03,
	0x28, 0x09, 0x52, 0x08, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x73, 0x12, 0x12, 0x0a, 0x04,
	0x74, 0x61, 0x67, 0x73, 0x18, 0x09, 0x20, 0x03, 0x28, 0x09, 0x52, 0x04, 0x74, 0x61, 0x67, 0x73,
	0x22, 0x35, 0x0a, 0x04, 0x4b, 0x69, 0x6e, 0x64, 0x12, 0x10, 0x0a, 0x0c, 0x55, 0x4e, 0x4b, 0x4e,
	0x4f, 0x57, 0x4e, 0x5f, 0x4b, 0x49, 0x4e, 0x44, 0x10, 0x00, 0x12, 0x1b, 0x0a, 0x17, 0x42, 0x45,
	0x41, 0x52, 0x45, 0x52, 0x5f, 0x44, 0x45, 0x4c, 0x45, 0x47, 0x41, 0x54, 0x49, 0x4f, 0x4e, 0x5f,
	0x54, 0x4f, 0x4b, 0x45, 0x4e, 0x10, 0x01, 0x42, 0x36, 0x5a, 0x34, 0x67, 0x6f, 0x2e, 0x63, 0x68,
	0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f,
	0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2f, 0x61, 0x75, 0x74, 0x68, 0x2f, 0x64, 0x65, 0x6c, 0x65,
	0x67, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2f, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x73, 0x62,
	0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_rawDescData []byte
)

func file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_rawDesc), len(file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_rawDescData
}

var file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_goTypes = []any{
	(Subtoken_Kind)(0),      // 0: messages.Subtoken.Kind
	(*DelegationToken)(nil), // 1: messages.DelegationToken
	(*Subtoken)(nil),        // 2: messages.Subtoken
}
var file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_depIdxs = []int32{
	0, // 0: messages.Subtoken.kind:type_name -> messages.Subtoken.Kind
	1, // [1:1] is the sub-list for method output_type
	1, // [1:1] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_init() }
func file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_init() {
	if File_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_rawDesc), len(file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_rawDesc)),
			NumEnums:      1,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_depIdxs,
		EnumInfos:         file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_enumTypes,
		MessageInfos:      file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto = out.File
	file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_goTypes = nil
	file_go_chromium_org_luci_server_auth_delegation_messages_delegation_proto_depIdxs = nil
}
