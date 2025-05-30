// Copyright 2022 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.36.5
// 	protoc        v6.30.2
// source: go.chromium.org/luci/server/quota/quotapb/ids.proto

package quotapb

import (
	_ "github.com/envoyproxy/protoc-gen-validate/validate"
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

// Identifies a PolicyConfig (group of Policies).
type PolicyConfigID struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The short application id.
	//
	// This is used to separate different applications in the same service
	// deployment.
	//
	// This should be a short indicator of what logical service this PolicyConfig
	// applies to (like `cv` or `rdb`, not `luci-cv.appspot.com`).
	AppId string `protobuf:"bytes,1,opt,name=app_id,json=appId,proto3" json:"app_id,omitempty"`
	// The LUCI realm that the PolicyConfig belongs to.
	//
	// This is a global realm string (i.e. includes the project).
	//
	// NOTE: This realm and the Account realm will USUALLY NOT match. The realm here
	// is indicating the realm in which this PolicyConfig is administered (e.g.
	// admins/services can write, read or purge it). The Account realm indicates
	// where the governed _resource_ lives, and the service can apply a Policy
	// from a Config in any realm it needs to.
	//
	// Examples:
	//
	//	chromium:@project
	//	@internal:<LUCI service-name>
	Realm string `protobuf:"bytes,2,opt,name=realm,proto3" json:"realm,omitempty"`
	// Version scheme indicates which algorithm was used to calculate `version`.
	//
	// The value `0` indicates that `version` was provided by the user.
	//
	// Currently the highest version_scheme supported is `1`.
	VersionScheme uint32 `protobuf:"varint,3,opt,name=version_scheme,json=versionScheme,proto3" json:"version_scheme,omitempty"`
	// The version of this PolicyConfig (ASI).
	Version       string `protobuf:"bytes,4,opt,name=version,proto3" json:"version,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *PolicyConfigID) Reset() {
	*x = PolicyConfigID{}
	mi := &file_go_chromium_org_luci_server_quota_quotapb_ids_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *PolicyConfigID) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PolicyConfigID) ProtoMessage() {}

func (x *PolicyConfigID) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_server_quota_quotapb_ids_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PolicyConfigID.ProtoReflect.Descriptor instead.
func (*PolicyConfigID) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_server_quota_quotapb_ids_proto_rawDescGZIP(), []int{0}
}

func (x *PolicyConfigID) GetAppId() string {
	if x != nil {
		return x.AppId
	}
	return ""
}

func (x *PolicyConfigID) GetRealm() string {
	if x != nil {
		return x.Realm
	}
	return ""
}

func (x *PolicyConfigID) GetVersionScheme() uint32 {
	if x != nil {
		return x.VersionScheme
	}
	return 0
}

func (x *PolicyConfigID) GetVersion() string {
	if x != nil {
		return x.Version
	}
	return ""
}

// Looking up a single policy requires a PolicyConfigID and a PolicyKey.
//
// The PolicyConfigID identifies which policy config contains the entry required,
// and the PolicyKey refers to a specific policy within that config.
//
// Also see PolicyRef.
type PolicyID struct {
	state         protoimpl.MessageState `protogen:"open.v1"`
	Config        *PolicyConfigID        `protobuf:"bytes,1,opt,name=config,proto3" json:"config,omitempty"`
	Key           *PolicyKey             `protobuf:"bytes,2,opt,name=key,proto3" json:"key,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *PolicyID) Reset() {
	*x = PolicyID{}
	mi := &file_go_chromium_org_luci_server_quota_quotapb_ids_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *PolicyID) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PolicyID) ProtoMessage() {}

func (x *PolicyID) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_server_quota_quotapb_ids_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PolicyID.ProtoReflect.Descriptor instead.
func (*PolicyID) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_server_quota_quotapb_ids_proto_rawDescGZIP(), []int{1}
}

func (x *PolicyID) GetConfig() *PolicyConfigID {
	if x != nil {
		return x.Config
	}
	return nil
}

func (x *PolicyID) GetKey() *PolicyKey {
	if x != nil {
		return x.Key
	}
	return nil
}

