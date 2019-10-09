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

	"golang.org/x/sync/errgroup"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

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
		// Read from Spanner concurrently.
		eg, ctx := errgroup.WithContext(ctx)

		// Check invocation state again.
		eg.Go(func() error {
			return mayMutateInvocation(ctx, txn, includingInvID)
		})

		// Compute ret.OverridingInclusion.Ready
		eg.Go(func() error {
			state, err := readInvocationState(ctx, txn, overridingInvID)
			if err != nil {
				return err
			}
			ret.OverridingInclusion.Ready = pbutil.IsFinalized(state)
			return nil
		})

		// Compute ret.OverriddenInclusion.Ready
		eg.Go(func() error {
			state, err := readInvocationState(ctx, txn, overriddenInvID)
			if err != nil {
				return err
			}
			ret.OverriddenInclusion.Ready = pbutil.IsFinalized(state)
			return nil
		})

		// Ensure the overriden inclusion exists and not overriden already.
		repeatedRequest := false
		eg.Go(func() (err error) {
			repeatedRequest, err = checkOverridingInclusion(ctx, txn, includingInvID, overriddenInvID, overridingInvID)
			return
		})
		switch err := eg.Wait(); {
		case err != nil:
			return err
		case repeatedRequest:
			// No need to write anything.
			return nil
		}

		// Write to Spanner.
		return txn.BufferWrite([]*spanner.Mutation{
			// Mark the existing inclusion as overridden.
			// Use spanner.Update to check if it does not exist yet.
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
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// checkOverridingInclusion checks whether the inclusion being overriden already exists
// and it is not overriden by any other inclusion.
// If it is already overriden by overridingInvID, repeatedRequest is true.
func checkOverridingInclusion(ctx context.Context, txn *spanner.ReadWriteTransaction, includingInvID, overriddenInvID, overridingInvID string) (repeatedRequest bool, err error) {
	var actualOverridingInvID spanner.NullString
	switch err := span.ReadRow(ctx, txn, "Inclusions", spanner.Key{includingInvID, overriddenInvID}, map[string]interface{}{"OverridenByIncludedInvocationID": &actualOverridingInvID}); {

	case spanner.ErrCode(err) == codes.NotFound:
		return false, errors.Reason("inclusion %q not found", pbutil.InclusionName(includingInvID, overriddenInvID)).
			InternalReason("%s", err).
			Tag(grpcutil.NotFoundTag).
			Err()

	case err != nil:
		return false, err

	case !actualOverridingInvID.Valid:
		// The inclusion is not overridden.
		return false, nil

	case actualOverridingInvID.StringVal == overridingInvID:
		// This makes this request idempotent.
		return true, nil

	default:
		return false, errors.
			Reason("inclusion %q is already overriden by %q", pbutil.InclusionName(includingInvID, overriddenInvID), pbutil.InclusionName(includingInvID, actualOverridingInvID.StringVal)).
			Tag(grpcutil.FailedPreconditionTag).
			Err()
	}

}
