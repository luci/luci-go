// Copyright 2023 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v6.30.2
// source: go.chromium.org/luci/config_service/proto/config_service.proto

package configpb

import (
	context "context"
	config "go.chromium.org/luci/common/proto/config"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	Configs_GetConfig_FullMethodName         = "/config.service.v2.Configs/GetConfig"
	Configs_GetProjectConfigs_FullMethodName = "/config.service.v2.Configs/GetProjectConfigs"
	Configs_ListConfigSets_FullMethodName    = "/config.service.v2.Configs/ListConfigSets"
	Configs_GetConfigSet_FullMethodName      = "/config.service.v2.Configs/GetConfigSet"
	Configs_ValidateConfigs_FullMethodName   = "/config.service.v2.Configs/ValidateConfigs"
)

// ConfigsClient is the client API for Configs service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
//
// Configs Service.
type ConfigsClient interface {
	// Get one configuration.
	GetConfig(ctx context.Context, in *GetConfigRequest, opts ...grpc.CallOption) (*Config, error)
	// Get the specified project configs from all projects.
	GetProjectConfigs(ctx context.Context, in *GetProjectConfigsRequest, opts ...grpc.CallOption) (*GetProjectConfigsResponse, error)
	// List config sets.
	ListConfigSets(ctx context.Context, in *ListConfigSetsRequest, opts ...grpc.CallOption) (*ListConfigSetsResponse, error)
	// Get a single config set.
	GetConfigSet(ctx context.Context, in *GetConfigSetRequest, opts ...grpc.CallOption) (*ConfigSet, error)
	// Validates configs for a config set.
	//
	// The validation workflow works as follows (assuming first time validation):
	//  1. Client sends the manifest of the config directory for validation. The
	//     manifest consists of the relative posix style path to each config file
	//     and the SHA256 hash of the content of each config file.
	//  2. Server returns grpc error status with InvalidArgument code and
	//     a `BadValidationRequestFixInfo` message in status_detail.
	//     `BadValidationRequestFixInfo` should contain a signed url for each
	//     config file and Client is responsible to upload the *gzip compressed*
	//     config to the url. Client should also fix any remaining error mentioned
	//     in `BadValidationRequestFixInfo`. Note that, if the request contains
	//     any invalid argument like malformed config set or absolute file path,
	//     LUCI Config will only return the grpc error status with InvalidArgument
	//     code but without anything in status_details because those type of errors
	//     are not fixable.
	//  3. Call the server again with the same validation request as in step 1. The
	//     Server should be able to perform the validation and return the
	//     result.
	//  4. Repeat step 1-3 for any subsequent validation request. Note that for
	//     step 2, the Server would only ask client to upload files that it has
	//     not seen their hashes in any previous validation session (for up to 1
	//     day).
	ValidateConfigs(ctx context.Context, in *ValidateConfigsRequest, opts ...grpc.CallOption) (*config.ValidationResult, error)
}

type configsClient struct {
	cc grpc.ClientConnInterface
}

func NewConfigsClient(cc grpc.ClientConnInterface) ConfigsClient {
	return &configsClient{cc}
}

