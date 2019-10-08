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

	includingInvID := pbutil.MustParseInvocationName(in.IncludingInvocation)
	includedInvID := pbutil.MustParseInvocationName(in.Inclusion.IncludedInvocation)

	ret := &pb.Inclusion{
		Name:               pbutil.InclusionName(includingInvID, includedInvID),
		IncludedInvocation: in.Inclusion.IncludedInvocation,
	}

	_, err := span.Client(ctx).ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		if err := mayMutateInvocation(ctx, txn, includingInvID); err != nil {
			return err
		}

		// Read the state of the included invocation to determine inclusion readiness.
		// TODO(nodir): unhardcode Spanner table and column names.
		// TODO(nodir): generalize reading invocations.
		includedRow, err := txn.ReadRow(ctx, "Invocations", spanner.Key{includedInvID}, []string{"State"})
		switch {
		case spanner.ErrCode(err) == codes.NotFound:
			return errors.Reason("invocation %q not found", in.Inclusion.IncludedInvocation).
				InternalReason("%s", err).
				Tag(grpcutil.NotFoundTag).
				Err()
		case err != nil:
			return errors.Annotate(err, "failed to retrieved included invocation").Err()
		}
		var includedInvState int64
		if err := includedRow.Columns(&includedInvState); err != nil {
			return err
		}
		ret.Ready = pbutil.IsFinalized(pb.Invocation_State(includedInvState))

		return txn.BufferWrite([]*spanner.Mutation{
			// Use InsertOrUpdate instead of Insert to ensure the request is
			// idempotent.
			spanner.InsertOrUpdate(
				"Inclusions",
				[]string{"InvocationID", "IncludedInvocationID", "Ready"},
				[]interface{}{includingInvID, includedInvID, ret.Ready},
			),
		})
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}
