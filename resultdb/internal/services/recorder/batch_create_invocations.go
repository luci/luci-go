// Copyright 2020 The LUCI Authors.
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

package recorder

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/resultdb/internal/appstatus"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// validateBatchCreateInvocationsRequest checks that the individual requests
// are valid, that they match the batch request requestID and that their names
// are not repeated.
func validateBatchCreateInvocationsRequest(
	now time.Time, reqs []*pb.CreateInvocationRequest, requestID string, allowCustomID bool) (span.InvocationIDSet, error) {
	if err := pbutil.ValidateRequestID(requestID); err != nil {
		return nil, errors.Annotate(err, "request_id").Err()
	}
	idSet := make(span.InvocationIDSet, len(reqs))
	for i, req := range reqs {
		if err := validateCreateInvocationRequest(req, now, allowCustomID); err != nil {
			return nil, errors.Annotate(err, "requests[%d]", i).Err()
		}

		// If there's multiple `CreateInvocationRequest`s their request id
		// must either be empty or match the one in the batch request.
		if req.RequestId != "" && req.RequestId != requestID {
			return nil, errors.Reason("requests[%d].request_id: %q does not match request_id %q", i, requestID, req.RequestId).Err()
		}

		invID := span.InvocationID(req.InvocationId)
		if idSet.Has(invID) {
			return nil, errors.Reason("requests[%d].invocation_id: duplicated invocation id %q", i, req.InvocationId).Err()
		}
		idSet.Add(invID)
	}
	return idSet, nil
}

// BatchCreateInvocations implements pb.RecorderServer.
func (s *recorderServer) BatchCreateInvocations(ctx context.Context, in *pb.BatchCreateInvocationsRequest) (*pb.BatchCreateInvocationsResponse, error) {
	now := clock.Now(ctx).UTC()

	allowCustomID, err := auth.IsMember(ctx, customIdGroup)
	if err != nil {
		return nil, err
	}

	idSet, err := validateBatchCreateInvocationsRequest(now, in.Requests, in.RequestId, allowCustomID)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	invs, tokens, err := s.createInvocations(ctx, in.Requests, in.RequestId, now, idSet)
	if err != nil {
		return nil, err
	}
	return &pb.BatchCreateInvocationsResponse{Invocations: invs, UpdateTokens: tokens}, nil
}

// createInvocations is a shared implementation for CreateInvocation and BatchCreateInvocations RPCs.
func (s *recorderServer) createInvocations(ctx context.Context, reqs []*pb.CreateInvocationRequest, requestID string, now time.Time, idSet span.InvocationIDSet) ([]*pb.Invocation, []string, error) {
	muts := s.createInvocationsRequestsToMutations(ctx, now, reqs, requestID)

	var err error
	deduped := false
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		deduped, err = deduplicateCreateInvocations(ctx, txn, idSet, requestID)
		if err != nil {
			return err
		}
		if !deduped {
			return txn.BufferWrite(muts)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if !deduped {
		span.IncRowCount(ctx, len(reqs), span.Invocations, span.Inserted)
	}

	return getCreatedInvocationsAndUpdateTokens(ctx, idSet, reqs)
}

// createInvocationsRequestsToMutations computes a database mutation for
// inserting a row for each invocation creation requested.
func (s *recorderServer) createInvocationsRequestsToMutations(ctx context.Context, now time.Time, reqs []*pb.CreateInvocationRequest, requestID string) []*spanner.Mutation {

	muts := make([]*spanner.Mutation, len(reqs))
	// Compute mutations
	for i, req := range reqs {

		// Prepare the invocation we will save to spanner.
		inv := &pb.Invocation{
			Name:            span.InvocationID(req.InvocationId).Name(),
			State:           pb.Invocation_ACTIVE,
			Deadline:        req.Invocation.GetDeadline(),
			Tags:            req.Invocation.GetTags(),
			BigqueryExports: req.Invocation.GetBigqueryExports(),
		}

		// Ensure the invocation has a deadline.
		if inv.Deadline == nil {
			inv.Deadline = pbutil.MustTimestampProto(now.Add(defaultInvocationDeadlineDuration))
		}

		pbutil.NormalizeInvocation(inv)
		// Create a mutation to create the invocation.
		muts[i] = span.InsertMap("Invocations", s.rowOfInvocation(ctx, inv, requestID))
	}
	return muts
}

// getCreatedInvocationsAndUpdateTokens reads the full details of the
// invocations just created in a separate read-only transaction, and
// generates an update token for each.
func getCreatedInvocationsAndUpdateTokens(ctx context.Context, idSet span.InvocationIDSet, reqs []*pb.CreateInvocationRequest) ([]*pb.Invocation, []string, error) {
	txn := span.Client(ctx).ReadOnlyTransaction()
	defer txn.Close()
	invMap, err := span.ReadInvocationsFull(ctx, txn, idSet)
	if err != nil {
		return nil, nil, err
	}

	// Arrange them in same order as the incoming requests.
	// Ordering is important to match the tokens.
	invs := make([]*pb.Invocation, len(reqs))
	for i, req := range reqs {
		invs[i] = invMap[span.InvocationID(req.InvocationId)]
	}

	tokens, err := generateTokens(ctx, invs)
	if err != nil {
		return nil, nil, err
	}
	return invs, tokens, nil
}

// deduplicateCreateInvocations checks if the invocations have already been
// created with the given requestID. Returns a true if they have.
func deduplicateCreateInvocations(ctx context.Context, txn span.Txn, idSet span.InvocationIDSet, requestID string) (bool, error) {
	invCount := 0
	err := txn.Read(ctx, "Invocations", idSet.Keys(), []string{"InvocationId", "CreateRequestId"}).Do(func(r *spanner.Row) error {
		var invID span.InvocationID
		var rowRequestID spanner.NullString
		if err := span.FromSpanner(r, &invID, &rowRequestID); err != nil {
			return err
		}
		if rowRequestID.IsNull() || rowRequestID.String() != requestID {
			return invocationAlreadyExists(invID)
		}
		invCount++
		return nil
	})
	switch {
	case err != nil:
		return false, err
	case invCount == len(idSet):
		// All invocations were previously created with this request id.
		return true, nil
	case invCount == 0:
		// None of the invocations exist already.
		return false, nil
	default:
		// Could happen if someone sent two different but overlapping batch create
		// requests, but reused the request_id.
		return false, appstatus.Errorf(codes.AlreadyExists, "some, but not all of the invocations already created with this request id")
	}
}

// generateTokens generates an update token for each invocation.
func generateTokens(ctx context.Context, invs []*pb.Invocation) ([]string, error) {
	ret := make([]string, len(invs))
	for i, inv := range invs {
		updateToken, err := generateInvocationToken(ctx, span.MustParseInvocationName(inv.Name))
		if err != nil {
			return nil, err
		}
		ret[i] = updateToken
	}
	return ret, nil
}
