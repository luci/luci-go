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

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// validateFinalizeInvocationRequest returns a non-nil error if req is determined
// to be invalid.
func validateFinalizeInvocationRequest(req *pb.FinalizeInvocationRequest) error {
	if _, err := pbutil.ParseInvocationName(req.Name); err != nil {
		return errors.Annotate(err, "name").Err()
	}

	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Annotate(err, "request_id").Err()
	}

	return nil
}

// FinalizeInvocation implements pb.RecorderServer.
func (s *recorderServer) FinalizeInvocation(ctx context.Context, in *pb.FinalizeInvocationRequest) (*pb.Invocation, error) {
	if err := validateFinalizeInvocationRequest(in); err != nil {
		return nil, errors.Annotate(err, "bad request").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	invID := pbutil.MustParseInvocationName(in.Name)
	var ret *pb.Invocation

	if in.RequestId != "" {
		// Check if the request is a duplicate.
		var err error
		_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
			var curRequestID spanner.NullString
			err := span.ReadInvocation(ctx, txn, invID, map[string]interface{}{
				"FinalizeRequestId": &curRequestID,
			})

			if err != nil {
				return err
			}

			if curRequestID.Valid && curRequestID.StringVal == in.RequestId {
				// Request is confirmed to be a duplicate.
				ret, err = span.ReadInvocationFull(ctx, txn, invID)

				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}

		if pbutil.IsFinalized(ret.State) {
			// Confirm the invocation is actually being finalized and return
			// it as is.
			return ret, nil
		}
	}

	err := mutateInvocation(ctx, invID, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		var err error
		if ret, err = span.ReadInvocationFull(ctx, txn, invID); err != nil {
			return err
		}

		ret.State = pb.Invocation_COMPLETED
		if in.Interrupted {
			ret.State = pb.Invocation_INTERRUPTED
		}

		ret.FinalizeTime = pbutil.MustTimestampProto(clock.Now(ctx))

		// TODO(chanli): Also update all inclusions that include this invocation.
		return txn.BufferWrite([]*spanner.Mutation{
			spanner.UpdateMap("Invocations", span.ToSpannerMap(map[string]interface{}{
				"InvocationId": invID,
				"State":        ret.State,
				"FinalizeTime": ret.FinalizeTime,
			})),
		})
	})

	if err != nil {
		return nil, err
	}
	return ret, nil
}
