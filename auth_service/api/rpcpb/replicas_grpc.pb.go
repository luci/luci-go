// Copyright 2024 The LUCI Authors.
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

// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v6.30.2
// source: go.chromium.org/luci/auth_service/api/rpcpb/replicas.proto

package rpcpb

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
	Replicas_ListReplicas_FullMethodName = "/auth.service.Replicas/ListReplicas"
)

// ReplicasClient is the client API for Replicas service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type ReplicasClient interface {
	// ListReplicas lists the replicas that have been linked with Auth Service
	// as their primary AuthDB service.
	ListReplicas(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*ListReplicasResponse, error)
}

type replicasClient struct {
	cc grpc.ClientConnInterface
}

func NewReplicasClient(cc grpc.ClientConnInterface) ReplicasClient {
	return &replicasClient{cc}
}

func (c *replicasClient) ListReplicas(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*ListReplicasResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ListReplicasResponse)
	err := c.cc.Invoke(ctx, Replicas_ListReplicas_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ReplicasServer is the server API for Replicas service.
// All implementations must embed UnimplementedReplicasServer
// for forward compatibility.
type ReplicasServer interface {
	// ListReplicas lists the replicas that have been linked with Auth Service
	// as their primary AuthDB service.
	ListReplicas(context.Context, *emptypb.Empty) (*ListReplicasResponse, error)
	mustEmbedUnimplementedReplicasServer()
}

// UnimplementedReplicasServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedReplicasServer struct{}

func (UnimplementedReplicasServer) ListReplicas(context.Context, *emptypb.Empty) (*ListReplicasResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListReplicas not implemented")
}
func (UnimplementedReplicasServer) mustEmbedUnimplementedReplicasServer() {}
func (UnimplementedReplicasServer) testEmbeddedByValue()                  {}

// UnsafeReplicasServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to ReplicasServer will
// result in compilation errors.
type UnsafeReplicasServer interface {
	mustEmbedUnimplementedReplicasServer()
}

func RegisterReplicasServer(s grpc.ServiceRegistrar, srv ReplicasServer) {
	// If the following call pancis, it indicates UnimplementedReplicasServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&Replicas_ServiceDesc, srv)
}

func _Replicas_ListReplicas_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(emptypb.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ReplicasServer).ListReplicas(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Replicas_ListReplicas_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ReplicasServer).ListReplicas(ctx, req.(*emptypb.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

// Replicas_ServiceDesc is the grpc.ServiceDesc for Replicas service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Replicas_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "auth.service.Replicas",
	HandlerType: (*ReplicasServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "ListReplicas",
			Handler:    _Replicas_ListReplicas_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "go.chromium.org/luci/auth_service/api/rpcpb/replicas.proto",
}
