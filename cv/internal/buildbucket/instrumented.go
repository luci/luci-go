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

package buildbucket

import (
	"context"

	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cv/internal/metrics"
)

func makeInstrumentedFactory(inner ClientFactory) ClientFactory {
	return instrumentedFactory{ClientFactory: inner}
}

type instrumentedFactory struct {
	ClientFactory
}

// MakeClient implements `ClientFactory`.
func (i instrumentedFactory) MakeClient(ctx context.Context, host, luciProject string) (Client, error) {
	c, err := i.ClientFactory.MakeClient(ctx, host, luciProject)
	if err != nil {
		return nil, err
	}
	return instrumentedClient{
		luciProject: luciProject,
		host:        host,
		inner:       c,
	}, nil
}

// instrumentedClient instruments Buildbucket RPCs.
type instrumentedClient struct {
	luciProject string
	host        string
	inner       Client
}

// start records the start time and returns the func to report the end.
func (i instrumentedClient) start(ctx context.Context, method string) func(err error) error {
	tStart := clock.Now(ctx)
	return func(err error) error {
		dur := clock.Since(ctx, tStart)
		c := status.Code(errors.Unwrap(err))
		canonicalCode, ok := code.Code_name[int32(c)]
		if !ok {
			canonicalCode = c.String() // Code(%d)
		}
		metrics.Internal.BuildbucketRPCCount.Add(ctx, 1, i.luciProject, i.host, method, canonicalCode)
		metrics.Internal.BuildbucketRPCDurations.Add(ctx, float64(dur.Milliseconds()), i.luciProject, i.host, method, canonicalCode)

		return err
	}
}

func (i instrumentedClient) GetBuild(ctx context.Context, in *bbpb.GetBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error) {
	end := i.start(ctx, "GetBuild")
	resp, err := i.inner.GetBuild(ctx, in, opts...)
	return resp, end(err)
}

func (i instrumentedClient) SearchBuilds(ctx context.Context, in *bbpb.SearchBuildsRequest, opts ...grpc.CallOption) (*bbpb.SearchBuildsResponse, error) {
	end := i.start(ctx, "SearchBuilds")
	resp, err := i.inner.SearchBuilds(ctx, in, opts...)
	return resp, end(err)
}

func (i instrumentedClient) CancelBuild(ctx context.Context, in *bbpb.CancelBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error) {
	end := i.start(ctx, "CancelBuild")
	resp, err := i.inner.CancelBuild(ctx, in, opts...)
	return resp, end(err)
}

func (i instrumentedClient) Batch(ctx context.Context, in *bbpb.BatchRequest, opts ...grpc.CallOption) (*bbpb.BatchResponse, error) {
	// LUCI CV doesn't mix different type of operations in the same batch request.
	// It would only have multiple operations that are of the same type.
	var method string
	if len(in.GetRequests()) > 0 { // be defensive
		req := in.GetRequests()[0]
		switch req.GetRequest().(type) {
		case *bbpb.BatchRequest_Request_CancelBuild:
			method = "Batch.CancelBuild"
		case *bbpb.BatchRequest_Request_GetBuild:
			method = "Batch.GetBuild"
		case *bbpb.BatchRequest_Request_SearchBuilds:
			method = "Batch.SearchBuilds"
		case *bbpb.BatchRequest_Request_ScheduleBuild:
			method = "Batch.ScheduleBuild"
		default:
			panic(errors.Fmt("unknown request type: %T", req))
		}
	} else {
		logging.Warningf(ctx, "calling buildbucket.Batch with empty requests")
	}
	end := i.start(ctx, method)
	resp, err := i.inner.Batch(ctx, in, opts...)
	return resp, end(err)
}
