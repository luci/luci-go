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
// source: go.chromium.org/luci/auth/loginsessionspb/service.proto

package loginsessionspb

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
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

// A session starts in PENDING state and then moves to one of other states
// (all of them are final) in response to user actions or passage of time.
type LoginSession_State int32

const (
	LoginSession_STATE_UNSPECIFIED LoginSession_State = 0
	LoginSession_PENDING           LoginSession_State = 1
	LoginSession_CANCELED          LoginSession_State = 2
	LoginSession_SUCCEEDED         LoginSession_State = 3
	LoginSession_FAILED            LoginSession_State = 4
	LoginSession_EXPIRED           LoginSession_State = 5
)

// Enum value maps for LoginSession_State.
var (
	LoginSession_State_name = map[int32]string{
		0: "STATE_UNSPECIFIED",
		1: "PENDING",
		2: "CANCELED",
		3: "SUCCEEDED",
		4: "FAILED",
		5: "EXPIRED",
	}
	LoginSession_State_value = map[string]int32{
		"STATE_UNSPECIFIED": 0,
		"PENDING":           1,
		"CANCELED":          2,
		"SUCCEEDED":         3,
		"FAILED":            4,
		"EXPIRED":           5,
	}
)

func (x LoginSession_State) Enum() *LoginSession_State {
	p := new(LoginSession_State)
	*p = x
	return p
}

func (x LoginSession_State) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (LoginSession_State) Descriptor() protoreflect.EnumDescriptor {
	return file_go_chromium_org_luci_auth_loginsessionspb_service_proto_enumTypes[0].Descriptor()
}

func (LoginSession_State) Type() protoreflect.EnumType {
	return &file_go_chromium_org_luci_auth_loginsessionspb_service_proto_enumTypes[0]
}

func (x LoginSession_State) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use LoginSession_State.Descriptor instead.
func (LoginSession_State) EnumDescriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_auth_loginsessionspb_service_proto_rawDescGZIP(), []int{2, 0}
}

// Inputs for CreateLoginSession
type CreateLoginSessionRequest struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// An OAuth2 client ID that should be known to the login sessions server.
	//
	// The eventual outcome of the login protocol is a set of tokens associated
	// with this OAuth2 client (e.g. the ID token will have this client as
	// `aud` claim).
	//
	// This client ID also identifies the application information that the user
	// will see at the OAuth2 consent screen.
	//
	// Required.
	OauthClientId string `protobuf:"bytes,1,opt,name=oauth_client_id,json=oauthClientId,proto3" json:"oauth_client_id,omitempty"`
	// A list of OAuth2 scopes to get the refresh and access tokens with.
	//
	// The server may deny usage of some sensitive scopes. This set of scopes
	// defined what the user will see at the OAuth2 consent screen.
	//
	// Required.
	OauthScopes []string `protobuf:"bytes,2,rep,name=oauth_scopes,json=oauthScopes,proto3" json:"oauth_scopes,omitempty"`
	// A `code_challenge` parameter for PKCE protocol using S256 method.
	//
	// See https://tools.ietf.org/html/rfc7636. It should be a base64 URL-encoded
	// SHA256 digest of a `code_verifier` random string (that the caller should
	// not disclose anywhere).
	//
	// Required.
	OauthS256CodeChallenge string `protobuf:"bytes,3,opt,name=oauth_s256_code_challenge,json=oauthS256CodeChallenge,proto3" json:"oauth_s256_code_challenge,omitempty"`
	// A name of the native program that started the flow.
	//
	// Will be shown on the confirmation web page in the login session UI to
	// provide some best-effort context around what opened the login session.
	// It is **not a security mechanism**, just an FYI for the user.
	//
	// Optional.
	ExecutableName string `protobuf:"bytes,4,opt,name=executable_name,json=executableName,proto3" json:"executable_name,omitempty"`
	// A hostname of the machine that started the flow.
	//
	// Used for the same purpose as `executable_name` to give some context around
	// what opened the login session. It is **not a security mechanism**, just
	// an FYI for the user.
	//
	// Optional.
	ClientHostname string `protobuf:"bytes,5,opt,name=client_hostname,json=clientHostname,proto3" json:"client_hostname,omitempty"`
	unknownFields  protoimpl.UnknownFields
	sizeCache      protoimpl.SizeCache
}

