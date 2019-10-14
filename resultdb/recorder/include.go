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

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// validateIncludeRequest returns a non-nil error if req is determined
// to be invalid.
func validateIncludeRequest(req *pb.IncludeRequest) error {
	if _, err := pbutil.ParseInvocationName(req.IncludingInvocation); err != nil {
		return errors.Annotate(err, "including_invocation").Err()
	}

	if _, err := pbutil.ParseInvocationName(req.IncludedInvocation); err != nil {
		return errors.Annotate(err, "included_invocation").Err()
	}

	if req.IncludedInvocation == req.IncludingInvocation {
		return errors.Reason("cannot include itself").Err()
	}

	if req.OverrideInvocation != "" {
		if _, err := pbutil.ParseInvocationName(req.OverrideInvocation); err != nil {
			return errors.Annotate(err, "override_invocation").Err()
		}
		if req.OverrideInvocation == req.IncludedInvocation {
			return errors.Reason("cannot override itself").Err()
		}
		if req.OverrideInvocation == req.IncludingInvocation {
			return errors.Reason("cannot include itself").Err()
		}
	}

	return nil
}

var errRepeatedRequest = fmt.Errorf("this request was handled before")

// Include implements pb.RecorderServer.
func (s *RecorderServer) Include(ctx context.Context, in *pb.IncludeRequest) (*empty.Empty, error) {
	if err := validateIncludeRequest(in); err != nil {
		return nil, errors.Annotate(err, "bad request").Tag(grpcutil.InvalidArgumentTag).Err()
	}
	includingInvID := pbutil.MustParseInvocationName(in.IncludingInvocation)
	includedInvID := pbutil.MustParseInvocationName(in.IncludedInvocation)
	overriddenInvID := ""
	if in.OverrideInvocation != "" {
		overriddenInvID = pbutil.MustParseInvocationName(in.OverrideInvocation)
	}

	// Check permissions before opening a RW txn.
	client := span.Client(ctx)
	if err := mayMutateInvocation(ctx, client.Single(), includingInvID); err != nil {
		return nil, err
	}

	// Now actually mutate state in a RW txn.
	_, err := span.Client(ctx).ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		eg, ctx := errgroup.WithContext(ctx)

		// Check invocation state again.
		eg.Go(func() error {
			return mayMutateInvocation(ctx, txn, includingInvID)
		})

		// Ensure the included invocation exists and also read its state to
		// compute inclusion readiness.
		var ready bool
		eg.Go(func() error {
			state, err := readInvocationState(ctx, txn, includedInvID)
			if err != nil {
				return err
			}
			ready = pbutil.IsFinalized(state)
			return nil
		})

		if overriddenInvID != "" {
			// Ensure the overridden inclusion exists and not overridden already.
			// Note that we don't update readiness of the inclusion being
			// overridden.
			eg.Go(func() error {
				return checkOverridingInclusion(ctx, txn, includingInvID, includedInvID, overriddenInvID)
			})
		}

		switch err := eg.Wait(); {
		case err == errRepeatedRequest:
			// No need to mutate anything.
			return nil

		case err != nil:
			return err
		}

		// Insert a new inclusion.
		// Note that this might cause an AlreadyExists error which is handled
		// below.
		muts := []*spanner.Mutation{
			spanner.InsertMap("Inclusions", map[string]interface{}{
				"InvocationId":         includingInvID,
				"IncludedInvocationId": includedInvID,
				"Ready":                ready,
			}),
		}

		if overriddenInvID != "" {
			// Mark the existing inclusion as overridden.
			// Note that it must NOT cause an AlreadyExists error since
			// checkOverridingInclusion already ensured that it exists.
			muts = append(muts, spanner.UpdateMap("Inclusions", map[string]interface{}{
				"InvocationId":                     includingInvID,
				"IncludedInvocationId":             overriddenInvID,
				"OverriddenByIncludedInvocationId": includedInvID,
			}))
		}
		return txn.BufferWrite(muts)
	})

	if spanner.ErrCode(err) == codes.AlreadyExists {
		// Perhaps this request was served before.
		err = nil
	}
	return &empty.Empty{}, err
}

// checkOverridingInclusion checks whether the inclusion being overridden already exists
// and it is not overridden by any other inclusion.
// If it is already overridden by overridingInvID, returns errRepeatedRequest.
func checkOverridingInclusion(ctx context.Context, txn *spanner.ReadWriteTransaction, includingInvID, includedInvID, overriddenInvID string) error {
	var currentOverridingInvID spanner.NullString
	err := span.ReadRow(ctx, txn, "Inclusions", spanner.Key{includingInvID, overriddenInvID}, map[string]interface{}{
		"OverriddenByIncludedInvocationId": &currentOverridingInvID,
	})
	switch {

	case spanner.ErrCode(err) == codes.NotFound:
		return errors.
			Reason(
				"%q does not exist or is not included in %q",
				pbutil.InvocationName(overriddenInvID),
				pbutil.InvocationName(includingInvID),
			).
			InternalReason("%s", err).
			Tag(grpcutil.NotFoundTag).
			Err()

	case err != nil:
		return err

	case !currentOverridingInvID.Valid:
		// The inclusion is not overridden.
		return nil

	case currentOverridingInvID.StringVal == includedInvID:
		// This makes this request idempotent.
		return errRepeatedRequest

	default:
		return errors.
			Reason(
				"inclusion of %q is already overridden by %q",
				pbutil.InvocationName(overriddenInvID),
				pbutil.InvocationName(currentOverridingInvID.StringVal),
			).
			Tag(grpcutil.FailedPreconditionTag).
			Err()
	}
}

func readInvocationState(ctx context.Context, txn *spanner.ReadWriteTransaction, invID string) (pb.Invocation_State, error) {
	var state int64
	err := span.ReadInvocation(ctx, txn, invID, map[string]interface{}{"State": &state})
	return pb.Invocation_State(state), err
}
