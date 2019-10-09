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

	ret := &pb.Inclusion{
		Name:               pbutil.InclusionName(includingInvID, includedInvID),
		IncludedInvocation: in.Inclusion.IncludedInvocation,
	}
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		// Read from Spanner concurrently.
		eg, ctx := errgroup.WithContext(ctx)

		// Check invocation state again.
		eg.Go(func() error {
			return mayMutateInvocation(ctx, txn, includingInvID)
		})

		// Compute ret.Ready.
		eg.Go(func() (err error) {
			includedInvState, err := readInvocationState(ctx, txn, includedInvID)
			if err != nil {
				return err
			}
			ret.Ready = pbutil.IsFinalized(includedInvState)
			return nil
		})
		if err := eg.Wait(); err != nil {
			return err
		}

		// Write to Spanner concurrently.
		return txn.BufferWrite([]*spanner.Mutation{
			// Use InsertOrUpdate instead of Insert to ensure the request is
			// idempotent.
			spanner.InsertOrUpdateMap("Inclusions", map[string]interface{}{
				"InvocationID":         includingInvID,
				"IncludedInvocationID": includedInvID,
				"Ready":                ret.Ready,
			}),
		})
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func readInvocationState(ctx context.Context, txn *spanner.ReadWriteTransaction, invID string) (pb.Invocation_State, error) {
	var state int64
	err := span.ReadInvocation(ctx, txn, invID, map[string]interface{}{"State": &state})
	if err != nil {
		return 0, err
	}
	return pb.Invocation_State(state), nil
}
