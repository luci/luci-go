// Code generated by svcdec; DO NOT EDIT.

package sinkpb

import (
	"context"

	proto "github.com/golang/protobuf/proto"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type DecoratedSink struct {
	// Service is the service to decorate.
	Service SinkServer
	// Prelude is called for each method before forwarding the call to Service.
	// If Prelude returns an error, then the call is skipped and the error is
	// processed via the Postlude (if one is defined), or it is returned directly.
	Prelude func(ctx context.Context, methodName string, req proto.Message) (context.Context, error)
	// Postlude is called for each method after Service has processed the call, or
	// after the Prelude has returned an error. This takes the Service's
	// response proto (which may be nil) and/or any error. The decorated
	// service will return the response (possibly mutated) and error that Postlude
	// returns.
	Postlude func(ctx context.Context, methodName string, rsp proto.Message, err error) error
}

func (s *DecoratedSink) ReportTestResults(ctx context.Context, req *ReportTestResultsRequest) (rsp *ReportTestResultsResponse, err error) {
	if s.Prelude != nil {
		var newCtx context.Context
		newCtx, err = s.Prelude(ctx, "ReportTestResults", req)
		if err == nil {
			ctx = newCtx
		}
	}
	if err == nil {
		rsp, err = s.Service.ReportTestResults(ctx, req)
	}
	if s.Postlude != nil {
		err = s.Postlude(ctx, "ReportTestResults", rsp, err)
	}
	return
}

func (s *DecoratedSink) ReportInvocationLevelArtifacts(ctx context.Context, req *ReportInvocationLevelArtifactsRequest) (rsp *emptypb.Empty, err error) {
	if s.Prelude != nil {
		var newCtx context.Context
		newCtx, err = s.Prelude(ctx, "ReportInvocationLevelArtifacts", req)
		if err == nil {
			ctx = newCtx
		}
	}
	if err == nil {
		rsp, err = s.Service.ReportInvocationLevelArtifacts(ctx, req)
	}
	if s.Postlude != nil {
		err = s.Postlude(ctx, "ReportInvocationLevelArtifacts", rsp, err)
	}
	return
}

func (s *DecoratedSink) UpdateInvocation(ctx context.Context, req *UpdateInvocationRequest) (rsp *Invocation, err error) {
	if s.Prelude != nil {
		var newCtx context.Context
		newCtx, err = s.Prelude(ctx, "UpdateInvocation", req)
		if err == nil {
			ctx = newCtx
		}
	}
	if err == nil {
		rsp, err = s.Service.UpdateInvocation(ctx, req)
	}
	if s.Postlude != nil {
		err = s.Postlude(ctx, "UpdateInvocation", rsp, err)
	}
	return
}