func (x *CreateLoginSessionRequest) Reset() {
	*x = CreateLoginSessionRequest{}
	mi := &file_go_chromium_org_luci_auth_loginsessionspb_service_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *CreateLoginSessionRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CreateLoginSessionRequest) ProtoMessage() {}

func (x *CreateLoginSessionRequest) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_auth_loginsessionspb_service_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CreateLoginSessionRequest.ProtoReflect.Descriptor instead.
func (*CreateLoginSessionRequest) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_auth_loginsessionspb_service_proto_rawDescGZIP(), []int{0}
}

func (x *CreateLoginSessionRequest) GetOauthClientId() string {
	if x != nil {
		return x.OauthClientId
	}
	return ""
}

func (x *CreateLoginSessionRequest) GetOauthScopes() []string {
	if x != nil {
		return x.OauthScopes
	}
	return nil
}

func (x *CreateLoginSessionRequest) GetOauthS256CodeChallenge() string {
	if x != nil {
		return x.OauthS256CodeChallenge
	}
	return ""
}

func (x *CreateLoginSessionRequest) GetExecutableName() string {
	if x != nil {
		return x.ExecutableName
	}
	return ""
}

func (x *CreateLoginSessionRequest) GetClientHostname() string {
	if x != nil {
		return x.ClientHostname
	}
	return ""
}

// Inputs for GetLoginSession.
type GetLoginSessionRequest struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// ID of the login session to get the state of. Required.
	LoginSessionId string `protobuf:"bytes,1,opt,name=login_session_id,json=loginSessionId,proto3" json:"login_session_id,omitempty"`
	// The password returned by CreateLoginSession. Required.
	LoginSessionPassword []byte `protobuf:"bytes,2,opt,name=login_session_password,json=loginSessionPassword,proto3" json:"login_session_password,omitempty"`
	unknownFields        protoimpl.UnknownFields
	sizeCache            protoimpl.SizeCache
}

func (x *GetLoginSessionRequest) Reset() {
	*x = GetLoginSessionRequest{}
	mi := &file_go_chromium_org_luci_auth_loginsessionspb_service_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetLoginSessionRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetLoginSessionRequest) ProtoMessage() {}

func (x *GetLoginSessionRequest) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_auth_loginsessionspb_service_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetLoginSessionRequest.ProtoReflect.Descriptor instead.
func (*GetLoginSessionRequest) Descriptor() ([]byte, []int) {
	return file_go_chromium_org_luci_auth_loginsessionspb_service_proto_rawDescGZIP(), []int{1}
}

func (x *GetLoginSessionRequest) GetLoginSessionId() string {
	if x != nil {
		return x.LoginSessionId
	}
	return ""
}

func (x *GetLoginSessionRequest) GetLoginSessionPassword() []byte {
	if x != nil {
		return x.LoginSessionPassword
	}
	return nil
}

