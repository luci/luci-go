// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v6.30.2
// source: go.chromium.org/luci/tokenserver/api/minter/v1/token_minter.proto

package minter

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	TokenMinter_MintMachineToken_FullMethodName        = "/tokenserver.minter.TokenMinter/MintMachineToken"
	TokenMinter_MintDelegationToken_FullMethodName     = "/tokenserver.minter.TokenMinter/MintDelegationToken"
	TokenMinter_MintProjectToken_FullMethodName        = "/tokenserver.minter.TokenMinter/MintProjectToken"
	TokenMinter_MintServiceAccountToken_FullMethodName = "/tokenserver.minter.TokenMinter/MintServiceAccountToken"
)

// TokenMinterClient is the client API for TokenMinter service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
//
// TokenMinter implements main API of the token server.
//
// It provides an interface for generating:
//
//   - Machine tokens: short lived stateless tokens used in Swarming bot
//     authentication protocol. They are derived from PKI keys deployed on bots,
//     and consumed primarily by Swarming. See MintMachineToken.
//   - Delegation tokens: these are involved whenever a service calls other
//     service on behalf of a user. They are passed via 'X-Delegation-Token-V1'
//     HTTP header along with a credentials of the impersonating user.
//     See MintDelegationToken.
//   - OAuth2 access tokens of project-scoped accounts: these are OAuth2 access
//     tokens that represents an identity associated with a LUCI project. See
//     MintProjectToken.
//   - Service accounts tokens: these are OAuth2 access tokens and ID tokens of
//     service accounts "residing" within various LUCI projects. They are
//     ultimately used by LUCI tasks as task service accounts.
//     See MintServiceAccountToken.
//
// RPCs that were deprecated and removed:
//
//   - MintOAuthTokenGrant and MintOAuthTokenViaGrant: were deprecated by
//     MintServiceAccountToken. Used (also now removed) service_accounts.cfg
//     config file.
type TokenMinterClient interface {
	// MintMachineToken generates a new token for an authenticated machine.
	//
	// It checks that provided certificate was signed by some trusted CA, and it
	// is still valid (non-expired and hasn't been revoked). It then checks that
	// the request was signed by the corresponding private key. Finally it checks
	// that the caller is authorized to generate requested kind of token.
	//
	// If everything checks out, it generates and returns a new machine token.
	//
	// On fatal error it returns detailed error response via same
	// MintMachineTokenResponse. On transient errors it returns generic
	// grpc.Internal error.
	MintMachineToken(ctx context.Context, in *MintMachineTokenRequest, opts ...grpc.CallOption) (*MintMachineTokenResponse, error)
	// MintDelegationToken generates a new bearer delegation token.
	//
	// Such token can be sent in 'X-Delegation-Token-V1' header (alongside regular
	// credentials like OAuth2 access token) to convey that the caller should be
	// authentication as 'delegated_identity' specified in the token.
	//
	// The delegation tokens are subject to multiple restrictions (embedded in
	// the token):
	//   - They have expiration time.
	//   - They are usable only if presented with a credential of someone from
	//     the 'audience' list.
	//   - They are usable only on services specified in the 'services' list.
	//
	// The token server must be configured in advance with all expected
	// combinations of (caller identity, delegated identity, audience, service)
	// tuples. See DelegationRule in config.proto.
	MintDelegationToken(ctx context.Context, in *MintDelegationTokenRequest, opts ...grpc.CallOption) (*MintDelegationTokenResponse, error)
	// MintProjectToken mints an OAuth2 access token that represents an identity
	// associated with a LUCI project.
	//
	// Project-scoped tokens prevent accidental cross-project identity confusion
	// when LUCI services access project specific resources such as a source code
	// repository.
	MintProjectToken(ctx context.Context, in *MintProjectTokenRequest, opts ...grpc.CallOption) (*MintProjectTokenResponse, error)
	// MintServiceAccountToken mints an OAuth2 access token or OpenID ID token
	// that belongs to some service account using LUCI Realms for authorization.
	//
	// As an input it takes a service account email and a name of a LUCI Realm the
	// caller is operating in. To authorize the call the token server checks the
	// following conditions:
	//  1. The caller has luci.serviceAccounts.mintToken permission in the
	//     realm, allowing them to "impersonate" all service accounts belonging
	//     to this realm.
	//  2. The service account has luci.serviceAccounts.existInRealm permission
	//     in the realm. This makes the account "belong" to the realm.
	//  3. Realm's LUCI project is allowed to impersonate this service account:
	//     a. Legacy approach being deprecated: realm's LUCI project is NOT listed
	//     in `use_project_scoped_account` set in project_owned_accounts.cfg
	//     global config file, but it has service accounts associated with it
	//     there via `mapping` field. In that case LUCI Token Server will check
	//     `mapping` and then use its own service account when minting tokens.
	//     b. New approach being rolled out: realm's LUCI project is listed in
	//     `use_project_scoped_account` set in project_owned_accounts.cfg
	//     global config file. In that case LUCI Token Server will use
	//     project-scoped account associated with this LUCI project when
	//     minting service account tokens. This essentially shifts mapping
	//     between LUCI projects and service accounts they can use into
	//     service account IAM policies.
	//
	// Check (3) makes sure different LUCI projects can't arbitrarily use each
	// others accounts by adding them to their respective realms.cfg. See also
	// comments for ServiceAccountsProjectMapping in api/admin/v1/config.proto.
	MintServiceAccountToken(ctx context.Context, in *MintServiceAccountTokenRequest, opts ...grpc.CallOption) (*MintServiceAccountTokenResponse, error)
}

