// Copyright 2016 The LUCI Authors.
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
// source: go.chromium.org/luci/scheduler/api/scheduler/v1/scheduler.proto

package scheduler

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
	Scheduler_GetJobs_FullMethodName         = "/scheduler.Scheduler/GetJobs"
	Scheduler_GetInvocations_FullMethodName  = "/scheduler.Scheduler/GetInvocations"
	Scheduler_GetInvocation_FullMethodName   = "/scheduler.Scheduler/GetInvocation"
	Scheduler_PauseJob_FullMethodName        = "/scheduler.Scheduler/PauseJob"
	Scheduler_ResumeJob_FullMethodName       = "/scheduler.Scheduler/ResumeJob"
	Scheduler_AbortJob_FullMethodName        = "/scheduler.Scheduler/AbortJob"
	Scheduler_AbortInvocation_FullMethodName = "/scheduler.Scheduler/AbortInvocation"
	Scheduler_EmitTriggers_FullMethodName    = "/scheduler.Scheduler/EmitTriggers"
)

// SchedulerClient is the client API for Scheduler service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
//
// Scheduler exposes public API of the Scheduler service.
type SchedulerClient interface {
	// GetJobs fetches all jobs satisfying JobsRequest and visibility ACLs.
	//
	// If JobsRequest.project is specified but the project doesn't exist, empty
	// list of Jobs is returned.
	//
	// A job is visible if the caller has "scheduler.jobs.get" permission.
	GetJobs(ctx context.Context, in *JobsRequest, opts ...grpc.CallOption) (*JobsReply, error)
	// GetInvocations fetches invocations of a given job, most recent first.
	//
	// Requires "scheduler.jobs.get" permission.
	GetInvocations(ctx context.Context, in *InvocationsRequest, opts ...grpc.CallOption) (*InvocationsReply, error)
	// GetInvocation fetches a single invocation.
	//
	// Requires "scheduler.jobs.get" permission.
	GetInvocation(ctx context.Context, in *InvocationRef, opts ...grpc.CallOption) (*Invocation, error)
	// PauseJob will prevent automatic triggering of a job.
	//
	// Manual triggering (e.g. via EmitTriggers RPC) is still allowed. Any pending
	// or running invocations are still executed.
	//
	// PauseJob does nothing if job is already paused.
	//
	// Requires "scheduler.jobs.pause" permission.
	PauseJob(ctx context.Context, in *JobRef, opts ...grpc.CallOption) (*emptypb.Empty, error)
	// ResumeJob resumes paused job.
	//
	// ResumeJob does nothing if job is not paused.
	//
	// Requires "scheduler.jobs.resume" permission.
	ResumeJob(ctx context.Context, in *JobRef, opts ...grpc.CallOption) (*emptypb.Empty, error)
	// AbortJob resets the job to scheduled state, aborting a currently pending or
	// running invocation if any.
	//
	// Note, that this is similar to AbortInvocation except that AbortInvocation
	// requires invocation ID and doesn't ensure that the invocation aborted is
	// actually latest triggered for the job.
	//
	// Requires "scheduler.jobs.abort" permission.
	AbortJob(ctx context.Context, in *JobRef, opts ...grpc.CallOption) (*emptypb.Empty, error)
	// AbortInvocation aborts a given job invocation.
	//
	// If an invocation is finalized, AbortInvocation does nothing.
	//
	// If you want to abort a specific hung invocation, use this request instead
	// of AbortJob.
	//
	// Requires "scheduler.jobs.abort" permission.
	AbortInvocation(ctx context.Context, in *InvocationRef, opts ...grpc.CallOption) (*emptypb.Empty, error)
	// EmitTriggers puts one or more triggers into pending trigger queues of the
	// specified jobs.
	//
	// This eventually causes jobs to start executing. The scheduler may merge
	// multiple triggers into one job execution, based on how the job is
	// configured.
	//
	// If at least one job doesn't exist or the caller has no permission to
	// trigger it, the entire request is aborted. Otherwise, the request is NOT
	// transactional: if it fails midway (e.g. by returning internal server error),
	// some triggers may have been submitted and some may not. It is safe to retry
	// the call, supplying the same trigger IDs. Triggers with the same IDs will
	// be deduplicated. See Trigger message for more details.
	//
	// Requires "scheduler.jobs.trigger" permission.
	EmitTriggers(ctx context.Context, in *EmitTriggersRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
}

type schedulerClient struct {
	cc grpc.ClientConnInterface
}

func NewSchedulerClient(cc grpc.ClientConnInterface) SchedulerClient {
	return &schedulerClient{cc}
}

func (c *schedulerClient) GetJobs(ctx context.Context, in *JobsRequest, opts ...grpc.CallOption) (*JobsReply, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(JobsReply)
	err := c.cc.Invoke(ctx, Scheduler_GetJobs_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerClient) GetInvocations(ctx context.Context, in *InvocationsRequest, opts ...grpc.CallOption) (*InvocationsReply, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(InvocationsReply)
	err := c.cc.Invoke(ctx, Scheduler_GetInvocations_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerClient) GetInvocation(ctx context.Context, in *InvocationRef, opts ...grpc.CallOption) (*Invocation, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(Invocation)
	err := c.cc.Invoke(ctx, Scheduler_GetInvocation_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerClient) PauseJob(ctx context.Context, in *JobRef, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, Scheduler_PauseJob_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerClient) ResumeJob(ctx context.Context, in *JobRef, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, Scheduler_ResumeJob_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerClient) AbortJob(ctx context.Context, in *JobRef, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, Scheduler_AbortJob_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerClient) AbortInvocation(ctx context.Context, in *InvocationRef, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, Scheduler_AbortInvocation_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *schedulerClient) EmitTriggers(ctx context.Context, in *EmitTriggersRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(emptypb.Empty)
	err := c.cc.Invoke(ctx, Scheduler_EmitTriggers_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SchedulerServer is the server API for Scheduler service.
// All implementations must embed UnimplementedSchedulerServer
// for forward compatibility.
//
// Scheduler exposes public API of the Scheduler service.
type SchedulerServer interface {
	// GetJobs fetches all jobs satisfying JobsRequest and visibility ACLs.
	//
	// If JobsRequest.project is specified but the project doesn't exist, empty
	// list of Jobs is returned.
	//
	// A job is visible if the caller has "scheduler.jobs.get" permission.
	GetJobs(context.Context, *JobsRequest) (*JobsReply, error)
	// GetInvocations fetches invocations of a given job, most recent first.
	//
	// Requires "scheduler.jobs.get" permission.
	GetInvocations(context.Context, *InvocationsRequest) (*InvocationsReply, error)
	// GetInvocation fetches a single invocation.
	//
	// Requires "scheduler.jobs.get" permission.
	GetInvocation(context.Context, *InvocationRef) (*Invocation, error)
	// PauseJob will prevent automatic triggering of a job.
	//
	// Manual triggering (e.g. via EmitTriggers RPC) is still allowed. Any pending
	// or running invocations are still executed.
	//
	// PauseJob does nothing if job is already paused.
	//
	// Requires "scheduler.jobs.pause" permission.
	PauseJob(context.Context, *JobRef) (*emptypb.Empty, error)
	// ResumeJob resumes paused job.
	//
	// ResumeJob does nothing if job is not paused.
	//
	// Requires "scheduler.jobs.resume" permission.
	ResumeJob(context.Context, *JobRef) (*emptypb.Empty, error)
	// AbortJob resets the job to scheduled state, aborting a currently pending or
	// running invocation if any.
	//
	// Note, that this is similar to AbortInvocation except that AbortInvocation
	// requires invocation ID and doesn't ensure that the invocation aborted is
	// actually latest triggered for the job.
	//
	// Requires "scheduler.jobs.abort" permission.
	AbortJob(context.Context, *JobRef) (*emptypb.Empty, error)
	// AbortInvocation aborts a given job invocation.
	//
	// If an invocation is finalized, AbortInvocation does nothing.
	//
	// If you want to abort a specific hung invocation, use this request instead
	// of AbortJob.
	//
	// Requires "scheduler.jobs.abort" permission.
	AbortInvocation(context.Context, *InvocationRef) (*emptypb.Empty, error)
	// EmitTriggers puts one or more triggers into pending trigger queues of the
	// specified jobs.
	//
	// This eventually causes jobs to start executing. The scheduler may merge
	// multiple triggers into one job execution, based on how the job is
	// configured.
	//
	// If at least one job doesn't exist or the caller has no permission to
	// trigger it, the entire request is aborted. Otherwise, the request is NOT
	// transactional: if it fails midway (e.g. by returning internal server error),
	// some triggers may have been submitted and some may not. It is safe to retry
	// the call, supplying the same trigger IDs. Triggers with the same IDs will
	// be deduplicated. See Trigger message for more details.
	//
	// Requires "scheduler.jobs.trigger" permission.
	EmitTriggers(context.Context, *EmitTriggersRequest) (*emptypb.Empty, error)
	mustEmbedUnimplementedSchedulerServer()
}

// UnimplementedSchedulerServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedSchedulerServer struct{}

func (UnimplementedSchedulerServer) GetJobs(context.Context, *JobsRequest) (*JobsReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetJobs not implemented")
}
func (UnimplementedSchedulerServer) GetInvocations(context.Context, *InvocationsRequest) (*InvocationsReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetInvocations not implemented")
}
func (UnimplementedSchedulerServer) GetInvocation(context.Context, *InvocationRef) (*Invocation, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetInvocation not implemented")
}
func (UnimplementedSchedulerServer) PauseJob(context.Context, *JobRef) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method PauseJob not implemented")
}
func (UnimplementedSchedulerServer) ResumeJob(context.Context, *JobRef) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ResumeJob not implemented")
}
func (UnimplementedSchedulerServer) AbortJob(context.Context, *JobRef) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method AbortJob not implemented")
}
func (UnimplementedSchedulerServer) AbortInvocation(context.Context, *InvocationRef) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method AbortInvocation not implemented")
}
func (UnimplementedSchedulerServer) EmitTriggers(context.Context, *EmitTriggersRequest) (*emptypb.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method EmitTriggers not implemented")
}
func (UnimplementedSchedulerServer) mustEmbedUnimplementedSchedulerServer() {}
func (UnimplementedSchedulerServer) testEmbeddedByValue()                   {}

// UnsafeSchedulerServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to SchedulerServer will
// result in compilation errors.
type UnsafeSchedulerServer interface {
	mustEmbedUnimplementedSchedulerServer()
}

func RegisterSchedulerServer(s grpc.ServiceRegistrar, srv SchedulerServer) {
	// If the following call pancis, it indicates UnimplementedSchedulerServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&Scheduler_ServiceDesc, srv)
}

func _Scheduler_GetJobs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(JobsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServer).GetJobs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Scheduler_GetJobs_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServer).GetJobs(ctx, req.(*JobsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_GetInvocations_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InvocationsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServer).GetInvocations(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Scheduler_GetInvocations_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServer).GetInvocations(ctx, req.(*InvocationsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_GetInvocation_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InvocationRef)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServer).GetInvocation(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Scheduler_GetInvocation_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServer).GetInvocation(ctx, req.(*InvocationRef))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_PauseJob_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(JobRef)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServer).PauseJob(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Scheduler_PauseJob_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServer).PauseJob(ctx, req.(*JobRef))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_ResumeJob_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(JobRef)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServer).ResumeJob(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Scheduler_ResumeJob_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServer).ResumeJob(ctx, req.(*JobRef))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_AbortJob_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(JobRef)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServer).AbortJob(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Scheduler_AbortJob_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServer).AbortJob(ctx, req.(*JobRef))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_AbortInvocation_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InvocationRef)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServer).AbortInvocation(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Scheduler_AbortInvocation_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServer).AbortInvocation(ctx, req.(*InvocationRef))
	}
	return interceptor(ctx, in, info, handler)
}

