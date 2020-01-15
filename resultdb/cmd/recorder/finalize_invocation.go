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
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/appstatus"
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

	return nil
}

func getUnmatchedInterruptedFlagError(invID span.InvocationID) error {
	return appstatus.Errorf(codes.FailedPrecondition, "%s has already been finalized with different interrupted flag", invID.Name())
}

// FinalizeInvocation implements pb.RecorderServer.
func (s *recorderServer) FinalizeInvocation(ctx context.Context, in *pb.FinalizeInvocationRequest) (*pb.Invocation, error) {
	if err := validateFinalizeInvocationRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	userToken, err := extractUserUpdateToken(ctx)
	if err != nil {
		return nil, err
	}

	invID := span.MustParseInvocationName(in.Name)

	ret := &pb.Invocation{Name: in.Name}
	var retErr error

	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		now := clock.Now(ctx)

		var updateToken spanner.NullString

		err = span.ReadInvocation(ctx, txn, invID, map[string]interface{}{
			"UpdateToken":  &updateToken,
			"State":        &ret.State,
			"Interrupted":  &ret.Interrupted,
			"CreateTime":   &ret.CreateTime,
			"FinalizeTime": &ret.FinalizeTime,
			"Deadline":     &ret.Deadline,
			"Tags":         &ret.Tags,
		})
		if err != nil {
			return err
		}

		finalizeTime := now
		deadline := pbutil.MustTimestamp(ret.Deadline)
		switch {
		case ret.State == pb.Invocation_COMPLETED && ret.Interrupted == in.Interrupted:
			// Idempotent.
			return nil
		case ret.State == pb.Invocation_COMPLETED && ret.Interrupted != in.Interrupted:
			return getUnmatchedInterruptedFlagError(invID)
		case deadline.Before(now):
			ret.State = pb.Invocation_COMPLETED
			ret.FinalizeTime = ret.Deadline
			ret.Interrupted = true
			finalizeTime = deadline

			if !in.Interrupted {
				retErr = getUnmatchedInterruptedFlagError(invID)
			}
		default:
			// Finalize as requested.
			ret.State = pb.Invocation_COMPLETED
			ret.FinalizeTime = pbutil.MustTimestampProto(now)
			ret.Interrupted = in.Interrupted
		}

		if err = validateUserUpdateToken(updateToken, userToken); err != nil {
			return err
		}

		return finalizeInvocation(ctx, txn, invID, in.Interrupted, finalizeTime)
	})

	if err != nil {
		return nil, err
	}

	if retErr != nil {
		return nil, retErr
	}

	return ret, nil
}