// PolicyRef holds the same data as a PolicyID, but in string-encoded form.
//
// This is used by the quota library internally, but also appears in e.g.
// Account (because the Account object needs to be directly manipulated by the
// quota library internals). All RPC and API surfaces for the quota library use
// PolicyID instead.
//
// TODO(iannucci) -- add regex?
type PolicyRef struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// config is a PolicyConfigID where all fields have been joined with `~`, and
	// then `"a~p~` prepended.
	Config string `protobuf:"bytes,1,opt,name=config,proto3" json:"config,omitempty"`
	// key is a PolicyKey where all fields have been joined with `~`.
	Key           string `protobuf:"bytes,2,opt,name=key,proto3" json:"key,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *PolicyRef) Reset() {
	*x = PolicyRef{}
	mi := &file_go_chromium_org_luci_server_quota_quotapb_ids_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *PolicyRef) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PolicyRef) ProtoMessage() {}

func (x *PolicyRef) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_server_quota_quotapb_ids_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PolicyRef.ProtoReflect.Descriptor instead.
func (*PolicyRef) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_server_quota_quotapb_ids_proto_rawDescGZIP(), []int{2}
}

func (x *PolicyRef) GetConfig() string {
	if x != nil {
		return x.Config
	}
	return ""
}

func (x *PolicyRef) GetKey() string {
	if x != nil {
		return x.Key
	}
	return ""
}

// PolicyKey identifies a single Policy inside of a PolicyConfig.
type PolicyKey struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The namespace of the Policy (ASI).
	//
	// Used by the application to logically separate multiple policy groups
	// within the same realm.
	//
	// Examples:
	//
	//	cqGroupName
	//	RPCService.RPCName
	Namespace string `protobuf:"bytes,1,opt,name=namespace,proto3" json:"namespace,omitempty"`
	// The name of the Policy (ASI)
	//
	// Examples:
	//
	//	account|user:identity
	//	sharedQuota|sharedAccountName
	Name string `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	// The type of resource managed by this policy (ASI)
	//
	// (e.g. runs, qps, data_size, etc.)
	ResourceType  string `protobuf:"bytes,3,opt,name=resource_type,json=resourceType,proto3" json:"resource_type,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *PolicyKey) Reset() {
	*x = PolicyKey{}
	mi := &file_go_chromium_org_luci_server_quota_quotapb_ids_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *PolicyKey) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PolicyKey) ProtoMessage() {}

func (x *PolicyKey) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_server_quota_quotapb_ids_proto_msgTypes[3]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PolicyKey.ProtoReflect.Descriptor instead.
func (*PolicyKey) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_server_quota_quotapb_ids_proto_rawDescGZIP(), []int{3}
}

func (x *PolicyKey) GetNamespace() string {
	if x != nil {
		return x.Namespace
	}
	return ""
}

func (x *PolicyKey) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *PolicyKey) GetResourceType() string {
	if x != nil {
		return x.ResourceType
	}
	return ""
}

// Identifies a single Account.
type AccountID struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// The short application id.
	//
	// This is used to separate different applications in the same service
	// deployment.
	//
	// This should be a short indicator of what logical service this Account
	// applies to (like `cv` or `rdb`, not `luci-cv.appspot.com`).
	//
	// Examples:
	//
	//	cv
	//	rdb
	AppId string `protobuf:"bytes,1,opt,name=app_id,json=appId,proto3" json:"app_id,omitempty"`
	// The LUCI realm that the Account belongs to.
	//
	// This is a global realm string (i.e. includes the project).
	//
	// NOTE: This realm and the PolicyConfig realm will USUALLY NOT match. The
	// realm here is indicating the realm of the resource governed by this
	// Account.
	//
	// Examples:
	//
	//	chromium:ci
	//	@internal:<LUCI service-name>
	Realm string `protobuf:"bytes,2,opt,name=realm,proto3" json:"realm,omitempty"`
	// The LUCI Auth identity that this Account belongs to, or "" if the account
	// is not owned by a specific identity.
	//
	// Identities look like:
	//   - anonymous:anonymous
	//   - bot:...
	//   - project:...
	//   - service:...
	//   - user:...
	Identity string `protobuf:"bytes,3,opt,name=identity,proto3" json:"identity,omitempty"`
	// The namespace of the Account (ASI).
	//
	// Used by the application to logically separate multiple account groups
	// within the same realm.
	//
	// Examples:
	//
	//	cqGroupName
	//	RPCService.RPCName
	Namespace string `protobuf:"bytes,4,opt,name=namespace,proto3" json:"namespace,omitempty"`
	// The name of the Account (ASI).
	//
	// This may be empty (e.g. if the account is sufficiently identitified by
	// realm/identity/namespace, etc).
	//
	// Examples:
	//
	//	"sharedAccountName"
	//	""
	Name string `protobuf:"bytes,5,opt,name=name,proto3" json:"name,omitempty"`
	// The type of resource managed by this account.
	//
	// Examples:
	//
	//	qps
	//	runningBuilds
	ResourceType  string `protobuf:"bytes,6,opt,name=resource_type,json=resourceType,proto3" json:"resource_type,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *AccountID) Reset() {
	*x = AccountID{}
	mi := &file_go_chromium_org_luci_server_quota_quotapb_ids_proto_msgTypes[4]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *AccountID) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AccountID) ProtoMessage() {}

