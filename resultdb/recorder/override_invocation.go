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
	"fmt"

	"golang.org/x/sync/errgroup"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

var errRepeatedRequest = fmt.Errorf("this request was handled before")

// OverrideInclusion implements pb.RecorderServer.
func (s *RecorderServer) OverrideInclusion(ctx context.Context, in *pb.OverrideInclusionRequest) (*pb.OverrideInclusionResponse, error) {
	if err := pbutil.ValidateOverrideInclusionRequest(in); err != nil {
		return nil, errors.Annotate(err, "bad request").Tag(grpcutil.InvalidArgumentTag).Err()
	}
	var (
		includingInvID  = pbutil.MustParseInvocationName(in.IncludingInvocation)
		overridingInvID = pbutil.MustParseInvocationName(in.OverridingIncludedInvocation)
		overriddenInvID = pbutil.MustParseInvocationName(in.OverriddenIncludedInvocation)
	)

	// Check permissions before opening a RW txn.
	client := span.Client(ctx)
	if err := mayMutateInvocation(ctx, client.Single(), includingInvID); err != nil {
		return nil, err
	}

	ret := &pb.OverrideInclusionResponse{
		OverriddenInclusion: &pb.Inclusion{
			Name:               pbutil.InclusionName(includingInvID, overriddenInvID),
			IncludedInvocation: in.OverriddenIncludedInvocation,
		},
		OverridingInclusion: &pb.Inclusion{
			Name:               pbutil.InclusionName(includingInvID, overridingInvID),
			IncludedInvocation: in.OverridingIncludedInvocation,
		},
	}
	_, err := span.Client(ctx).ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		eg, ctx := errgroup.WithContext(ctx)

		// Check invocation state again.
		eg.Go(func() error {
			return mayMutateInvocation(ctx, txn, includingInvID)
		})

		// Compute ret.{Overriding,Overridden}Inclusion.Ready
		computeReady := func(inc *pb.Inclusion, invID string) {
			eg.Go(func() error {
				state, err := readInvocationState(ctx, txn, invID)
				if err != nil {
					return err
				}
				inc.Ready = pbutil.IsFinalized(state)
				return nil
			})
		}
		computeReady(ret.OverriddenInclusion, overriddenInvID)
		computeReady(ret.OverridingInclusion, overridingInvID)

		// Ensure the overriden inclusion exists and not overriden already.
		eg.Go(func() error {
			return checkOverridingInclusion(ctx, txn, includingInvID, overriddenInvID, overridingInvID)
		})

		switch err := eg.Wait(); {
		case err == errRepeatedRequest:
			// No need to write anything.
			return nil

		case err != nil:
			return err
		}

		return txn.BufferWrite([]*spanner.Mutation{
			// Mark the existing inclusion as overridden.
			spanner.UpdateMap("Inclusions", map[string]interface{}{
				"InvocationID":                    includingInvID,
				"IncludedInvocationID":            overriddenInvID,
				"OverridenByIncludedInvocationID": overridingInvID,
				"Ready":                           ret.OverriddenInclusion.Ready,
			}),
			// Ensure the overriding inclusion exists.
			spanner.InsertOrUpdateMap("Inclusions", map[string]interface{}{
				"InvocationID":         includingInvID,
				"IncludedInvocationID": overridingInvID,
				"Ready":                ret.OverridingInclusion.Ready,
			}),
		})
	})
	return ret, err
}

// checkOverridingInclusion checks whether the inclusion being overriden already exists
// and it is not overriden by any other inclusion.
// If it is already overriden by overridingInvID, returns errRepeatedRequest.
func checkOverridingInclusion(ctx context.Context, txn *spanner.ReadWriteTransaction, includingInvID, overriddenInvID, overridingInvID string) error {
	var actualOverridingInvID spanner.NullString
	switch err := span.ReadRow(ctx, txn, "Inclusions", spanner.Key{includingInvID, overriddenInvID}, map[string]interface{}{"OverridenByIncludedInvocationID": &actualOverridingInvID}); {

	case spanner.ErrCode(err) == codes.NotFound:
		return errors.Reason("inclusion %q not found", pbutil.InclusionName(includingInvID, overriddenInvID)).
			InternalReason("%s", err).
			Tag(grpcutil.NotFoundTag).
			Err()

	case err != nil:
		return err

	case !actualOverridingInvID.Valid:
		// The inclusion is not overridden.
		return nil

	case actualOverridingInvID.StringVal == overridingInvID:
		// This makes this request idempotent.
		return errRepeatedRequest

	default:
		return errors.
			Reason("inclusion %q is already overriden by %q", pbutil.InclusionName(includingInvID, overriddenInvID), pbutil.InclusionName(includingInvID, actualOverridingInvID.StringVal)).
			Tag(grpcutil.FailedPreconditionTag).
			Err()
	}
}