type tokenMinterClient struct {
	cc grpc.ClientConnInterface
}

func NewTokenMinterClient(cc grpc.ClientConnInterface) TokenMinterClient {
	return &tokenMinterClient{cc}
}

func (c *tokenMinterClient) MintMachineToken(ctx context.Context, in *MintMachineTokenRequest, opts ...grpc.CallOption) (*MintMachineTokenResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(MintMachineTokenResponse)
	err := c.cc.Invoke(ctx, TokenMinter_MintMachineToken_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *tokenMinterClient) MintDelegationToken(ctx context.Context, in *MintDelegationTokenRequest, opts ...grpc.CallOption) (*MintDelegationTokenResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(MintDelegationTokenResponse)
	err := c.cc.Invoke(ctx, TokenMinter_MintDelegationToken_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *tokenMinterClient) MintProjectToken(ctx context.Context, in *MintProjectTokenRequest, opts ...grpc.CallOption) (*MintProjectTokenResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(MintProjectTokenResponse)
	err := c.cc.Invoke(ctx, TokenMinter_MintProjectToken_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *tokenMinterClient) MintServiceAccountToken(ctx context.Context, in *MintServiceAccountTokenRequest, opts ...grpc.CallOption) (*MintServiceAccountTokenResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(MintServiceAccountTokenResponse)
	err := c.cc.Invoke(ctx, TokenMinter_MintServiceAccountToken_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// TokenMinterServer is the server API for TokenMinter service.
// All implementations must embed UnimplementedTokenMinterServer
// for forward compatibility.
//
// TokenMinter implements main API of the token server.
//
// It provides an interface for generating:
//
//   - Machine tokens: short lived stateless tokens used in Swarming bot
//     authentication protocol. They are derived from PKI keys deployed on bots,
//     and consumed primarily by Swarming. See MintMachineToken.
//   - Delegation tokens: these are involved whenever a service calls other
//     service on behalf of a user. They are passed via 'X-Delegation-Token-V1'
//     HTTP header along with a credentials of the impersonating user.
//     See MintDelegationToken.
//   - OAuth2 access tokens of project-scoped accounts: these are OAuth2 access
//     tokens that represents an identity associated with a LUCI project. See
//     MintProjectToken.
//   - Service accounts tokens: these are OAuth2 access tokens and ID tokens of
//     service accounts "residing" within various LUCI projects. They are
//     ultimately used by LUCI tasks as task service accounts.
//     See MintServiceAccountToken.
//
// RPCs that were deprecated and removed:
//
//   - MintOAuthTokenGrant and MintOAuthTokenViaGrant: were deprecated by
//     MintServiceAccountToken. Used (also now removed) service_accounts.cfg
//     config file.
type TokenMinterServer interface {
	// MintMachineToken generates a new token for an authenticated machine.
	//
	// It checks that provided certificate was signed by some trusted CA, and it
	// is still valid (non-expired and hasn't been revoked). It then checks that
	// the request was signed by the corresponding private key. Finally it checks
	// that the caller is authorized to generate requested kind of token.
	//
	// If everything checks out, it generates and returns a new machine token.
	//
	// On fatal error it returns detailed error response via same
	// MintMachineTokenResponse. On transient errors it returns generic
	// grpc.Internal error.
	MintMachineToken(context.Context, *MintMachineTokenRequest) (*MintMachineTokenResponse, error)
	// MintDelegationToken generates a new bearer delegation token.
	//
	// Such token can be sent in 'X-Delegation-Token-V1' header (alongside regular
	// credentials like OAuth2 access token) to convey that the caller should be
	// authentication as 'delegated_identity' specified in the token.
	//
	// The delegation tokens are subject to multiple restrictions (embedded in
	// the token):
	//   - They have expiration time.
	//   - They are usable only if presented with a credential of someone from
	//     the 'audience' list.
	//   - They are usable only on services specified in the 'services' list.
	//
	// The token server must be configured in advance with all expected
	// combinations of (caller identity, delegated identity, audience, service)
	// tuples. See DelegationRule in config.proto.
	MintDelegationToken(context.Context, *MintDelegationTokenRequest) (*MintDelegationTokenResponse, error)
	// MintProjectToken mints an OAuth2 access token that represents an identity
	// associated with a LUCI project.
	//
	// Project-scoped tokens prevent accidental cross-project identity confusion
	// when LUCI services access project specific resources such as a source code
	// repository.
	MintProjectToken(context.Context, *MintProjectTokenRequest) (*MintProjectTokenResponse, error)
	// MintServiceAccountToken mints an OAuth2 access token or OpenID ID token
	// that belongs to some service account using LUCI Realms for authorization.
	//
	// As an input it takes a service account email and a name of a LUCI Realm the
	// caller is operating in. To authorize the call the token server checks the
	// following conditions:
	//  1. The caller has luci.serviceAccounts.mintToken permission in the
	//     realm, allowing them to "impersonate" all service accounts belonging
	//     to this realm.
	//  2. The service account has luci.serviceAccounts.existInRealm permission
	//     in the realm. This makes the account "belong" to the realm.
	//  3. Realm's LUCI project is allowed to impersonate this service account:
	//     a. Legacy approach being deprecated: realm's LUCI project is NOT listed
	//     in `use_project_scoped_account` set in project_owned_accounts.cfg
	//     global config file, but it has service accounts associated with it
	//     there via `mapping` field. In that case LUCI Token Server will check
	//     `mapping` and then use its own service account when minting tokens.
	//     b. New approach being rolled out: realm's LUCI project is listed in
	//     `use_project_scoped_account` set in project_owned_accounts.cfg
	//     global config file. In that case LUCI Token Server will use
	//     project-scoped account associated with this LUCI project when
	//     minting service account tokens. This essentially shifts mapping
	//     between LUCI projects and service accounts they can use into
	//     service account IAM policies.
	//
	// Check (3) makes sure different LUCI projects can't arbitrarily use each
	// others accounts by adding them to their respective realms.cfg. See also
	// comments for ServiceAccountsProjectMapping in api/admin/v1/config.proto.
	MintServiceAccountToken(context.Context, *MintServiceAccountTokenRequest) (*MintServiceAccountTokenResponse, error)
	mustEmbedUnimplementedTokenMinterServer()
}

// UnimplementedTokenMinterServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedTokenMinterServer struct{}

func (UnimplementedTokenMinterServer) MintMachineToken(context.Context, *MintMachineTokenRequest) (*MintMachineTokenResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method MintMachineToken not implemented")
}
func (UnimplementedTokenMinterServer) MintDelegationToken(context.Context, *MintDelegationTokenRequest) (*MintDelegationTokenResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method MintDelegationToken not implemented")
}
func (UnimplementedTokenMinterServer) MintProjectToken(context.Context, *MintProjectTokenRequest) (*MintProjectTokenResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method MintProjectToken not implemented")
}
func (UnimplementedTokenMinterServer) MintServiceAccountToken(context.Context, *MintServiceAccountTokenRequest) (*MintServiceAccountTokenResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method MintServiceAccountToken not implemented")
}
func (UnimplementedTokenMinterServer) mustEmbedUnimplementedTokenMinterServer() {}
func (UnimplementedTokenMinterServer) testEmbeddedByValue()                     {}

// UnsafeTokenMinterServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to TokenMinterServer will
// result in compilation errors.
type UnsafeTokenMinterServer interface {
	mustEmbedUnimplementedTokenMinterServer()
}

func RegisterTokenMinterServer(s grpc.ServiceRegistrar, srv TokenMinterServer) {
	// If the following call pancis, it indicates UnimplementedTokenMinterServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&TokenMinter_ServiceDesc, srv)
}

func _TokenMinter_MintMachineToken_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(MintMachineTokenRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TokenMinterServer).MintMachineToken(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: TokenMinter_MintMachineToken_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TokenMinterServer).MintMachineToken(ctx, req.(*MintMachineTokenRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TokenMinter_MintDelegationToken_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(MintDelegationTokenRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TokenMinterServer).MintDelegationToken(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: TokenMinter_MintDelegationToken_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TokenMinterServer).MintDelegationToken(ctx, req.(*MintDelegationTokenRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TokenMinter_MintProjectToken_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(MintProjectTokenRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TokenMinterServer).MintProjectToken(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: TokenMinter_MintProjectToken_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TokenMinterServer).MintProjectToken(ctx, req.(*MintProjectTokenRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _TokenMinter_MintServiceAccountToken_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(MintServiceAccountTokenRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(TokenMinterServer).MintServiceAccountToken(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: TokenMinter_MintServiceAccountToken_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(TokenMinterServer).MintServiceAccountToken(ctx, req.(*MintServiceAccountTokenRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// TokenMinter_ServiceDesc is the grpc.ServiceDesc for TokenMinter service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var TokenMinter_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "tokenserver.minter.TokenMinter",
	HandlerType: (*TokenMinterServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "MintMachineToken",
			Handler:    _TokenMinter_MintMachineToken_Handler,
		},
		{
			MethodName: "MintDelegationToken",
			Handler:    _TokenMinter_MintDelegationToken_Handler,
		},
		{
			MethodName: "MintProjectToken",
			Handler:    _TokenMinter_MintProjectToken_Handler,
		},
		{
			MethodName: "MintServiceAccountToken",
			Handler:    _TokenMinter_MintServiceAccountToken_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "go.chromium.org/luci/tokenserver/api/minter/v1/token_minter.proto",
}
