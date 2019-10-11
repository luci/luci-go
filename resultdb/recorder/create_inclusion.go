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
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// CreateInclusion implements pb.RecorderServer.
func (s *RecorderServer) CreateInclusion(ctx context.Context, in *pb.CreateInclusionRequest) (*pb.Inclusion, error) {
	if err := pbutil.ValidateCreateInclusionRequest(in); err != nil {
		return nil, errors.Annotate(err, "bad request").Tag(grpcutil.InvalidArgumentTag).Err()
	}
	var (
		includingInvID = pbutil.MustParseInvocationName(in.IncludingInvocation)
		includedInvID  = pbutil.MustParseInvocationName(in.Inclusion.IncludedInvocation)
	)

	// Check permissions before opening a RW txn.
	client := span.Client(ctx)
	if err := mayMutateInvocation(ctx, client.Single(), includingInvID); err != nil {
		return nil, err
	}

	// Do not reuse in.Inclusion as it may have random garbage.
	ret := &pb.Inclusion{
		Name:               pbutil.InclusionName(includingInvID, includedInvID),
		IncludedInvocation: in.Inclusion.IncludedInvocation,
	}
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		eg, ctx := errgroup.WithContext(ctx)

		// Check invocation state again.
		eg.Go(func() error {
			return mayMutateInvocation(ctx, txn, includingInvID)
		})

		// Compute ret.Ready.
		eg.Go(func() error {
			includedInvState, err := readInvocationState(ctx, txn, includedInvID)
			ret.Ready = pbutil.IsFinalized(includedInvState)
			return errors.Annotate(err, "invocation %q", pbutil.InvocationName((includedInvID))).Err()
		})

		if err := eg.Wait(); err != nil {
			return err
		}

		return txn.BufferWrite([]*spanner.Mutation{
			// We want idempotency, and we achieve it by using InsertOrUpdate
			// instead of Insert, such that we don't have to check for errors
			// in case inclusion is already there.
			spanner.InsertOrUpdateMap("Inclusions", map[string]interface{}{
				"InvocationID":         includingInvID,
				"IncludedInvocationID": includedInvID,
				"Ready":                ret.Ready,
			}),
		})
	})
	return ret, err
}

func readInvocationState(ctx context.Context, txn *spanner.ReadWriteTransaction, invID string) (pb.Invocation_State, error) {
	var state int64
	err := span.ReadInvocation(ctx, txn, invID, map[string]interface{}{"State": &state})
	return pb.Invocation_State(state), err
}