func (x *AccountID) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_server_quota_quotapb_ids_proto_msgTypes[4]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AccountID.ProtoReflect.Descriptor instead.
func (*AccountID) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_server_quota_quotapb_ids_proto_rawDescGZIP(), []int{4}
}

func (x *AccountID) GetAppId() string {
	if x != nil {
		return x.AppId
	}
	return ""
}

func (x *AccountID) GetRealm() string {
	if x != nil {
		return x.Realm
	}
	return ""
}

func (x *AccountID) GetIdentity() string {
	if x != nil {
		return x.Identity
	}
	return ""
}

func (x *AccountID) GetNamespace() string {
	if x != nil {
		return x.Namespace
	}
	return ""
}

func (x *AccountID) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *AccountID) GetResourceType() string {
	if x != nil {
		return x.ResourceType
	}
	return ""
}

var File_go_chromium_org_luci_server_quota_quotapb_ids_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_server_quota_quotapb_ids_proto_rawDesc = string([]byte{
	0x0a, 0x33, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2f, 0x71, 0x75,
	0x6f, 0x74, 0x61, 0x2f, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x70, 0x62, 0x2f, 0x69, 0x64, 0x73, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x29, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69,
	0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x73, 0x65, 0x72, 0x76,
	0x65, 0x72, 0x2e, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x2e, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x70, 0x62,
	0x1a, 0x17, 0x76, 0x61, 0x6c, 0x69, 0x64, 0x61, 0x74, 0x65, 0x2f, 0x76, 0x61, 0x6c, 0x69, 0x64,
	0x61, 0x74, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0xa8, 0x01, 0x0a, 0x0e, 0x50, 0x6f,
	0x6c, 0x69, 0x63, 0x79, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x49, 0x44, 0x12, 0x20, 0x0a, 0x06,
	0x61, 0x70, 0x70, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x42, 0x09, 0xfa, 0x42,
	0x06, 0x72, 0x04, 0xba, 0x01, 0x01, 0x7e, 0x52, 0x05, 0x61, 0x70, 0x70, 0x49, 0x64, 0x12, 0x1f,
	0x0a, 0x05, 0x72, 0x65, 0x61, 0x6c, 0x6d, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x42, 0x09, 0xfa,
	0x42, 0x06, 0x72, 0x04, 0xba, 0x01, 0x01, 0x7e, 0x52, 0x05, 0x72, 0x65, 0x61, 0x6c, 0x6d, 0x12,
	0x2e, 0x0a, 0x0e, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x5f, 0x73, 0x63, 0x68, 0x65, 0x6d,
	0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0d, 0x42, 0x07, 0xfa, 0x42, 0x04, 0x2a, 0x02, 0x18, 0x01,
	0x52, 0x0d, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x53, 0x63, 0x68, 0x65, 0x6d, 0x65, 0x12,
	0x23, 0x0a, 0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09,
	0x42, 0x09, 0xfa, 0x42, 0x06, 0x72, 0x04, 0xba, 0x01, 0x01, 0x7e, 0x52, 0x07, 0x76, 0x65, 0x72,
	0x73, 0x69, 0x6f, 0x6e, 0x22, 0xb9, 0x01, 0x0a, 0x08, 0x50, 0x6f, 0x6c, 0x69, 0x63, 0x79, 0x49,
	0x44, 0x12, 0x5b, 0x0a, 0x06, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x39, 0x2e, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e,
	0x6f, 0x72, 0x67, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2e,
	0x71, 0x75, 0x6f, 0x74, 0x61, 0x2e, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x70, 0x62, 0x2e, 0x50, 0x6f,
	0x6c, 0x69, 0x63, 0x79, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x49, 0x44, 0x42, 0x08, 0xfa, 0x42,
	0x05, 0x8a, 0x01, 0x02, 0x10, 0x01, 0x52, 0x06, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x12, 0x50,
	0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x34, 0x2e, 0x67, 0x6f,
	0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2e, 0x6c, 0x75,
	0x63, 0x69, 0x2e, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2e, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x2e,
	0x71, 0x75, 0x6f, 0x74, 0x61, 0x70, 0x62, 0x2e, 0x50, 0x6f, 0x6c, 0x69, 0x63, 0x79, 0x4b, 0x65,
	0x79, 0x42, 0x08, 0xfa, 0x42, 0x05, 0x8a, 0x01, 0x02, 0x10, 0x01, 0x52, 0x03, 0x6b, 0x65, 0x79,
	0x22, 0x35, 0x0a, 0x09, 0x50, 0x6f, 0x6c, 0x69, 0x63, 0x79, 0x52, 0x65, 0x66, 0x12, 0x16, 0x0a,
	0x06, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x63,
	0x6f, 0x6e, 0x66, 0x69, 0x67, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x22, 0x83, 0x01, 0x0a, 0x09, 0x50, 0x6f, 0x6c, 0x69,
	0x63, 0x79, 0x4b, 0x65, 0x79, 0x12, 0x27, 0x0a, 0x09, 0x6e, 0x61, 0x6d, 0x65, 0x73, 0x70, 0x61,
	0x63, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x42, 0x09, 0xfa, 0x42, 0x06, 0x72, 0x04, 0xba,
	0x01, 0x01, 0x7e, 0x52, 0x09, 0x6e, 0x61, 0x6d, 0x65, 0x73, 0x70, 0x61, 0x63, 0x65, 0x12, 0x1d,
	0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x42, 0x09, 0xfa, 0x42,
	0x06, 0x72, 0x04, 0xba, 0x01, 0x01, 0x7e, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x2e, 0x0a,
	0x0d, 0x72, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x5f, 0x74, 0x79, 0x70, 0x65, 0x18, 0x03,
	0x20, 0x01, 0x28, 0x09, 0x42, 0x09, 0xfa, 0x42, 0x06, 0x72, 0x04, 0xba, 0x01, 0x01, 0x7e, 0x52,
	0x0c, 0x72, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x54, 0x79, 0x70, 0x65, 0x22, 0xed, 0x01,
	0x0a, 0x09, 0x41, 0x63, 0x63, 0x6f, 0x75, 0x6e, 0x74, 0x49, 0x44, 0x12, 0x20, 0x0a, 0x06, 0x61,
	0x70, 0x70, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x42, 0x09, 0xfa, 0x42, 0x06,
	0x72, 0x04, 0xba, 0x01, 0x01, 0x7e, 0x52, 0x05, 0x61, 0x70, 0x70, 0x49, 0x64, 0x12, 0x1f, 0x0a,
	0x05, 0x72, 0x65, 0x61, 0x6c, 0x6d, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x42, 0x09, 0xfa, 0x42,
	0x06, 0x72, 0x04, 0xba, 0x01, 0x01, 0x7e, 0x52, 0x05, 0x72, 0x65, 0x61, 0x6c, 0x6d, 0x12, 0x25,
	0x0a, 0x08, 0x69, 0x64, 0x65, 0x6e, 0x74, 0x69, 0x74, 0x79, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09,
	0x42, 0x09, 0xfa, 0x42, 0x06, 0x72, 0x04, 0xba, 0x01, 0x01, 0x7e, 0x52, 0x08, 0x69, 0x64, 0x65,
	0x6e, 0x74, 0x69, 0x74, 0x79, 0x12, 0x27, 0x0a, 0x09, 0x6e, 0x61, 0x6d, 0x65, 0x73, 0x70, 0x61,
	0x63, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x42, 0x09, 0xfa, 0x42, 0x06, 0x72, 0x04, 0xba,
	0x01, 0x01, 0x7e, 0x52, 0x09, 0x6e, 0x61, 0x6d, 0x65, 0x73, 0x70, 0x61, 0x63, 0x65, 0x12, 0x1d,
	0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09, 0x42, 0x09, 0xfa, 0x42,
	0x06, 0x72, 0x04, 0xba, 0x01, 0x01, 0x7e, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x2e, 0x0a,
	0x0d, 0x72, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x5f, 0x74, 0x79, 0x70, 0x65, 0x18, 0x06,
	0x20, 0x01, 0x28, 0x09, 0x42, 0x09, 0xfa, 0x42, 0x06, 0x72, 0x04, 0xba, 0x01, 0x01, 0x7e, 0x52,
	0x0c, 0x72, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x54, 0x79, 0x70, 0x65, 0x42, 0x2b, 0x5a,
	0x29, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67,
	0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2f, 0x71, 0x75, 0x6f,
	0x74, 0x61, 0x2f, 0x71, 0x75, 0x6f, 0x74, 0x61, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_server_quota_quotapb_ids_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_server_quota_quotapb_ids_proto_rawDescData []byte
)

func file_go_chromium_org_luci_server_quota_quotapb_ids_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_server_quota_quotapb_ids_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_server_quota_quotapb_ids_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_server_quota_quotapb_ids_proto_rawDesc), len(file_go_chromium_org_luci_server_quota_quotapb_ids_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_server_quota_quotapb_ids_proto_rawDescData
}