func _Scheduler_EmitTriggers_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(EmitTriggersRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SchedulerServer).EmitTriggers(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Scheduler_EmitTriggers_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(SchedulerServer).EmitTriggers(ctx, req.(*EmitTriggersRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Scheduler_ServiceDesc is the grpc.ServiceDesc for Scheduler service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Scheduler_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "scheduler.Scheduler",
	HandlerType: (*SchedulerServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetJobs",
			Handler:    _Scheduler_GetJobs_Handler,
		},
		{
			MethodName: "GetInvocations",
			Handler:    _Scheduler_GetInvocations_Handler,
		},
		{
			MethodName: "GetInvocation",
			Handler:    _Scheduler_GetInvocation_Handler,
		},
		{
			MethodName: "PauseJob",
			Handler:    _Scheduler_PauseJob_Handler,
		},
		{
			MethodName: "ResumeJob",
			Handler:    _Scheduler_ResumeJob_Handler,
		},
		{
			MethodName: "AbortJob",
			Handler:    _Scheduler_AbortJob_Handler,
		},
		{
			MethodName: "AbortInvocation",
			Handler:    _Scheduler_AbortInvocation_Handler,
		},
		{
			MethodName: "EmitTriggers",
			Handler:    _Scheduler_EmitTriggers_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "go.chromium.org/luci/scheduler/api/scheduler/v1/scheduler.proto",
}
