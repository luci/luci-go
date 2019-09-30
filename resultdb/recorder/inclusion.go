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
	"go.chromium.org/luci/resultdb/pbutil"
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/grpc/grpcutil"

	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/storage"
)

// CreateInclusion implements pb.RecorderServer.
func (s *RecorderServer) CreateInclusion(ctx context.Context, in *pb.CreateInclusionRequest) (*pb.Inclusion, error) {
	if err := pbutil.ValidateCreateInclusionRequest(req); err != nil {
		return errors.Anonotate(err, "bad request", grpcutil.InvalidArgumentTag).Err()
	}

	includingInvID, err := pbutil.MustParseInvocationName(in.IncludingInvocation)
	includedInvID, err := pbutil.MustParseInvocationName(in.Inclusion.IncludedInvocation)

	err := storage.Client(ctx).ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		if err := mayMutateInvocation(ctx, tx, in.Parent); err != nil {
			return err
		}

		insertInv := spanner.InsertOrUpdate(
			"Inclusions",
			[]string{"InvocationID", "IncludedInvocationID"},
			[]interface{includingInvId, includedInvID},
		)
		return tx.BufferWrite([]*spanner.Mutation{insertInv})
	})
	if err != nil {
		return err
	}

	return &pb.Inclusion{
		Name: pbutil.InclusionName(includingInvID, includedInvID),
	}, nil
}

// OverrideInclusion implements pb.RecorderServer.
func (s *RecorderServer) OverrideInclusion(ctx context.Context, in *pb.OverrideInclusionRequest) (*pb.OverrideInclusionResponse, error) {
	if err := pbutil.ValidateOverrideInclusionRequest(req); err != nil {
		return errors.Anonotate(err, "bad request", grpcutil.InvalidArgumentTag).Err()
	}

	includingInvID := pbutil.MustParseInvocationName(in.IncludingInvocation)
	overridingIncludedInvID := pbutil.MustParseInvocationName(in.Inclusion.IncludedInvocation)
	overriddenIncludedInvID := pbutil.MustParseInvocationName(in.OverriddenIncludedInvocation)

	err := storage.Client(ctx).ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		if err := mayMutateInvocation(ctx, tx, in.Parent); err != nil {
			return err
		}

		// Mark the existing inclusion as overridden.
		overrideInv := spanner.Update(
			"Inclusions",
			[]string{"InvocationID", "IncludedInvocationID", "OverridenBy"},
			[]interface{includingInvId, includedInvID, overridingIncludedInvID},
		)

		// Ensure the overriding inclusion exists.
		insertInv := spanner.InsertOrUpdate(
			"Inclusions",
			[]string{"InvocationID", "IncludedInvocationID"},
			[]interface{includingInvId, includedInvID},
		)
		return tx.BufferWrite([]*spanner.Mutation{insertInv, insertInv})
	})
	if err != nil {
		return err
	}

	return &pb.Inclusion{
		Name: pbutil.InclusionName(includingInvID, overriddenIncludedInvID),
		OverriddenBy: pbutil.InclusionName(includingInvID, overridingIncludedInvID),
	}, nil
}
