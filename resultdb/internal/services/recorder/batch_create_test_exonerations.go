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

package recorder

import (
	"context"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func validateBatchCreateTestExonerationsRequest(req *pb.BatchCreateTestExonerationsRequest, cfg *config.CompiledServiceConfig) error {
	if err := pbutil.ValidateInvocationName(req.Invocation); err != nil {
		return errors.Annotate(err, "invocation").Err()
	}

	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Annotate(err, "request_id").Err()
	}

	if err := pbutil.ValidateBatchRequestCount(len(req.Requests)); err != nil {
		return err
	}

	for i, sub := range req.Requests {
		if err := validateCreateTestExonerationRequest(sub, cfg, false); err != nil {
			return errors.Annotate(err, "requests[%d]", i).Err()
		}
		if sub.Invocation != "" && sub.Invocation != req.Invocation {
			return errors.Reason("requests[%d]: invocation: inconsistent with top-level invocation", i).Err()
		}
		if sub.RequestId != "" && sub.RequestId != req.RequestId {
			return errors.Reason("requests[%d]: request_id: inconsistent with top-level request_id", i).Err()
		}
	}
	return nil
}

// BatchCreateTestExonerations implements pb.RecorderServer.
func (s *recorderServer) BatchCreateTestExonerations(ctx context.Context, in *pb.BatchCreateTestExonerationsRequest) (rsp *pb.BatchCreateTestExonerationsResponse, err error) {
	cfg, err := config.Service(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateBatchCreateTestExonerationsRequest(in, cfg); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	invID := invocations.MustParseName(in.Invocation)

	ret := &pb.BatchCreateTestExonerationsResponse{
		TestExonerations: make([]*pb.TestExoneration, len(in.Requests)),
	}
	ms := make([]*spanner.Mutation, len(in.Requests))
	for i, sub := range in.Requests {
		ret.TestExonerations[i], ms[i] = insertTestExoneration(ctx, invID, in.RequestId, i, sub.TestExoneration)
	}
	_, err = mutateInvocation(ctx, invID, func(ctx context.Context) error {
		span.BufferWrite(ctx, ms...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}
