// Copyright 2019 The LUCI Authors.
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

package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/maruel/subcommands"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/buildbucket"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func cmdBatch(p Params) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `batch [flags]`,
		ShortDesc: "calls buildbucket.v2.Builds.Batch",
		LongDesc: doc(`
			Calls buildbucket.v2.Builds.Batch.

			Stdin must be buildbucket.v2.BatchRequest in JSON format.
			Stdout will be buildbucket.v2.BatchResponse in JSON format.
			Exits with code 1 if at least one sub-request fails.
		`),
		CommandRun: func() subcommands.CommandRun {
			r := &batchRun{}
			r.RegisterDefaultFlags(p)
			return r
		},
	}
}

type batchRun struct {
	baseCommandRun
	pb.BatchRequest
}

func (r *batchRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if err := r.initClients(ctx, nil); err != nil {
		return r.done(ctx, err)
	}

	if len(args) != 0 {
		return r.done(ctx, fmt.Errorf("unexpected argument"))
	}

	requestBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		return r.done(ctx, errors.Annotate(err, "failed to read stdin").Err())
	}
	req := &pb.BatchRequest{}
	if err := proto.UnmarshalJSONWithNonStandardFieldMasks(requestBytes, req); err != nil {
		return r.done(ctx, errors.Annotate(err, "failed to parse BatchRequest from stdin").Err())
	}

	// Do not attach the buildbucket token if it's empty or the build is a led build.
	// Because led builds are not real Buildbucket builds and they don't have
	// real buildbucket tokens, so we cannot make them  any builds's parent,
	// even for the builds they scheduled.
	if r.scheduleBuildToken != "" && r.scheduleBuildToken != buildbucket.DummyBuildbucketToken {
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, r.scheduleBuildToken))
	}

	// For led build, also clear out the canOutliveParent fields.
	updateRequest(ctx, req, r.scheduleBuildToken)

	res, err := sendBatchReq(ctx, req, r.buildsClient)
	if err != nil {
		return r.done(ctx, err)
	}

	buf, err := (&protojson.MarshalOptions{Indent: "  "}).Marshal(res)
	if err != nil {
		return r.done(ctx, err)
	}
	if _, err := os.Stdout.Write(buf); err != nil {
		return r.done(ctx, err)
	}

	for _, r := range res.Responses {
		if _, ok := r.Response.(*pb.BatchResponse_Response_Error); ok {
			return 1
		}
	}
	return 0
}

// sendBatchReq sends the Batch request to Buildbucket and handles retries.
func sendBatchReq(ctx context.Context, req *pb.BatchRequest, buildsClient pb.BuildsClient) (*pb.BatchResponse, error) {
	res := &pb.BatchResponse{
		Responses: make([]*pb.BatchResponse_Response, len(req.Requests)),
	}
	idxMap := make([]int, len(req.Requests)) // req.Requests index -> res.Responses index
	for i := 0; i < len(req.Requests); i++ {
		idxMap[i] = i
	}
	var globalErr error
	_ = retry.Retry(ctx, transient.Only(retry.Default), func() error {
		toRetry := make([]*pb.BatchRequest_Request, 0, len(req.Requests))
		idxMapForRetry := make([]int, 0, len(req.Requests))
		results, err := buildsClient.Batch(ctx, req)
		if err != nil {
			globalErr = err
			return nil // buildsClient has already handled top-level retryable errors.
		}
		for i, result := range results.Responses {
			if sts := result.GetError(); sts != nil {
				code := status.FromProto(sts).Code()
				// Should also retry if some sub-requests timed out.
				if grpcutil.IsTransientCode(code) || code == codes.DeadlineExceeded {
					idxMapForRetry = append(idxMapForRetry, idxMap[i])
					toRetry = append(toRetry, req.Requests[i])
				}
			}
			res.Responses[idxMap[i]] = result
		}
		if len(toRetry) > 0 {
			idxMap = idxMapForRetry
			req = &pb.BatchRequest{
				Requests: toRetry,
			}
			return errors.Reason("%d/%d batch subrequests failed", len(toRetry), len(res.Responses)).Tag(transient.Tag).Err()
		}
		return nil
	}, func(err error, d time.Duration) {
		logging.WithError(err).Debugf(ctx, "retrying them in %s...", d)
	})

	return res, globalErr
}

// updateRequest makes changes to the batch request.
// Currently it unsets each sub ScheduleBuild request's CanOutliveParent
// if the token is a dummy token (meaning the build is a led build).
func updateRequest(ctx context.Context, req *pb.BatchRequest, tok string) {
	updated := false
	if tok == buildbucket.DummyBuildbucketToken {
		for _, r := range req.Requests {
			switch r.Request.(type) {
			case *pb.BatchRequest_Request_ScheduleBuild:
				r.GetScheduleBuild().CanOutliveParent = pb.Trinary_UNSET
				updated = true
			default:
				continue
			}
		}
	}
	if updated {
		logging.Infof(ctx, "ScheduleBuildRequest.CanOutliveParent is unset for led build")
	}
}
