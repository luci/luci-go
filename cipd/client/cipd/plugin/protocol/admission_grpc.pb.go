// Copyright 2020 The LUCI Authors.
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
// source: go.chromium.org/luci/cipd/client/cipd/plugin/protocol/admission.proto

package protocol

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
	Admissions_ListAdmissions_FullMethodName   = "/cipd.plugin.Admissions/ListAdmissions"
	Admissions_ResolveAdmission_FullMethodName = "/cipd.plugin.Admissions/ResolveAdmission"
)

// AdmissionsClient is the client API for Admissions service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
//
// Admissions service is available to deployment admission plugins.
//
// They control what CIPD packages are allowed to be deployed. The admission
// plugin must call ListAdmissions as soon as it connects, and for each incoming
// Admission message eventually make ResolveAdmission RPC. It should abort as
// soon as ListAdmissions stream ends for whatever reason (in particular is
// should not try to call ListAdmissions again).
type AdmissionsClient interface {
	// ListAdmissions returns a stream of admission requests to process.
	ListAdmissions(ctx context.Context, in *ListAdmissionsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[Admission], error)
	// ResolveAdmission submits a decision on an admission request.
	ResolveAdmission(ctx context.Context, in *ResolveAdmissionRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
}

type admissionsClient struct {
	cc grpc.ClientConnInterface
}

func NewAdmissionsClient(cc grpc.ClientConnInterface) AdmissionsClient {
	return &admissionsClient{cc}
}

func (c *admissionsClient) ListAdmissions(ctx context.Context, in *ListAdmissionsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[Admission], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &Admissions_ServiceDesc.Streams[0], Admissions_ListAdmissions_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[ListAdmissionsRequest, Admission]{ClientStream: stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Admissions_ListAdmissionsClient = grpc.ServerStreamingClient[Admission]

func (c *admissionsClient) ResolveAdmission(ctx context.Context, in *ResolveAdmissionRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, Admissions_ResolveAdmission_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// AdmissionsServer is the server API for Admissions service.
// All implementations must embed UnimplementedAdmissionsServer
// for forward compatibility.
//
// Admissions service is available to deployment admission plugins.
//
// They control what CIPD packages are allowed to be deployed. The admission
// plugin must call ListAdmissions as soon as it connects, and for each incoming
// Admission message eventually make ResolveAdmission RPC. It should abort as
// soon as ListAdmissions stream ends for whatever reason (in particular is
// should not try to call ListAdmissions again).
type AdmissionsServer interface {
	// ListAdmissions returns a stream of admission requests to process.
	ListAdmissions(*ListAdmissionsRequest, grpc.ServerStreamingServer[Admission]) error
	// ResolveAdmission submits a decision on an admission request.
	ResolveAdmission(context.Context, *ResolveAdmissionRequest) (*emptypb.Empty, error)
	mustEmbedUnimplementedAdmissionsServer()
}

// UnimplementedAdmissionsServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedAdmissionsServer struct{}

func (UnimplementedAdmissionsServer) ListAdmissions(*ListAdmissionsRequest, grpc.ServerStreamingServer[Admission]) error {
	return status.Errorf(codes.Unimplemented, "method ListAdmissions not implemented")
}
func (UnimplementedAdmissionsServer) ResolveAdmission(context.Context, *ResolveAdmissionRequest) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ResolveAdmission not implemented")
}
func (UnimplementedAdmissionsServer) mustEmbedUnimplementedAdmissionsServer() {}
func (UnimplementedAdmissionsServer) testEmbeddedByValue()                    {}

// UnsafeAdmissionsServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to AdmissionsServer will
// result in compilation errors.
type UnsafeAdmissionsServer interface {
	mustEmbedUnimplementedAdmissionsServer()
}

func RegisterAdmissionsServer(s grpc.ServiceRegistrar, srv AdmissionsServer) {
	// If the following call pancis, it indicates UnimplementedAdmissionsServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&Admissions_ServiceDesc, srv)
}

func _Admissions_ListAdmissions_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(ListAdmissionsRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(AdmissionsServer).ListAdmissions(m, &grpc.GenericServerStream[ListAdmissionsRequest, Admission]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Admissions_ListAdmissionsServer = grpc.ServerStreamingServer[Admission]

func _Admissions_ResolveAdmission_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ResolveAdmissionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdmissionsServer).ResolveAdmission(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Admissions_ResolveAdmission_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdmissionsServer).ResolveAdmission(ctx, req.(*ResolveAdmissionRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Admissions_ServiceDesc is the grpc.ServiceDesc for Admissions service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Admissions_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "cipd.plugin.Admissions",
	HandlerType: (*AdmissionsServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "ResolveAdmission",
			Handler:    _Admissions_ResolveAdmission_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "ListAdmissions",
			Handler:       _Admissions_ListAdmissions_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "go.chromium.org/luci/cipd/client/cipd/plugin/protocol/admission.proto",
}
