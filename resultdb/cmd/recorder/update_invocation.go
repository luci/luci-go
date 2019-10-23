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

package main

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// validateUpdateInvocationRequest returns non-nil error if req is invalid.
func validateUpdateInvocationRequest(req *pb.UpdateInvocationRequest, now time.Time) error {
	if err := pbutil.ValidateInvocationName(req.Invocation.GetName()); err != nil {
		return errors.Annotate(err, "invocation: name").Err()
	}

	if len(req.UpdateMask.GetPaths()) == 0 {
		return errors.Reason("update_mask: paths is empty").Err()
	}

	for _, path := range req.UpdateMask.GetPaths() {
		switch path {
		// The cases in this switch statement must be synchronized with a
		// similar switch statement in UpdateInvocation implementaiton.

		case "deadline":
			if err := validateInvocationDeadline(req.Invocation.GetDeadline(), now); err != nil {
				return errors.Annotate(err, "invocation: deadline").Err()
			}

		default:
			return errors.Reason("update_mask: unsupported path %q", path).Err()
		}
	}

	return nil
}

// UpdateInvocation implements pb.RecorderServer.
func (s *recorderServer) UpdateInvocation(ctx context.Context, in *pb.UpdateInvocationRequest) (*pb.Invocation, error) {
	if err := validateUpdateInvocationRequest(in, clock.Now(ctx)); err != nil {
		return nil, errors.Annotate(err, "bad request").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	invID := pbutil.MustParseInvocationName(in.Invocation.Name)

	var ret *pb.Invocation
	err := mutateInvocation(ctx, invID, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		var err error
		if ret, err = span.ReadInvocationFull(ctx, txn, invID); err != nil {
			return err
		}

		values := map[string]interface{}{
			"InvocationId": invID,
		}

		for _, path := range in.UpdateMask.Paths {
			switch path {
			// The cases in this switch statement must be synchronized with a
			// similar switch statement in validateUpdateInvocationRequest.

			case "deadline":
				values["Deadline"] = in.Invocation.Deadline
				ret.Deadline = in.Invocation.Deadline

			default:
				panic("impossible")
			}
		}

		return txn.BufferWrite([]*spanner.Mutation{spanner.UpdateMap("Invocations", span.ToSpannerMap(values))})
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}