func (c *configsClient) GetConfig(ctx context.Context, in *GetConfigRequest, opts ...grpc.CallOption) (*Config, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(Config)
	err := c.cc.Invoke(ctx, Configs_GetConfig_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *configsClient) GetProjectConfigs(ctx context.Context, in *GetProjectConfigsRequest, opts ...grpc.CallOption) (*GetProjectConfigsResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetProjectConfigsResponse)
	err := c.cc.Invoke(ctx, Configs_GetProjectConfigs_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *configsClient) ListConfigSets(ctx context.Context, in *ListConfigSetsRequest, opts ...grpc.CallOption) (*ListConfigSetsResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ListConfigSetsResponse)
	err := c.cc.Invoke(ctx, Configs_ListConfigSets_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *configsClient) GetConfigSet(ctx context.Context, in *GetConfigSetRequest, opts ...grpc.CallOption) (*ConfigSet, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ConfigSet)
	err := c.cc.Invoke(ctx, Configs_GetConfigSet_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *configsClient) ValidateConfigs(ctx context.Context, in *ValidateConfigsRequest, opts ...grpc.CallOption) (*config.ValidationResult, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(config.ValidationResult)
	err := c.cc.Invoke(ctx, Configs_ValidateConfigs_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ConfigsServer is the server API for Configs service.
// All implementations must embed UnimplementedConfigsServer
// for forward compatibility.
//
// Configs Service.
type ConfigsServer interface {
	// Get one configuration.
	GetConfig(context.Context, *GetConfigRequest) (*Config, error)
	// Get the specified project configs from all projects.
	GetProjectConfigs(context.Context, *GetProjectConfigsRequest) (*GetProjectConfigsResponse, error)
	// List config sets.
	ListConfigSets(context.Context, *ListConfigSetsRequest) (*ListConfigSetsResponse, error)
	// Get a single config set.
	GetConfigSet(context.Context, *GetConfigSetRequest) (*ConfigSet, error)
	// Validates configs for a config set.
	//
	// The validation workflow works as follows (assuming first time validation):
	//  1. Client sends the manifest of the config directory for validation. The
	//     manifest consists of the relative posix style path to each config file
	//     and the SHA256 hash of the content of each config file.
	//  2. Server returns grpc error status with InvalidArgument code and
	//     a `BadValidationRequestFixInfo` message in status_detail.
	//     `BadValidationRequestFixInfo` should contain a signed url for each
	//     config file and Client is responsible to upload the *gzip compressed*
	//     config to the url. Client should also fix any remaining error mentioned
	//     in `BadValidationRequestFixInfo`. Note that, if the request contains
	//     any invalid argument like malformed config set or absolute file path,
	//     LUCI Config will only return the grpc error status with InvalidArgument
	//     code but without anything in status_details because those type of errors
	//     are not fixable.
	//  3. Call the server again with the same validation request as in step 1. The
	//     Server should be able to perform the validation and return the
	//     result.
	//  4. Repeat step 1-3 for any subsequent validation request. Note that for
	//     step 2, the Server would only ask client to upload files that it has
	//     not seen their hashes in any previous validation session (for up to 1
	//     day).
	ValidateConfigs(context.Context, *ValidateConfigsRequest) (*config.ValidationResult, error)
	mustEmbedUnimplementedConfigsServer()
}

// UnimplementedConfigsServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedConfigsServer struct{}

func (UnimplementedConfigsServer) GetConfig(context.Context, *GetConfigRequest) (*Config, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetConfig not implemented")
}
func (UnimplementedConfigsServer) GetProjectConfigs(context.Context, *GetProjectConfigsRequest) (*GetProjectConfigsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetProjectConfigs not implemented")
}
func (UnimplementedConfigsServer) ListConfigSets(context.Context, *ListConfigSetsRequest) (*ListConfigSetsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListConfigSets not implemented")
}
func (UnimplementedConfigsServer) GetConfigSet(context.Context, *GetConfigSetRequest) (*ConfigSet, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetConfigSet not implemented")
}
func (UnimplementedConfigsServer) ValidateConfigs(context.Context, *ValidateConfigsRequest) (*config.ValidationResult, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ValidateConfigs not implemented")
}
func (UnimplementedConfigsServer) mustEmbedUnimplementedConfigsServer() {}
func (UnimplementedConfigsServer) testEmbeddedByValue()                 {}

// UnsafeConfigsServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to ConfigsServer will
// result in compilation errors.
type UnsafeConfigsServer interface {
	mustEmbedUnimplementedConfigsServer()
}

func RegisterConfigsServer(s grpc.ServiceRegistrar, srv ConfigsServer) {
	// If the following call pancis, it indicates UnimplementedConfigsServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&Configs_ServiceDesc, srv)
}

func _Configs_GetConfig_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetConfigRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ConfigsServer).GetConfig(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Configs_GetConfig_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ConfigsServer).GetConfig(ctx, req.(*GetConfigRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Configs_GetProjectConfigs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetProjectConfigsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ConfigsServer).GetProjectConfigs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Configs_GetProjectConfigs_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ConfigsServer).GetProjectConfigs(ctx, req.(*GetProjectConfigsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Configs_ListConfigSets_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListConfigSetsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ConfigsServer).ListConfigSets(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Configs_ListConfigSets_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ConfigsServer).ListConfigSets(ctx, req.(*ListConfigSetsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Configs_GetConfigSet_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetConfigSetRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ConfigsServer).GetConfigSet(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Configs_GetConfigSet_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ConfigsServer).GetConfigSet(ctx, req.(*GetConfigSetRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Configs_ValidateConfigs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ValidateConfigsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ConfigsServer).ValidateConfigs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Configs_ValidateConfigs_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ConfigsServer).ValidateConfigs(ctx, req.(*ValidateConfigsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Configs_ServiceDesc is the grpc.ServiceDesc for Configs service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Configs_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "config.service.v2.Configs",
	HandlerType: (*ConfigsServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetConfig",
			Handler:    _Configs_GetConfig_Handler,
		},
		{
			MethodName: "GetProjectConfigs",
			Handler:    _Configs_GetProjectConfigs_Handler,
		},
		{
			MethodName: "ListConfigSets",
			Handler:    _Configs_ListConfigSets_Handler,
		},
		{
			MethodName: "GetConfigSet",
			Handler:    _Configs_GetConfigSet_Handler,
		},
		{
			MethodName: "ValidateConfigs",
			Handler:    _Configs_ValidateConfigs_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "go.chromium.org/luci/config_service/proto/config_service.proto",
}
