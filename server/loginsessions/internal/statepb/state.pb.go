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
// 	protoc-gen-go v1.28.0
// 	protoc        v3.17.3
// source: go.chromium.org/luci/server/loginsessions/internal/statepb/state.proto

package statepb

import (
	loginsessionspb "go.chromium.org/luci/auth/loginsessionspb"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// LoginSession is stored in the datastore.
//
// It is a superset of luci.auth.loginsessions.LoginSession from the public API
// that has additional internal fields.
type LoginSession struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Unique ID of the session, matches luci.auth.loginsessions.LoginSession.
	Id string `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	// Password protecting access to GetLoginSession RPC.
	Password []byte `protobuf:"bytes,2,opt,name=password,proto3" json:"password,omitempty"`
	// The current session state.
	State loginsessionspb.LoginSession_State `protobuf:"varint,3,opt,name=state,proto3,enum=luci.auth.loginsessions.LoginSession_State" json:"state,omitempty"`
	// When the session was created. Always populated.
	Created *timestamppb.Timestamp `protobuf:"bytes,4,opt,name=created,proto3" json:"created,omitempty"`
	// When the session will expire. Always populated.
	Expiry *timestamppb.Timestamp `protobuf:"bytes,5,opt,name=expiry,proto3" json:"expiry,omitempty"`
	// When the session moved to a final state. Populated for finished sessions.
	Completed *timestamppb.Timestamp `protobuf:"bytes,6,opt,name=completed,proto3" json:"completed,omitempty"`
	// Details provided in CreateLoginSessionRequest. Always populated.
	OauthClientId          string                           `protobuf:"bytes,7,opt,name=oauth_client_id,json=oauthClientId,proto3" json:"oauth_client_id,omitempty"`
	OauthScopes            []string                         `protobuf:"bytes,8,rep,name=oauth_scopes,json=oauthScopes,proto3" json:"oauth_scopes,omitempty"`
	OauthS256CodeChallenge string                           `protobuf:"bytes,9,opt,name=oauth_s256_code_challenge,json=oauthS256CodeChallenge,proto3" json:"oauth_s256_code_challenge,omitempty"`
	ExecutableName         string                           `protobuf:"bytes,10,opt,name=executable_name,json=executableName,proto3" json:"executable_name,omitempty"`
	ClientHostname         string                           `protobuf:"bytes,11,opt,name=client_hostname,json=clientHostname,proto3" json:"client_hostname,omitempty"`
	ConfirmationCodes      []*LoginSession_ConfirmationCode `protobuf:"bytes,12,rep,name=confirmation_codes,json=confirmationCodes,proto3" json:"confirmation_codes,omitempty"`
	// The outcome of the protocol.
	OauthAuthorizationCode string `protobuf:"bytes,13,opt,name=oauth_authorization_code,json=oauthAuthorizationCode,proto3" json:"oauth_authorization_code,omitempty"`
	OauthRedirectUrl       string `protobuf:"bytes,14,opt,name=oauth_redirect_url,json=oauthRedirectUrl,proto3" json:"oauth_redirect_url,omitempty"`
	OauthError             string `protobuf:"bytes,15,opt,name=oauth_error,json=oauthError,proto3" json:"oauth_error,omitempty"`
}

func (x *LoginSession) Reset() {
	*x = LoginSession{}
	if protoimpl.UnsafeEnabled {
		mi := &file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *LoginSession) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*LoginSession) ProtoMessage() {}

func (x *LoginSession) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use LoginSession.ProtoReflect.Descriptor instead.
func (*LoginSession) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_rawDescGZIP(), []int{0}
}

func (x *LoginSession) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *LoginSession) GetPassword() []byte {
	if x != nil {
		return x.Password
	}
	return nil
}

func (x *LoginSession) GetState() loginsessionspb.LoginSession_State {
	if x != nil {
		return x.State
	}
	return loginsessionspb.LoginSession_State(0)
}

func (x *LoginSession) GetCreated() *timestamppb.Timestamp {
	if x != nil {
		return x.Created
	}
	return nil
}

func (x *LoginSession) GetExpiry() *timestamppb.Timestamp {
	if x != nil {
		return x.Expiry
	}
	return nil
}

func (x *LoginSession) GetCompleted() *timestamppb.Timestamp {
	if x != nil {
		return x.Completed
	}
	return nil
}

func (x *LoginSession) GetOauthClientId() string {
	if x != nil {
		return x.OauthClientId
	}
	return ""
}

func (x *LoginSession) GetOauthScopes() []string {
	if x != nil {
		return x.OauthScopes
	}
	return nil
}

func (x *LoginSession) GetOauthS256CodeChallenge() string {
	if x != nil {
		return x.OauthS256CodeChallenge
	}
	return ""
}

func (x *LoginSession) GetExecutableName() string {
	if x != nil {
		return x.ExecutableName
	}
	return ""
}

func (x *LoginSession) GetClientHostname() string {
	if x != nil {
		return x.ClientHostname
	}
	return ""
}

func (x *LoginSession) GetConfirmationCodes() []*LoginSession_ConfirmationCode {
	if x != nil {
		return x.ConfirmationCodes
	}
	return nil
}

func (x *LoginSession) GetOauthAuthorizationCode() string {
	if x != nil {
		return x.OauthAuthorizationCode
	}
	return ""
}

func (x *LoginSession) GetOauthRedirectUrl() string {
	if x != nil {
		return x.OauthRedirectUrl
	}
	return ""
}

func (x *LoginSession) GetOauthError() string {
	if x != nil {
		return x.OauthError
	}
	return ""
}

// OpenIDState is encrypted and used as `state` in OpenID Connect protocol.
type OpenIDState struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	LoginSessionId   string `protobuf:"bytes,1,opt,name=login_session_id,json=loginSessionId,proto3" json:"login_session_id,omitempty"`
	LoginCookieValue string `protobuf:"bytes,2,opt,name=login_cookie_value,json=loginCookieValue,proto3" json:"login_cookie_value,omitempty"`
}

func (x *OpenIDState) Reset() {
	*x = OpenIDState{}
	if protoimpl.UnsafeEnabled {
		mi := &file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *OpenIDState) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*OpenIDState) ProtoMessage() {}

func (x *OpenIDState) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use OpenIDState.ProtoReflect.Descriptor instead.
func (*OpenIDState) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_rawDescGZIP(), []int{1}
}

func (x *OpenIDState) GetLoginSessionId() string {
	if x != nil {
		return x.LoginSessionId
	}
	return ""
}

func (x *OpenIDState) GetLoginCookieValue() string {
	if x != nil {
		return x.LoginCookieValue
	}
	return ""
}

// Active (non-expired) confirmation codes.
type LoginSession_ConfirmationCode struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Code    string                 `protobuf:"bytes,1,opt,name=code,proto3" json:"code,omitempty"`
	Expiry  *timestamppb.Timestamp `protobuf:"bytes,2,opt,name=expiry,proto3" json:"expiry,omitempty"`
	Refresh *timestamppb.Timestamp `protobuf:"bytes,3,opt,name=refresh,proto3" json:"refresh,omitempty"`
}

func (x *LoginSession_ConfirmationCode) Reset() {
	*x = LoginSession_ConfirmationCode{}
	if protoimpl.UnsafeEnabled {
		mi := &file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *LoginSession_ConfirmationCode) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*LoginSession_ConfirmationCode) ProtoMessage() {}

func (x *LoginSession_ConfirmationCode) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use LoginSession_ConfirmationCode.ProtoReflect.Descriptor instead.
func (*LoginSession_ConfirmationCode) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_rawDescGZIP(), []int{0, 0}
}

func (x *LoginSession_ConfirmationCode) GetCode() string {
	if x != nil {
		return x.Code
	}
	return ""
}

func (x *LoginSession_ConfirmationCode) GetExpiry() *timestamppb.Timestamp {
	if x != nil {
		return x.Expiry
	}
	return nil
}

func (x *LoginSession_ConfirmationCode) GetRefresh() *timestamppb.Timestamp {
	if x != nil {
		return x.Refresh
	}
	return nil
}

var File_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_rawDesc = []byte{
	0x0a, 0x46, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2f, 0x6c, 0x6f,
	0x67, 0x69, 0x6e, 0x73, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x73, 0x2f, 0x69, 0x6e, 0x74, 0x65,
	0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x73, 0x74, 0x61, 0x74, 0x65, 0x70, 0x62, 0x2f, 0x73, 0x74, 0x61,
	0x74, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x19, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x73,
	0x65, 0x72, 0x76, 0x65, 0x72, 0x2e, 0x6c, 0x6f, 0x67, 0x69, 0x6e, 0x73, 0x65, 0x73, 0x73, 0x69,
	0x6f, 0x6e, 0x73, 0x1a, 0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x62, 0x75, 0x66, 0x2f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x37, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75,
	0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x61, 0x75, 0x74, 0x68, 0x2f,
	0x6c, 0x6f, 0x67, 0x69, 0x6e, 0x73, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x73, 0x70, 0x62, 0x2f,
	0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0xfe, 0x06,
	0x0a, 0x0c, 0x4c, 0x6f, 0x67, 0x69, 0x6e, 0x53, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x12, 0x0e,
	0x0a, 0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x69, 0x64, 0x12, 0x1a,
	0x0a, 0x08, 0x70, 0x61, 0x73, 0x73, 0x77, 0x6f, 0x72, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c,
	0x52, 0x08, 0x70, 0x61, 0x73, 0x73, 0x77, 0x6f, 0x72, 0x64, 0x12, 0x41, 0x0a, 0x05, 0x73, 0x74,
	0x61, 0x74, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x2b, 0x2e, 0x6c, 0x75, 0x63, 0x69,
	0x2e, 0x61, 0x75, 0x74, 0x68, 0x2e, 0x6c, 0x6f, 0x67, 0x69, 0x6e, 0x73, 0x65, 0x73, 0x73, 0x69,
	0x6f, 0x6e, 0x73, 0x2e, 0x4c, 0x6f, 0x67, 0x69, 0x6e, 0x53, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e,
	0x2e, 0x53, 0x74, 0x61, 0x74, 0x65, 0x52, 0x05, 0x73, 0x74, 0x61, 0x74, 0x65, 0x12, 0x34, 0x0a,
	0x07, 0x63, 0x72, 0x65, 0x61, 0x74, 0x65, 0x64, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a,
	0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66,
	0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x52, 0x07, 0x63, 0x72, 0x65, 0x61,
	0x74, 0x65, 0x64, 0x12, 0x32, 0x0a, 0x06, 0x65, 0x78, 0x70, 0x69, 0x72, 0x79, 0x18, 0x05, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x52,
	0x06, 0x65, 0x78, 0x70, 0x69, 0x72, 0x79, 0x12, 0x38, 0x0a, 0x09, 0x63, 0x6f, 0x6d, 0x70, 0x6c,
	0x65, 0x74, 0x65, 0x64, 0x18, 0x06, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f,
	0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d,
	0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x52, 0x09, 0x63, 0x6f, 0x6d, 0x70, 0x6c, 0x65, 0x74, 0x65,
	0x64, 0x12, 0x26, 0x0a, 0x0f, 0x6f, 0x61, 0x75, 0x74, 0x68, 0x5f, 0x63, 0x6c, 0x69, 0x65, 0x6e,
	0x74, 0x5f, 0x69, 0x64, 0x18, 0x07, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0d, 0x6f, 0x61, 0x75, 0x74,
	0x68, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x49, 0x64, 0x12, 0x21, 0x0a, 0x0c, 0x6f, 0x61, 0x75,
	0x74, 0x68, 0x5f, 0x73, 0x63, 0x6f, 0x70, 0x65, 0x73, 0x18, 0x08, 0x20, 0x03, 0x28, 0x09, 0x52,
	0x0b, 0x6f, 0x61, 0x75, 0x74, 0x68, 0x53, 0x63, 0x6f, 0x70, 0x65, 0x73, 0x12, 0x39, 0x0a, 0x19,
	0x6f, 0x61, 0x75, 0x74, 0x68, 0x5f, 0x73, 0x32, 0x35, 0x36, 0x5f, 0x63, 0x6f, 0x64, 0x65, 0x5f,
	0x63, 0x68, 0x61, 0x6c, 0x6c, 0x65, 0x6e, 0x67, 0x65, 0x18, 0x09, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x16, 0x6f, 0x61, 0x75, 0x74, 0x68, 0x53, 0x32, 0x35, 0x36, 0x43, 0x6f, 0x64, 0x65, 0x43, 0x68,
	0x61, 0x6c, 0x6c, 0x65, 0x6e, 0x67, 0x65, 0x12, 0x27, 0x0a, 0x0f, 0x65, 0x78, 0x65, 0x63, 0x75,
	0x74, 0x61, 0x62, 0x6c, 0x65, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x0a, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x0e, 0x65, 0x78, 0x65, 0x63, 0x75, 0x74, 0x61, 0x62, 0x6c, 0x65, 0x4e, 0x61, 0x6d, 0x65,
	0x12, 0x27, 0x0a, 0x0f, 0x63, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x5f, 0x68, 0x6f, 0x73, 0x74, 0x6e,
	0x61, 0x6d, 0x65, 0x18, 0x0b, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0e, 0x63, 0x6c, 0x69, 0x65, 0x6e,
	0x74, 0x48, 0x6f, 0x73, 0x74, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x67, 0x0a, 0x12, 0x63, 0x6f, 0x6e,
	0x66, 0x69, 0x72, 0x6d, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x63, 0x6f, 0x64, 0x65, 0x73, 0x18,
	0x0c, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x38, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x73, 0x65, 0x72,
	0x76, 0x65, 0x72, 0x2e, 0x6c, 0x6f, 0x67, 0x69, 0x6e, 0x73, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e,
	0x73, 0x2e, 0x4c, 0x6f, 0x67, 0x69, 0x6e, 0x53, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x2e, 0x43,
	0x6f, 0x6e, 0x66, 0x69, 0x72, 0x6d, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x43, 0x6f, 0x64, 0x65, 0x52,
	0x11, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x72, 0x6d, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x43, 0x6f, 0x64,
	0x65, 0x73, 0x12, 0x38, 0x0a, 0x18, 0x6f, 0x61, 0x75, 0x74, 0x68, 0x5f, 0x61, 0x75, 0x74, 0x68,
	0x6f, 0x72, 0x69, 0x7a, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x63, 0x6f, 0x64, 0x65, 0x18, 0x0d,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x16, 0x6f, 0x61, 0x75, 0x74, 0x68, 0x41, 0x75, 0x74, 0x68, 0x6f,
	0x72, 0x69, 0x7a, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x43, 0x6f, 0x64, 0x65, 0x12, 0x2c, 0x0a, 0x12,
	0x6f, 0x61, 0x75, 0x74, 0x68, 0x5f, 0x72, 0x65, 0x64, 0x69, 0x72, 0x65, 0x63, 0x74, 0x5f, 0x75,
	0x72, 0x6c, 0x18, 0x0e, 0x20, 0x01, 0x28, 0x09, 0x52, 0x10, 0x6f, 0x61, 0x75, 0x74, 0x68, 0x52,
	0x65, 0x64, 0x69, 0x72, 0x65, 0x63, 0x74, 0x55, 0x72, 0x6c, 0x12, 0x1f, 0x0a, 0x0b, 0x6f, 0x61,
	0x75, 0x74, 0x68, 0x5f, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x18, 0x0f, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x0a, 0x6f, 0x61, 0x75, 0x74, 0x68, 0x45, 0x72, 0x72, 0x6f, 0x72, 0x1a, 0x90, 0x01, 0x0a, 0x10,
	0x43, 0x6f, 0x6e, 0x66, 0x69, 0x72, 0x6d, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x43, 0x6f, 0x64, 0x65,
	0x12, 0x12, 0x0a, 0x04, 0x63, 0x6f, 0x64, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04,
	0x63, 0x6f, 0x64, 0x65, 0x12, 0x32, 0x0a, 0x06, 0x65, 0x78, 0x70, 0x69, 0x72, 0x79, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70,
	0x52, 0x06, 0x65, 0x78, 0x70, 0x69, 0x72, 0x79, 0x12, 0x34, 0x0a, 0x07, 0x72, 0x65, 0x66, 0x72,
	0x65, 0x73, 0x68, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65,
	0x73, 0x74, 0x61, 0x6d, 0x70, 0x52, 0x07, 0x72, 0x65, 0x66, 0x72, 0x65, 0x73, 0x68, 0x22, 0x65,
	0x0a, 0x0b, 0x4f, 0x70, 0x65, 0x6e, 0x49, 0x44, 0x53, 0x74, 0x61, 0x74, 0x65, 0x12, 0x28, 0x0a,
	0x10, 0x6c, 0x6f, 0x67, 0x69, 0x6e, 0x5f, 0x73, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x5f, 0x69,
	0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0e, 0x6c, 0x6f, 0x67, 0x69, 0x6e, 0x53, 0x65,
	0x73, 0x73, 0x69, 0x6f, 0x6e, 0x49, 0x64, 0x12, 0x2c, 0x0a, 0x12, 0x6c, 0x6f, 0x67, 0x69, 0x6e,
	0x5f, 0x63, 0x6f, 0x6f, 0x6b, 0x69, 0x65, 0x5f, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x10, 0x6c, 0x6f, 0x67, 0x69, 0x6e, 0x43, 0x6f, 0x6f, 0x6b, 0x69, 0x65,
	0x56, 0x61, 0x6c, 0x75, 0x65, 0x42, 0x3c, 0x5a, 0x3a, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f,
	0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x73, 0x65,
	0x72, 0x76, 0x65, 0x72, 0x2f, 0x6c, 0x6f, 0x67, 0x69, 0x6e, 0x73, 0x65, 0x73, 0x73, 0x69, 0x6f,
	0x6e, 0x73, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x73, 0x74, 0x61, 0x74,
	0x65, 0x70, 0x62, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_rawDescData = file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_rawDesc
)

func file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_rawDescData = protoimpl.X.CompressGZIP(file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_rawDescData)
	})
	return file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_rawDescData
}

var file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_goTypes = []interface{}{
	(*LoginSession)(nil),                    // 0: luci.server.loginsessions.LoginSession
	(*OpenIDState)(nil),                     // 1: luci.server.loginsessions.OpenIDState
	(*LoginSession_ConfirmationCode)(nil),   // 2: luci.server.loginsessions.LoginSession.ConfirmationCode
	(loginsessionspb.LoginSession_State)(0), // 3: luci.auth.loginsessions.LoginSession.State
	(*timestamppb.Timestamp)(nil),           // 4: google.protobuf.Timestamp
}
var file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_depIdxs = []int32{
	3, // 0: luci.server.loginsessions.LoginSession.state:type_name -> luci.auth.loginsessions.LoginSession.State
	4, // 1: luci.server.loginsessions.LoginSession.created:type_name -> google.protobuf.Timestamp
	4, // 2: luci.server.loginsessions.LoginSession.expiry:type_name -> google.protobuf.Timestamp
	4, // 3: luci.server.loginsessions.LoginSession.completed:type_name -> google.protobuf.Timestamp
	2, // 4: luci.server.loginsessions.LoginSession.confirmation_codes:type_name -> luci.server.loginsessions.LoginSession.ConfirmationCode
	4, // 5: luci.server.loginsessions.LoginSession.ConfirmationCode.expiry:type_name -> google.protobuf.Timestamp
	4, // 6: luci.server.loginsessions.LoginSession.ConfirmationCode.refresh:type_name -> google.protobuf.Timestamp
	7, // [7:7] is the sub-list for method output_type
	7, // [7:7] is the sub-list for method input_type
	7, // [7:7] is the sub-list for extension type_name
	7, // [7:7] is the sub-list for extension extendee
	0, // [0:7] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_init() }
func file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_init() {
	if File_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*LoginSession); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*OpenIDState); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*LoginSession_ConfirmationCode); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_depIdxs,
		MessageInfos:      file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto = out.File
	file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_rawDesc = nil
	file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_goTypes = nil
	file_go_chromium_org_luci_server_loginsessions_internal_statepb_state_proto_depIdxs = nil
}