var file_go_chromium_org_luci_server_quota_quotapb_ids_proto_msgTypes = make([]protoimpl.MessageInfo, 5)
var file_go_chromium_org_luci_server_quota_quotapb_ids_proto_goTypes = []any{
	(*PolicyConfigID)(nil), // 0: go.chromium.org.luci.server.quota.quotapb.PolicyConfigID
	(*PolicyID)(nil),       // 1: go.chromium.org.luci.server.quota.quotapb.PolicyID
	(*PolicyRef)(nil),      // 2: go.chromium.org.luci.server.quota.quotapb.PolicyRef
	(*PolicyKey)(nil),      // 3: go.chromium.org.luci.server.quota.quotapb.PolicyKey
	(*AccountID)(nil),      // 4: go.chromium.org.luci.server.quota.quotapb.AccountID
}
var file_go_chromium_org_luci_server_quota_quotapb_ids_proto_depIdxs = []int32{
	0, // 0: go.chromium.org.luci.server.quota.quotapb.PolicyID.config:type_name -> go.chromium.org.luci.server.quota.quotapb.PolicyConfigID
	3, // 1: go.chromium.org.luci.server.quota.quotapb.PolicyID.key:type_name -> go.chromium.org.luci.server.quota.quotapb.PolicyKey
	2, // [2:2] is the sub-list for method output_type
	2, // [2:2] is the sub-list for method input_type
	2, // [2:2] is the sub-list for extension type_name
	2, // [2:2] is the sub-list for extension extendee
	0, // [0:2] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_server_quota_quotapb_ids_proto_init() }
func file_go_chromium_org_luci_server_quota_quotapb_ids_proto_init() {
	if File_go_chromium_org_luci_server_quota_quotapb_ids_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_server_quota_quotapb_ids_proto_rawDesc), len(file_go_chromium_org_luci_server_quota_quotapb_ids_proto_rawDesc)),
			NumEnums:      0,
			NumMessages:   5,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_server_quota_quotapb_ids_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_server_quota_quotapb_ids_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_server_quota_quotapb_ids_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_server_quota_quotapb_ids_proto = out.File
	file_go_chromium_org_luci_server_quota_quotapb_ids_proto_goTypes = nil
	file_go_chromium_org_luci_server_quota_quotapb_ids_proto_depIdxs = nil
}
