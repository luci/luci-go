// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v6.30.2
// source: go.chromium.org/luci/tokenserver/api/admin/v1/admin.proto

package admin

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	Admin_ImportCAConfigs_FullMethodName                   = "/tokenserver.admin.Admin/ImportCAConfigs"
	Admin_ImportDelegationConfigs_FullMethodName           = "/tokenserver.admin.Admin/ImportDelegationConfigs"
	Admin_ImportProjectIdentityConfigs_FullMethodName      = "/tokenserver.admin.Admin/ImportProjectIdentityConfigs"
	Admin_ImportProjectOwnedAccountsConfigs_FullMethodName = "/tokenserver.admin.Admin/ImportProjectOwnedAccountsConfigs"
	Admin_InspectMachineToken_FullMethodName               = "/tokenserver.admin.Admin/InspectMachineToken"
	Admin_InspectDelegationToken_FullMethodName            = "/tokenserver.admin.Admin/InspectDelegationToken"
)

// AdminClient is the client API for Admin service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
//
// Admin service is used by service administrators to manage the server.
type AdminClient interface {
	// ImportCAConfigs makes the server read 'tokenserver.cfg'.
	ImportCAConfigs(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*ImportedConfigs, error)
	// ImportDelegationConfigs makes the server read 'delegation.cfg'.
	ImportDelegationConfigs(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*ImportedConfigs, error)
	// ImportProjectIdentityConfigs makes the server read 'projects.cfg'.
	ImportProjectIdentityConfigs(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*ImportedConfigs, error)
	// ImportProjectOwnedAccountsConfigs makes the server read 'project_owned_accounts.cfg'.
	ImportProjectOwnedAccountsConfigs(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*ImportedConfigs, error)
	// InspectMachineToken decodes a machine token and verifies it is valid.
	//
	// It verifies the token was signed by a private key of the token server and
	// checks token's expiration time and revocation status.
	//
	// It tries to give as much information about the token and its status as
	// possible (e.g. it checks for revocation status even if token is already
	// expired).
	//
	// Administrators can use this call to debug issues with tokens.
	//
	// Returns:
	//
	//	InspectMachineTokenResponse for tokens of supported kind.
	//	grpc.InvalidArgument error for unsupported token kind.
	//	grpc.Internal error for transient errors.
	InspectMachineToken(ctx context.Context, in *InspectMachineTokenRequest, opts ...grpc.CallOption) (*InspectMachineTokenResponse, error)
	// InspectDelegationToken decodes a delegation token and verifies it is valid.
	//
	// It verifies the token was signed by a private key of the token server and
	// checks token's expiration time.
	//
	// It tries to give as much information about the token and its status as
	// possible (e.g. attempts to decode the body even if the signing key has been
	// rotated already).
	//
	// Administrators can use this call to debug issues with tokens.
	//
	// Returns:
	//
	//	InspectDelegationTokenResponse for tokens of supported kind.
	//	grpc.InvalidArgument error for unsupported token kind.
	//	grpc.Internal error for transient errors.
	InspectDelegationToken(ctx context.Context, in *InspectDelegationTokenRequest, opts ...grpc.CallOption) (*InspectDelegationTokenResponse, error)
}

type adminClient struct {
	cc grpc.ClientConnInterface
}

func NewAdminClient(cc grpc.ClientConnInterface) AdminClient {
	return &adminClient{cc}
}