// Represents a login session whose eventual outcome if an OAuth2 authorization
// code.
type LoginSession struct {
	state protoimpl.MessageState `protogen:"open.v1"`
	// Globally identifies this session.
	//
	// It is a randomly generated URL-safe string. Knowing it is enough to
	// complete the login session via the web UI. Should be used only by the user
	// that started the login flow.
	//
	// It will also appear as a `nonce` claim in the ID token produced by the
	// protocol.
	Id string `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	// Password is required to call GetLoginSession.
	//
	// It is populated only in the response from CreateLoginSession. It exists
	// to make sure that only whoever created the session can check its status.
	// Must not be shared or stored.
	Password []byte             `protobuf:"bytes,2,opt,name=password,proto3" json:"password,omitempty"`
	State    LoginSession_State `protobuf:"varint,3,opt,name=state,proto3,enum=luci.auth.loginsessions.LoginSession_State" json:"state,omitempty"`
	// When the session was created. Always populated.
	Created *timestamppb.Timestamp `protobuf:"bytes,4,opt,name=created,proto3" json:"created,omitempty"`
	// When the session will expire. Always populated.
	Expiry *timestamppb.Timestamp `protobuf:"bytes,5,opt,name=expiry,proto3" json:"expiry,omitempty"`
	// When the session moved to a final state. Populated for finished sessions.
	Completed *timestamppb.Timestamp `protobuf:"bytes,6,opt,name=completed,proto3" json:"completed,omitempty"`
	// A full URL to a webpage the user should visit to perform the login flow.
	//
	// It encodes `id` inside. Always populated.
	//
	// Knowing it is enough to complete the login session via the web UI. Should
	// be used only by the user that started the login flow.
	LoginFlowUrl string `protobuf:"bytes,7,opt,name=login_flow_url,json=loginFlowUrl,proto3" json:"login_flow_url,omitempty"`
	// How often the caller should poll the session status via GetLoginSession.
	//
	// It is a mechanism to adjust the global poll rate without redeploying
	// new clients.
	//
	// Populated for sessions in PENDING state. The caller is allowed to ignore it
	// if absolutely necessary.
	PollInterval *durationpb.Duration `protobuf:"bytes,8,opt,name=poll_interval,json=pollInterval,proto3" json:"poll_interval,omitempty"`
	// The active confirmation code.
	//
	// The user will be asked to provide this code by the web UI as the final step
	// of the login flow. The code should be shown to the user by the native
	// program in the terminal. This code is very short lived (~ 1 min) and the
	// native program should periodically fetch and show the most recent code.
	//
	// The purpose of this mechanism is to make sure the user is completing the
	// flow they have actually started in their own terminal. It makes phishing
	// attempts harder, since the target of a phishing attack should not only
	// click through the web UI login flow initiated from a link (which is
	// relatively easy to arrange), but also actively copy-paste an up-to-date
	// code that expires very fast (making "asynchronous" phishing attempts
	// relatively hard to perform).
	//
	// Populated only if the session is still in PENDING state.
	ConfirmationCode string `protobuf:"bytes,9,opt,name=confirmation_code,json=confirmationCode,proto3" json:"confirmation_code,omitempty"`
	// When the confirmation code expires, as duration since when the request to
	// get it completed.
	//
	// It is a relative time (instead of an absolute timestamp) to avoid relying
	// on clock synchronization between the backend and the client machine. Since
	// the code expires pretty fast, even small differences in clocks may cause
	// issues.
	//
	// This value is always sufficiently larger than zero (to give the user some
	// time to use it). The server will prepare a new code in advance if the
	// existing one expires soon. See confirmation_code_refresh below. During such
	// transitions both codes are valid.
	//
	// Populated only if the session is still in PENDING state.
	ConfirmationCodeExpiry *durationpb.Duration `protobuf:"bytes,10,opt,name=confirmation_code_expiry,json=confirmationCodeExpiry,proto3" json:"confirmation_code_expiry,omitempty"`
	// When the confirmation code will be refreshed (approximately).
	//
	// A "refresh" in this context means GetLoginSession will start returning
	// a new code. It happens somewhat before the previous code expires. That way
	// the user always sees a code that is sufficiently fresh to be copy-pasted
	// into the confirmation web page in a leisurely pace.
	//
	// Populated only if the session is still in PENDING state.
	ConfirmationCodeRefresh *durationpb.Duration `protobuf:"bytes,11,opt,name=confirmation_code_refresh,json=confirmationCodeRefresh,proto3" json:"confirmation_code_refresh,omitempty"`
	// The OAuth2 authorization code that can be exchanged for OAuth2 tokens.
	//
	// Populated only for sessions in SUCCEEDED state. Getting this code is the
	// goal of LoginSessions service. Knowing this code, an OAuth2 client secret
	// (which is usually hardcoded in the native program code) and the PKCE code
	// verifier secret (which was used to derive `oauth_s256_code_challenge`) is
	// enough to get all OAuth2 tokens.
	//
	// Must not be shared.
	OauthAuthorizationCode string `protobuf:"bytes,12,opt,name=oauth_authorization_code,json=oauthAuthorizationCode,proto3" json:"oauth_authorization_code,omitempty"`
	// An URL that should be used as `redirect_url` parameter when calling the
	// authorization server token endpoint when exchanging the authorization code
	// for tokens.
	//
	// Populated only for sessions in SUCCEEDED state. It is usually a static
	// well-known URL pointing to a page on the login sessions service domain,
	// but it is returned with the session to avoid hardcoding dependencies on
	// implementation details of the login sessions server.
	OauthRedirectUrl string `protobuf:"bytes,13,opt,name=oauth_redirect_url,json=oauthRedirectUrl,proto3" json:"oauth_redirect_url,omitempty"`
	// An optional error message if the login flow failed.
	//
	// Populated only for sessions in FAILED state.
	OauthError    string `protobuf:"bytes,14,opt,name=oauth_error,json=oauthError,proto3" json:"oauth_error,omitempty"`
	unknownFields protoimpl.UnknownFields
	sizeCache     protoimpl.SizeCache
}

func (x *LoginSession) Reset() {
	*x = LoginSession{}
	mi := &file_go_chromium_org_luci_auth_loginsessionspb_service_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *LoginSession) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*LoginSession) ProtoMessage() {}

func (x *LoginSession) ProtoReflect() protoreflect.Message {
	mi := &file_go_chromium_org_luci_auth_loginsessionspb_service_proto_msgTypes[2]
	if x != nil {
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
	return file_go_chromium_org_luci_auth_loginsessionspb_service_proto_rawDescGZIP(), []int{2}
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

func (x *LoginSession) GetState() LoginSession_State {
	if x != nil {
		return x.State
	}
	return LoginSession_STATE_UNSPECIFIED
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

func (x *LoginSession) GetLoginFlowUrl() string {
	if x != nil {
		return x.LoginFlowUrl
	}
	return ""
}

func (x *LoginSession) GetPollInterval() *durationpb.Duration {
	if x != nil {
		return x.PollInterval
	}
	return nil
}

func (x *LoginSession) GetConfirmationCode() string {
	if x != nil {
		return x.ConfirmationCode
	}
	return ""
}

func (x *LoginSession) GetConfirmationCodeExpiry() *durationpb.Duration {
	if x != nil {
		return x.ConfirmationCodeExpiry
	}
	return nil
}

func (x *LoginSession) GetConfirmationCodeRefresh() *durationpb.Duration {
	if x != nil {
		return x.ConfirmationCodeRefresh
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

var File_go_chromium_org_luci_auth_loginsessionspb_service_proto protoreflect.FileDescriptor

var file_go_chromium_org_luci_auth_loginsessionspb_service_proto_rawDesc = string([]byte{
	0x0a, 0x37, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e, 0x6f, 0x72,
	0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x61, 0x75, 0x74, 0x68, 0x2f, 0x6c, 0x6f, 0x67, 0x69,
	0x6e, 0x73, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x73, 0x70, 0x62, 0x2f, 0x73, 0x65, 0x72, 0x76,
	0x69, 0x63, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x17, 0x6c, 0x75, 0x63, 0x69, 0x2e,
	0x61, 0x75, 0x74, 0x68, 0x2e, 0x6c, 0x6f, 0x67, 0x69, 0x6e, 0x73, 0x65, 0x73, 0x73, 0x69, 0x6f,
	0x6e, 0x73, 0x1a, 0x1e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x62, 0x75, 0x66, 0x2f, 0x64, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x1a, 0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x62, 0x75, 0x66, 0x2f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x22, 0xf3, 0x01, 0x0a, 0x19, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x4c, 0x6f,
	0x67, 0x69, 0x6e, 0x53, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x12, 0x26, 0x0a, 0x0f, 0x6f, 0x61, 0x75, 0x74, 0x68, 0x5f, 0x63, 0x6c, 0x69, 0x65, 0x6e,
	0x74, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0d, 0x6f, 0x61, 0x75, 0x74,
	0x68, 0x43, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x49, 0x64, 0x12, 0x21, 0x0a, 0x0c, 0x6f, 0x61, 0x75,
	0x74, 0x68, 0x5f, 0x73, 0x63, 0x6f, 0x70, 0x65, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28, 0x09, 0x52,
	0x0b, 0x6f, 0x61, 0x75, 0x74, 0x68, 0x53, 0x63, 0x6f, 0x70, 0x65, 0x73, 0x12, 0x39, 0x0a, 0x19,
	0x6f, 0x61, 0x75, 0x74, 0x68, 0x5f, 0x73, 0x32, 0x35, 0x36, 0x5f, 0x63, 0x6f, 0x64, 0x65, 0x5f,
	0x63, 0x68, 0x61, 0x6c, 0x6c, 0x65, 0x6e, 0x67, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x16, 0x6f, 0x61, 0x75, 0x74, 0x68, 0x53, 0x32, 0x35, 0x36, 0x43, 0x6f, 0x64, 0x65, 0x43, 0x68,
	0x61, 0x6c, 0x6c, 0x65, 0x6e, 0x67, 0x65, 0x12, 0x27, 0x0a, 0x0f, 0x65, 0x78, 0x65, 0x63, 0x75,
	0x74, 0x61, 0x62, 0x6c, 0x65, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x0e, 0x65, 0x78, 0x65, 0x63, 0x75, 0x74, 0x61, 0x62, 0x6c, 0x65, 0x4e, 0x61, 0x6d, 0x65,
	0x12, 0x27, 0x0a, 0x0f, 0x63, 0x6c, 0x69, 0x65, 0x6e, 0x74, 0x5f, 0x68, 0x6f, 0x73, 0x74, 0x6e,
	0x61, 0x6d, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0e, 0x63, 0x6c, 0x69, 0x65, 0x6e,
	0x74, 0x48, 0x6f, 0x73, 0x74, 0x6e, 0x61, 0x6d, 0x65, 0x22, 0x78, 0x0a, 0x16, 0x47, 0x65, 0x74,
	0x4c, 0x6f, 0x67, 0x69, 0x6e, 0x53, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x12, 0x28, 0x0a, 0x10, 0x6c, 0x6f, 0x67, 0x69, 0x6e, 0x5f, 0x73, 0x65, 0x73,
	0x73, 0x69, 0x6f, 0x6e, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0e, 0x6c,
	0x6f, 0x67, 0x69, 0x6e, 0x53, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x49, 0x64, 0x12, 0x34, 0x0a,
	0x16, 0x6c, 0x6f, 0x67, 0x69, 0x6e, 0x5f, 0x73, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x5f, 0x70,
	0x61, 0x73, 0x73, 0x77, 0x6f, 0x72, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x14, 0x6c,
	0x6f, 0x67, 0x69, 0x6e, 0x53, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x50, 0x61, 0x73, 0x73, 0x77,
	0x6f, 0x72, 0x64, 0x22, 0xcc, 0x06, 0x0a, 0x0c, 0x4c, 0x6f, 0x67, 0x69, 0x6e, 0x53, 0x65, 0x73,
	0x73, 0x69, 0x6f, 0x6e, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x02, 0x69, 0x64, 0x12, 0x1a, 0x0a, 0x08, 0x70, 0x61, 0x73, 0x73, 0x77, 0x6f, 0x72, 0x64,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x08, 0x70, 0x61, 0x73, 0x73, 0x77, 0x6f, 0x72, 0x64,
	0x12, 0x41, 0x0a, 0x05, 0x73, 0x74, 0x61, 0x74, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0e, 0x32,
	0x2b, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x61, 0x75, 0x74, 0x68, 0x2e, 0x6c, 0x6f, 0x67, 0x69,
	0x6e, 0x73, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x4c, 0x6f, 0x67, 0x69, 0x6e, 0x53,
	0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x2e, 0x53, 0x74, 0x61, 0x74, 0x65, 0x52, 0x05, 0x73, 0x74,
	0x61, 0x74, 0x65, 0x12, 0x34, 0x0a, 0x07, 0x63, 0x72, 0x65, 0x61, 0x74, 0x65, 0x64, 0x18, 0x04,
	0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70,
	0x52, 0x07, 0x63, 0x72, 0x65, 0x61, 0x74, 0x65, 0x64, 0x12, 0x32, 0x0a, 0x06, 0x65, 0x78, 0x70,
	0x69, 0x72, 0x79, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67,
	0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65,
	0x73, 0x74, 0x61, 0x6d, 0x70, 0x52, 0x06, 0x65, 0x78, 0x70, 0x69, 0x72, 0x79, 0x12, 0x38, 0x0a,
	0x09, 0x63, 0x6f, 0x6d, 0x70, 0x6c, 0x65, 0x74, 0x65, 0x64, 0x18, 0x06, 0x20, 0x01, 0x28, 0x0b,
	0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62,
	0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x52, 0x09, 0x63, 0x6f,
	0x6d, 0x70, 0x6c, 0x65, 0x74, 0x65, 0x64, 0x12, 0x24, 0x0a, 0x0e, 0x6c, 0x6f, 0x67, 0x69, 0x6e,
	0x5f, 0x66, 0x6c, 0x6f, 0x77, 0x5f, 0x75, 0x72, 0x6c, 0x18, 0x07, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x0c, 0x6c, 0x6f, 0x67, 0x69, 0x6e, 0x46, 0x6c, 0x6f, 0x77, 0x55, 0x72, 0x6c, 0x12, 0x3e, 0x0a,
	0x0d, 0x70, 0x6f, 0x6c, 0x6c, 0x5f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x76, 0x61, 0x6c, 0x18, 0x08,
	0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52,
	0x0c, 0x70, 0x6f, 0x6c, 0x6c, 0x49, 0x6e, 0x74, 0x65, 0x72, 0x76, 0x61, 0x6c, 0x12, 0x2b, 0x0a,
	0x11, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x72, 0x6d, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x63, 0x6f,
	0x64, 0x65, 0x18, 0x09, 0x20, 0x01, 0x28, 0x09, 0x52, 0x10, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x72,
	0x6d, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x43, 0x6f, 0x64, 0x65, 0x12, 0x53, 0x0a, 0x18, 0x63, 0x6f,
	0x6e, 0x66, 0x69, 0x72, 0x6d, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x63, 0x6f, 0x64, 0x65, 0x5f,
	0x65, 0x78, 0x70, 0x69, 0x72, 0x79, 0x18, 0x0a, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44,
	0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x16, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x72, 0x6d,
	0x61, 0x74, 0x69, 0x6f, 0x6e, 0x43, 0x6f, 0x64, 0x65, 0x45, 0x78, 0x70, 0x69, 0x72, 0x79, 0x12,
	0x55, 0x0a, 0x19, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x72, 0x6d, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x5f,
	0x63, 0x6f, 0x64, 0x65, 0x5f, 0x72, 0x65, 0x66, 0x72, 0x65, 0x73, 0x68, 0x18, 0x0b, 0x20, 0x01,
	0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x52, 0x17, 0x63,
	0x6f, 0x6e, 0x66, 0x69, 0x72, 0x6d, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x43, 0x6f, 0x64, 0x65, 0x52,
	0x65, 0x66, 0x72, 0x65, 0x73, 0x68, 0x12, 0x38, 0x0a, 0x18, 0x6f, 0x61, 0x75, 0x74, 0x68, 0x5f,
	0x61, 0x75, 0x74, 0x68, 0x6f, 0x72, 0x69, 0x7a, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x63, 0x6f,
	0x64, 0x65, 0x18, 0x0c, 0x20, 0x01, 0x28, 0x09, 0x52, 0x16, 0x6f, 0x61, 0x75, 0x74, 0x68, 0x41,
	0x75, 0x74, 0x68, 0x6f, 0x72, 0x69, 0x7a, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x43, 0x6f, 0x64, 0x65,
	0x12, 0x2c, 0x0a, 0x12, 0x6f, 0x61, 0x75, 0x74, 0x68, 0x5f, 0x72, 0x65, 0x64, 0x69, 0x72, 0x65,
	0x63, 0x74, 0x5f, 0x75, 0x72, 0x6c, 0x18, 0x0d, 0x20, 0x01, 0x28, 0x09, 0x52, 0x10, 0x6f, 0x61,
	0x75, 0x74, 0x68, 0x52, 0x65, 0x64, 0x69, 0x72, 0x65, 0x63, 0x74, 0x55, 0x72, 0x6c, 0x12, 0x1f,
	0x0a, 0x0b, 0x6f, 0x61, 0x75, 0x74, 0x68, 0x5f, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x18, 0x0e, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x0a, 0x6f, 0x61, 0x75, 0x74, 0x68, 0x45, 0x72, 0x72, 0x6f, 0x72, 0x22,
	0x61, 0x0a, 0x05, 0x53, 0x74, 0x61, 0x74, 0x65, 0x12, 0x15, 0x0a, 0x11, 0x53, 0x54, 0x41, 0x54,
	0x45, 0x5f, 0x55, 0x4e, 0x53, 0x50, 0x45, 0x43, 0x49, 0x46, 0x49, 0x45, 0x44, 0x10, 0x00, 0x12,
	0x0b, 0x0a, 0x07, 0x50, 0x45, 0x4e, 0x44, 0x49, 0x4e, 0x47, 0x10, 0x01, 0x12, 0x0c, 0x0a, 0x08,
	0x43, 0x41, 0x4e, 0x43, 0x45, 0x4c, 0x45, 0x44, 0x10, 0x02, 0x12, 0x0d, 0x0a, 0x09, 0x53, 0x55,
	0x43, 0x43, 0x45, 0x45, 0x44, 0x45, 0x44, 0x10, 0x03, 0x12, 0x0a, 0x0a, 0x06, 0x46, 0x41, 0x49,
	0x4c, 0x45, 0x44, 0x10, 0x04, 0x12, 0x0b, 0x0a, 0x07, 0x45, 0x58, 0x50, 0x49, 0x52, 0x45, 0x44,
	0x10, 0x05, 0x32, 0xeb, 0x01, 0x0a, 0x0d, 0x4c, 0x6f, 0x67, 0x69, 0x6e, 0x53, 0x65, 0x73, 0x73,
	0x69, 0x6f, 0x6e, 0x73, 0x12, 0x6f, 0x0a, 0x12, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x4c, 0x6f,
	0x67, 0x69, 0x6e, 0x53, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x12, 0x32, 0x2e, 0x6c, 0x75, 0x63,
	0x69, 0x2e, 0x61, 0x75, 0x74, 0x68, 0x2e, 0x6c, 0x6f, 0x67, 0x69, 0x6e, 0x73, 0x65, 0x73, 0x73,
	0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x43, 0x72, 0x65, 0x61, 0x74, 0x65, 0x4c, 0x6f, 0x67, 0x69, 0x6e,
	0x53, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x25,
	0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e, 0x61, 0x75, 0x74, 0x68, 0x2e, 0x6c, 0x6f, 0x67, 0x69, 0x6e,
	0x73, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x73, 0x2e, 0x4c, 0x6f, 0x67, 0x69, 0x6e, 0x53, 0x65,
	0x73, 0x73, 0x69, 0x6f, 0x6e, 0x12, 0x69, 0x0a, 0x0f, 0x47, 0x65, 0x74, 0x4c, 0x6f, 0x67, 0x69,
	0x6e, 0x53, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x12, 0x2f, 0x2e, 0x6c, 0x75, 0x63, 0x69, 0x2e,
	0x61, 0x75, 0x74, 0x68, 0x2e, 0x6c, 0x6f, 0x67, 0x69, 0x6e, 0x73, 0x65, 0x73, 0x73, 0x69, 0x6f,
	0x6e, 0x73, 0x2e, 0x47, 0x65, 0x74, 0x4c, 0x6f, 0x67, 0x69, 0x6e, 0x53, 0x65, 0x73, 0x73, 0x69,
	0x6f, 0x6e, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x25, 0x2e, 0x6c, 0x75, 0x63, 0x69,
	0x2e, 0x61, 0x75, 0x74, 0x68, 0x2e, 0x6c, 0x6f, 0x67, 0x69, 0x6e, 0x73, 0x65, 0x73, 0x73, 0x69,
	0x6f, 0x6e, 0x73, 0x2e, 0x4c, 0x6f, 0x67, 0x69, 0x6e, 0x53, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e,
	0x42, 0x2b, 0x5a, 0x29, 0x67, 0x6f, 0x2e, 0x63, 0x68, 0x72, 0x6f, 0x6d, 0x69, 0x75, 0x6d, 0x2e,
	0x6f, 0x72, 0x67, 0x2f, 0x6c, 0x75, 0x63, 0x69, 0x2f, 0x61, 0x75, 0x74, 0x68, 0x2f, 0x6c, 0x6f,
	0x67, 0x69, 0x6e, 0x73, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e, 0x73, 0x70, 0x62, 0x62, 0x06, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x33,
})

var (
	file_go_chromium_org_luci_auth_loginsessionspb_service_proto_rawDescOnce sync.Once
	file_go_chromium_org_luci_auth_loginsessionspb_service_proto_rawDescData []byte
)

func file_go_chromium_org_luci_auth_loginsessionspb_service_proto_rawDescGZIP() []byte {
	file_go_chromium_org_luci_auth_loginsessionspb_service_proto_rawDescOnce.Do(func() {
		file_go_chromium_org_luci_auth_loginsessionspb_service_proto_rawDescData = protoimpl.X.CompressGZIP(unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_auth_loginsessionspb_service_proto_rawDesc), len(file_go_chromium_org_luci_auth_loginsessionspb_service_proto_rawDesc)))
	})
	return file_go_chromium_org_luci_auth_loginsessionspb_service_proto_rawDescData
}

var file_go_chromium_org_luci_auth_loginsessionspb_service_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_go_chromium_org_luci_auth_loginsessionspb_service_proto_msgTypes = make([]protoimpl.MessageInfo, 3)
var file_go_chromium_org_luci_auth_loginsessionspb_service_proto_goTypes = []any{
	(LoginSession_State)(0),           // 0: luci.auth.loginsessions.LoginSession.State
	(*CreateLoginSessionRequest)(nil), // 1: luci.auth.loginsessions.CreateLoginSessionRequest
	(*GetLoginSessionRequest)(nil),    // 2: luci.auth.loginsessions.GetLoginSessionRequest
	(*LoginSession)(nil),              // 3: luci.auth.loginsessions.LoginSession
	(*timestamppb.Timestamp)(nil),     // 4: google.protobuf.Timestamp
	(*durationpb.Duration)(nil),       // 5: google.protobuf.Duration
}
var file_go_chromium_org_luci_auth_loginsessionspb_service_proto_depIdxs = []int32{
	0, // 0: luci.auth.loginsessions.LoginSession.state:type_name -> luci.auth.loginsessions.LoginSession.State
	4, // 1: luci.auth.loginsessions.LoginSession.created:type_name -> google.protobuf.Timestamp
	4, // 2: luci.auth.loginsessions.LoginSession.expiry:type_name -> google.protobuf.Timestamp
	4, // 3: luci.auth.loginsessions.LoginSession.completed:type_name -> google.protobuf.Timestamp
	5, // 4: luci.auth.loginsessions.LoginSession.poll_interval:type_name -> google.protobuf.Duration
	5, // 5: luci.auth.loginsessions.LoginSession.confirmation_code_expiry:type_name -> google.protobuf.Duration
	5, // 6: luci.auth.loginsessions.LoginSession.confirmation_code_refresh:type_name -> google.protobuf.Duration
	1, // 7: luci.auth.loginsessions.LoginSessions.CreateLoginSession:input_type -> luci.auth.loginsessions.CreateLoginSessionRequest
	2, // 8: luci.auth.loginsessions.LoginSessions.GetLoginSession:input_type -> luci.auth.loginsessions.GetLoginSessionRequest
	3, // 9: luci.auth.loginsessions.LoginSessions.CreateLoginSession:output_type -> luci.auth.loginsessions.LoginSession
	3, // 10: luci.auth.loginsessions.LoginSessions.GetLoginSession:output_type -> luci.auth.loginsessions.LoginSession
	9, // [9:11] is the sub-list for method output_type
	7, // [7:9] is the sub-list for method input_type
	7, // [7:7] is the sub-list for extension type_name
	7, // [7:7] is the sub-list for extension extendee
	0, // [0:7] is the sub-list for field type_name
}

func init() { file_go_chromium_org_luci_auth_loginsessionspb_service_proto_init() }
func file_go_chromium_org_luci_auth_loginsessionspb_service_proto_init() {
	if File_go_chromium_org_luci_auth_loginsessionspb_service_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: unsafe.Slice(unsafe.StringData(file_go_chromium_org_luci_auth_loginsessionspb_service_proto_rawDesc), len(file_go_chromium_org_luci_auth_loginsessionspb_service_proto_rawDesc)),
			NumEnums:      1,
			NumMessages:   3,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_go_chromium_org_luci_auth_loginsessionspb_service_proto_goTypes,
		DependencyIndexes: file_go_chromium_org_luci_auth_loginsessionspb_service_proto_depIdxs,
		EnumInfos:         file_go_chromium_org_luci_auth_loginsessionspb_service_proto_enumTypes,
		MessageInfos:      file_go_chromium_org_luci_auth_loginsessionspb_service_proto_msgTypes,
	}.Build()
	File_go_chromium_org_luci_auth_loginsessionspb_service_proto = out.File
	file_go_chromium_org_luci_auth_loginsessionspb_service_proto_goTypes = nil
	file_go_chromium_org_luci_auth_loginsessionspb_service_proto_depIdxs = nil
}
