// Copyright 2021 The LUCI Authors.
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
// source: go.chromium.org/luci/cv/api/v0/service_runs.proto

package cvpb

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
	Runs_GetRun_FullMethodName     = "/cv.v0.Runs/GetRun"
	Runs_SearchRuns_FullMethodName = "/cv.v0.Runs/SearchRuns"
)

// RunsClient is the client API for Runs service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
//
// Runs service exposes CV Runs and their sub-resources.
//
// !!!!! WARNING !!!!!
//   - Use at your own risk.
//   - We will stop supporting this v0 API without notice.
//   - No backwards compatibility guaranteed.
//   - Please, contact CV maintainers at luci-eng@ before using this and
//     we may provide additional guarantees to you/your service.
type RunsClient interface {
	// GetRun returns Run details.
	GetRun(ctx context.Context, in *GetRunRequest, opts ...grpc.CallOption) (*Run, error)
	// SearchRuns searches for Runs.
	SearchRuns(ctx context.Context, in *SearchRunsRequest, opts ...grpc.CallOption) (*SearchRunsResponse, error)
}

type runsClient struct {
	cc grpc.ClientConnInterface
}

func NewRunsClient(cc grpc.ClientConnInterface) RunsClient {
	return &runsClient{cc}
}

func (c *runsClient) GetRun(ctx context.Context, in *GetRunRequest, opts ...grpc.CallOption) (*Run, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(Run)
	err := c.cc.Invoke(ctx, Runs_GetRun_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *runsClient) SearchRuns(ctx context.Context, in *SearchRunsRequest, opts ...grpc.CallOption) (*SearchRunsResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(SearchRunsResponse)
	err := c.cc.Invoke(ctx, Runs_SearchRuns_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RunsServer is the server API for Runs service.
// All implementations must embed UnimplementedRunsServer
// for forward compatibility.
//
// Runs service exposes CV Runs and their sub-resources.
//
// !!!!! WARNING !!!!!
//   - Use at your own risk.
//   - We will stop supporting this v0 API without notice.
//   - No backwards compatibility guaranteed.
//   - Please, contact CV maintainers at luci-eng@ before using this and
//     we may provide additional guarantees to you/your service.
type RunsServer interface {
	// GetRun returns Run details.
	GetRun(context.Context, *GetRunRequest) (*Run, error)
	// SearchRuns searches for Runs.
	SearchRuns(context.Context, *SearchRunsRequest) (*SearchRunsResponse, error)
	mustEmbedUnimplementedRunsServer()
}

// UnimplementedRunsServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedRunsServer struct{}

func (UnimplementedRunsServer) GetRun(context.Context, *GetRunRequest) (*Run, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetRun not implemented")
}
func (UnimplementedRunsServer) SearchRuns(context.Context, *SearchRunsRequest) (*SearchRunsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SearchRuns not implemented")
}
func (UnimplementedRunsServer) mustEmbedUnimplementedRunsServer() {}
func (UnimplementedRunsServer) testEmbeddedByValue()              {}

// UnsafeRunsServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to RunsServer will
// result in compilation errors.
type UnsafeRunsServer interface {
	mustEmbedUnimplementedRunsServer()
}

func RegisterRunsServer(s grpc.ServiceRegistrar, srv RunsServer) {
	// If the following call pancis, it indicates UnimplementedRunsServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&Runs_ServiceDesc, srv)
}

func _Runs_GetRun_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetRunRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RunsServer).GetRun(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Runs_GetRun_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RunsServer).GetRun(ctx, req.(*GetRunRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Runs_SearchRuns_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SearchRunsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RunsServer).SearchRuns(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Runs_SearchRuns_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RunsServer).SearchRuns(ctx, req.(*SearchRunsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Runs_ServiceDesc is the grpc.ServiceDesc for Runs service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Runs_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "cv.v0.Runs",
	HandlerType: (*RunsServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetRun",
			Handler:    _Runs_GetRun_Handler,
		},
		{
			MethodName: "SearchRuns",
			Handler:    _Runs_SearchRuns_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "go.chromium.org/luci/cv/api/v0/service_runs.proto",
}
