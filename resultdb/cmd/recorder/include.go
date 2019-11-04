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
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
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
func (s *recorderServer) Include(ctx context.Context, in *pb.IncludeRequest) (*empty.Empty, error) {
	if err := validateIncludeRequest(in); err != nil {
		return nil, errors.Annotate(err, "bad request").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	including := span.MustParseInvocationName(in.IncludingInvocation)
	included := span.MustParseInvocationName(in.IncludedInvocation)
	var overridden span.InvocationID
	if in.OverrideInvocation != "" {
		overridden = span.MustParseInvocationName(in.OverrideInvocation)
	}

	err := mutateInvocation(ctx, including, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		eg, ctx := errgroup.WithContext(ctx)

		// Ensure the included invocation exists.
		eg.Go(func() (err error) {
			_, err = readInvocationState(ctx, txn, included)
			return
		})

		if overridden != "" {
			// Ensure the overridden inclusion exists and not overridden already.
			// Note that we don't update readiness of the inclusion being
			// overridden.
			eg.Go(func() error {
				return checkOverridingInclusion(ctx, txn, including, included, overridden)
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
			span.InsertMap("Inclusions", map[string]interface{}{
				"InvocationId":         including,
				"IncludedInvocationId": included,
			}),
		}

		if overridden != "" {
			// Mark the existing inclusion as overridden.
			// Note that it must NOT cause an AlreadyExists error since
			// checkOverridingInclusion already ensured that it exists.
			muts = append(muts, span.UpdateMap("Inclusions", map[string]interface{}{
				"InvocationId":                     including,
				"IncludedInvocationId":             overridden,
				"OverriddenByIncludedInvocationId": included,
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
func checkOverridingInclusion(ctx context.Context, txn *spanner.ReadWriteTransaction, including, included, overridden span.InvocationID) error {
	var currentOverriding span.InvocationID
	err := span.ReadRow(ctx, txn, "Inclusions", span.InclusionKey(including, overridden), map[string]interface{}{
		"OverriddenByIncludedInvocationId": &currentOverriding,
	})
	switch {

	case spanner.ErrCode(err) == codes.NotFound:
		return errors.
			Reason("%q does not exist or is not included in %q", overridden.Name(), including.Name()).
			InternalReason("%s", err).
			Tag(grpcutil.NotFoundTag).
			Err()

	case err != nil:
		return err

	case currentOverriding == "":
		// The inclusion is not overridden.
		return nil

	case currentOverriding == included:
		// This makes this request idempotent.
		return errRepeatedRequest

	default:
		return errors.
			Reason(
				"inclusion of %q is already overridden by %q",
				overridden.Name(),
				currentOverriding.Name(),
			).
			Tag(grpcutil.FailedPreconditionTag).
			Err()
	}
}