func (c *adminClient) ImportCAConfigs(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*ImportedConfigs, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ImportedConfigs)
	err := c.cc.Invoke(ctx, Admin_ImportCAConfigs_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminClient) ImportDelegationConfigs(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*ImportedConfigs, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ImportedConfigs)
	err := c.cc.Invoke(ctx, Admin_ImportDelegationConfigs_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminClient) ImportProjectIdentityConfigs(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*ImportedConfigs, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ImportedConfigs)
	err := c.cc.Invoke(ctx, Admin_ImportProjectIdentityConfigs_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminClient) ImportProjectOwnedAccountsConfigs(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*ImportedConfigs, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ImportedConfigs)
	err := c.cc.Invoke(ctx, Admin_ImportProjectOwnedAccountsConfigs_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminClient) InspectMachineToken(ctx context.Context, in *InspectMachineTokenRequest, opts ...grpc.CallOption) (*InspectMachineTokenResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(InspectMachineTokenResponse)
	err := c.cc.Invoke(ctx, Admin_InspectMachineToken_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminClient) InspectDelegationToken(ctx context.Context, in *InspectDelegationTokenRequest, opts ...grpc.CallOption) (*InspectDelegationTokenResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(InspectDelegationTokenResponse)
	err := c.cc.Invoke(ctx, Admin_InspectDelegationToken_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// AdminServer is the server API for Admin service.
// All implementations must embed UnimplementedAdminServer
// for forward compatibility.
//
// Admin service is used by service administrators to manage the server.
type AdminServer interface {
	// ImportCAConfigs makes the server read 'tokenserver.cfg'.
	ImportCAConfigs(context.Context, *emptypb.Empty) (*ImportedConfigs, error)
	// ImportDelegationConfigs makes the server read 'delegation.cfg'.
	ImportDelegationConfigs(context.Context, *emptypb.Empty) (*ImportedConfigs, error)
	// ImportProjectIdentityConfigs makes the server read 'projects.cfg'.
	ImportProjectIdentityConfigs(context.Context, *emptypb.Empty) (*ImportedConfigs, error)
	// ImportProjectOwnedAccountsConfigs makes the server read 'project_owned_accounts.cfg'.
	ImportProjectOwnedAccountsConfigs(context.Context, *emptypb.Empty) (*ImportedConfigs, error)
	// InspectMachineToken decodes a machine token and verifies it is valid.
	//
	// It verifies the token was signed by a private key of the token server and
	// checks token's expiration time and revocation status.
	//
	// It tries to give as much information about the token and its status as
	// possible (e.g. it checks for revocation status even if token is already
	// expired).
	//
	// Administrators can use this call to debug issues with tokens.
	//
	// Returns:
	//
	//	InspectMachineTokenResponse for tokens of supported kind.
	//	grpc.InvalidArgument error for unsupported token kind.
	//	grpc.Internal error for transient errors.
	InspectMachineToken(context.Context, *InspectMachineTokenRequest) (*InspectMachineTokenResponse, error)
	// InspectDelegationToken decodes a delegation token and verifies it is valid.
	//
	// It verifies the token was signed by a private key of the token server and
	// checks token's expiration time.
	//
	// It tries to give as much information about the token and its status as
	// possible (e.g. attempts to decode the body even if the signing key has been
	// rotated already).
	//
	// Administrators can use this call to debug issues with tokens.
	//
	// Returns:
	//
	//	InspectDelegationTokenResponse for tokens of supported kind.
	//	grpc.InvalidArgument error for unsupported token kind.
	//	grpc.Internal error for transient errors.
	InspectDelegationToken(context.Context, *InspectDelegationTokenRequest) (*InspectDelegationTokenResponse, error)
	mustEmbedUnimplementedAdminServer()
}

// UnimplementedAdminServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedAdminServer struct{}

func (UnimplementedAdminServer) ImportCAConfigs(context.Context, *emptypb.Empty) (*ImportedConfigs, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ImportCAConfigs not implemented")
}
func (UnimplementedAdminServer) ImportDelegationConfigs(context.Context, *emptypb.Empty) (*ImportedConfigs, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ImportDelegationConfigs not implemented")
}
func (UnimplementedAdminServer) ImportProjectIdentityConfigs(context.Context, *emptypb.Empty) (*ImportedConfigs, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ImportProjectIdentityConfigs not implemented")
}
func (UnimplementedAdminServer) ImportProjectOwnedAccountsConfigs(context.Context, *emptypb.Empty) (*ImportedConfigs, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ImportProjectOwnedAccountsConfigs not implemented")
}
func (UnimplementedAdminServer) InspectMachineToken(context.Context, *InspectMachineTokenRequest) (*InspectMachineTokenResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method InspectMachineToken not implemented")
}
func (UnimplementedAdminServer) InspectDelegationToken(context.Context, *InspectDelegationTokenRequest) (*InspectDelegationTokenResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method InspectDelegationToken not implemented")
}
func (UnimplementedAdminServer) mustEmbedUnimplementedAdminServer() {}
func (UnimplementedAdminServer) testEmbeddedByValue()               {}

// UnsafeAdminServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to AdminServer will
// result in compilation errors.
type UnsafeAdminServer interface {
	mustEmbedUnimplementedAdminServer()
}

func RegisterAdminServer(s grpc.ServiceRegistrar, srv AdminServer) {
	// If the following call pancis, it indicates UnimplementedAdminServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&Admin_ServiceDesc, srv)
}

func _Admin_ImportCAConfigs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(emptypb.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServer).ImportCAConfigs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Admin_ImportCAConfigs_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServer).ImportCAConfigs(ctx, req.(*emptypb.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

func _Admin_ImportDelegationConfigs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(emptypb.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServer).ImportDelegationConfigs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Admin_ImportDelegationConfigs_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServer).ImportDelegationConfigs(ctx, req.(*emptypb.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

func _Admin_ImportProjectIdentityConfigs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(emptypb.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServer).ImportProjectIdentityConfigs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Admin_ImportProjectIdentityConfigs_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServer).ImportProjectIdentityConfigs(ctx, req.(*emptypb.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

func _Admin_ImportProjectOwnedAccountsConfigs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(emptypb.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServer).ImportProjectOwnedAccountsConfigs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Admin_ImportProjectOwnedAccountsConfigs_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServer).ImportProjectOwnedAccountsConfigs(ctx, req.(*emptypb.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

func _Admin_InspectMachineToken_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InspectMachineTokenRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServer).InspectMachineToken(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Admin_InspectMachineToken_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServer).InspectMachineToken(ctx, req.(*InspectMachineTokenRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Admin_InspectDelegationToken_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InspectDelegationTokenRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServer).InspectDelegationToken(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Admin_InspectDelegationToken_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServer).InspectDelegationToken(ctx, req.(*InspectDelegationTokenRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Admin_ServiceDesc is the grpc.ServiceDesc for Admin service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Admin_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "tokenserver.admin.Admin",
	HandlerType: (*AdminServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "ImportCAConfigs",
			Handler:    _Admin_ImportCAConfigs_Handler,
		},
		{
			MethodName: "ImportDelegationConfigs",
			Handler:    _Admin_ImportDelegationConfigs_Handler,
		},
		{
			MethodName: "ImportProjectIdentityConfigs",
			Handler:    _Admin_ImportProjectIdentityConfigs_Handler,
		},
		{
			MethodName: "ImportProjectOwnedAccountsConfigs",
			Handler:    _Admin_ImportProjectOwnedAccountsConfigs_Handler,
		},
		{
			MethodName: "InspectMachineToken",
			Handler:    _Admin_InspectMachineToken_Handler,
		},
		{
			MethodName: "InspectDelegationToken",
			Handler:    _Admin_InspectDelegationToken_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "go.chromium.org/luci/tokenserver/api/admin/v1/admin.proto",
}
