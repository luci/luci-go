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
	"github.com/golang/protobuf/ptypes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"
	"google.golang.org/grpc/codes"

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

// FinalizeInvocation implements pb.RecorderServer.
func (s *recorderServer) FinalizeInvocation(ctx context.Context, in *pb.FinalizeInvocationRequest) (*pb.Invocation, error) {
	if err := validateFinalizeInvocationRequest(in); err != nil {
		return nil, errors.Annotate(err, "bad request").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	invID := pbutil.MustParseInvocationName(in.Name)
	requestState := pb.Invocation_COMPLETED
	if in.Interrupted {
		requestState = pb.Invocation_INTERRUPTED
	}

	ret := &pb.Invocation{Name: pbutil.InvocationName(invID)}
	var retErr error

	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		userToken, err := extractUserUpdateToken(ctx)
		if err != nil {
			return err
		}

		now := clock.Now(ctx)

		var updateToken spanner.NullString
		var deadline time.Time

		if err = span.ReadInvocation(ctx, txn, invID, map[string]interface{}{
			"UpdateToken":        &updateToken,
			"State":              &ret.State,
			"CreateTime":         &ret.CreateTime,
			"FinalizeTime":       &ret.FinalizeTime,
			"Deadline":           &ret.Deadline,
			"BaseTestVariantDef": &ret.BaseTestVariantDef,
			"Tags":               &ret.Tags,
		}); err != nil {
			if spanner.ErrCode(err) == codes.NotFound {
				return errors.Reason("%q not found", pbutil.InvocationName(invID)).Tag(grpcutil.NotFoundTag).Err()
			}

			return err
		}

		if deadline, err = ptypes.Timestamp(ret.Deadline); err != nil {
			return err
		}

		switch {
		case ret.State == requestState:
			// Idempotent.
			return nil

		case ret.State != pb.Invocation_ACTIVE:
			return errors.Reason("%q has already been finalized with different state.", pbutil.InvocationName(invID)).Tag(grpcutil.FailedPreconditionTag).Err()

		case deadline.Before(now):
			ret.State = pb.Invocation_INTERRUPTED
			ret.FinalizeTime = ret.Deadline

			if !in.Interrupted {
				retErr = errors.Reason("%q has already been finalized with different state.", pbutil.InvocationName(invID)).Tag(grpcutil.FailedPreconditionTag).Err()
			}
		default:
			// Finalize as requested.
			ret.State = requestState
			ret.FinalizeTime = pbutil.MustTimestampProto(now)
		}

		if err = validateUserUpdateToken(updateToken, userToken); err != nil {
			return err
		}

		// TODO(chanli): Also update all inclusions that include this invocation.
		return finalizeInvocation(txn, invID, in.Interrupted, ret.FinalizeTime)
	})

	if err != nil {
		return nil, err
	}

	if retErr != nil {
		return nil, retErr
	}

	return ret, nil
}
