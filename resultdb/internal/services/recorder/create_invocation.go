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

package recorder

import (
	"context"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/ptypes"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/appstatus"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

var (
	// customIdGroup is a CIA group that can create invocations with custom id's.
	customIdGroup = "luci-resultdb-custom-invocation-id"
)

// validateInvocationDeadline returns a non-nil error if deadline is invalid.
func validateInvocationDeadline(deadline *tspb.Timestamp, now time.Time) error {
	internal.AssertUTC(now)
	switch deadline, err := ptypes.Timestamp(deadline); {
	case err != nil:
		return err

	case deadline.Sub(now) < 10*time.Second:
		return errors.Reason("must be at least 10 seconds in the future").Err()

	case deadline.Sub(now) > 2*24*time.Hour:
		return errors.Reason("must be before 48h in the future").Err()

	default:
		return nil
	}
}

// validateCreateInvocationRequest returns an error if req is determined to be
// invalid.
func validateCreateInvocationRequest(req *pb.CreateInvocationRequest, now time.Time, allowCustomID bool) error {
	if err := pbutil.ValidateInvocationID(req.InvocationId); err != nil {
		return errors.Annotate(err, "invocation_id").Err()
	}

	if !strings.HasPrefix(req.InvocationId, "u:") && !allowCustomID {
		return errors.Reason(`invocation_id: an invocation created by a non-LUCI system must have id starting with "u:"; please generate "u:{GUID}"`).Err()
	}

	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Annotate(err, "request_id").Err()
	}

	inv := req.Invocation
	if inv == nil {
		return nil
	}

	if err := pbutil.ValidateStringPairs(inv.GetTags()); err != nil {
		return errors.Annotate(err, "invocation.tags").Err()
	}

	if inv.GetDeadline() != nil {
		if err := validateInvocationDeadline(inv.Deadline, now); err != nil {
			return errors.Annotate(err, "invocation: deadline").Err()
		}
	}

	for i, bqExport := range inv.GetBigqueryExports() {
		if err := pbutil.ValidateBigQueryExport(bqExport); err != nil {
			return errors.Annotate(err, "bigquery_export[%d]", i).Err()
		}
	}

	return nil
}

// CreateInvocation implements pb.RecorderServer.
func (s *recorderServer) CreateInvocation(ctx context.Context, in *pb.CreateInvocationRequest) (*pb.Invocation, error) {
	invs, tokens, err := s.createInvocations(ctx, []*pb.CreateInvocationRequest{in}, in.RequestId)
	if err != nil {
		return nil, err
	}
	if len(invs) != 1 || len(tokens) != 1 {
		panic("createInvocations did not return either an error or a valid invocation/token pair")
	}
	md := metadata.MD{}
	md.Set(UpdateTokenMetadataKey, tokens...)
	prpc.SetHeader(ctx, md)
	return invs[0], nil
}

// BatchCreateInvocations implements pb.RecorderServer.
func (s *recorderServer) BatchCreateInvocations(ctx context.Context, in *pb.BatchCreateInvocationsRequest) (*pb.BatchCreateInvocationsResponse, error) {
	invs, tokens, err := s.createInvocations(ctx, in.Requests, in.RequestId)
	if err != nil {
		return nil, err
	}
	md := metadata.MD{}
	md.Set(UpdateTokenMetadataKey, tokens...)
	prpc.SetHeader(ctx, md)
	return &pb.BatchCreateInvocationsResponse{Invocations: invs}, nil
}

func invocationAlreadyExists(id span.InvocationID) error {
	return appstatus.Errorf(codes.AlreadyExists, "%s already exsts", id.Name())
}

func (s *recorderServer) createInvocations(ctx context.Context, reqs []*pb.CreateInvocationRequest, requestID string) ([]*pb.Invocation, []string, error) {
	now := clock.Now(ctx).UTC()

	allowCustomID, err := auth.IsMember(ctx, customIdGroup)
	if err != nil {
		return nil, nil, err
	}

	muts := make([]*spanner.Mutation, 0, len(reqs))
	idSet := make(span.InvocationIDSet, len(reqs))
	// Validate Requests
	for _, req := range reqs {
		if err := validateCreateInvocationRequest(req, now, allowCustomID); err != nil {
			return nil, nil, appstatus.BadRequest(err)
		}
		// If there's multiple `CreateInvocationRequest`s their request id
		// must either be empty or match the one in the batch request.
		if req.RequestId != "" && req.RequestId != requestID {
			return nil, nil, appstatus.BadRequest(
				errors.Reason("request_id %s does not match %s", req.RequestId, requestID).Err())
		}

		invID := span.InvocationID(req.InvocationId)
		if idSet.Has(invID) {
			return nil, nil, appstatus.BadRequest(
				errors.Reason("duplicate invocation id %s in request", req.InvocationId).Err())
		}
		idSet.Add(invID)

		// Prepare the invocation we will save to spanner.
		inv := &pb.Invocation{
			Name:            invID.Name(),
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
		muts = append(muts, span.InsertMap("Invocations", s.rowOfInvocation(ctx, inv, requestID)))
	}

	deduped := false
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		deduped, err := s.deduplicateCreateInvocations(ctx, idSet, requestID, txn)
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

	// Read the full details of the invocations just created or deduplicated
	// in a separate read-only transaction.
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
func (s *recorderServer) deduplicateCreateInvocations(ctx context.Context, idSet span.InvocationIDSet, requestID string, txn span.Txn) (bool, error) {
	invCount := 0
	err := txn.Read(ctx, "Invocations", idSet.Keys(), []string{"InvocationId", "CreateRequestId"}).Do(func(r *spanner.Row) error {
		var invID span.InvocationID
		var rowRequestID spanner.NullString
		if err := span.FromSpanner(r, &invID, &rowRequestID); err != nil {
			return errors.Annotate(err, "failed to fetch %s", idSet).Err()
		}
		if rowRequestID.IsNull() || rowRequestID.String() != requestID {
			return invocationAlreadyExists(invID)
		}
		invCount += 1
		return nil
	})
	if err != nil {
		return false, err
	}
	if invCount == len(idSet) {
		return true, nil
	} else if invCount != 0 {
		// Some invocations already exist and have the right request id but some are
		// missing, but not all.
		panic("while deduplicating a request to create invocations at least one was unexpectedly missing")
	}
	return false, nil
}

// generateTokens generates an update token for each invocation.
func generateTokens(ctx context.Context, invs []*pb.Invocation) ([]string, error) {
	ret := make([]string, 0, len(invs))
	for _, inv := range invs {
		updateToken, err := generateInvocationToken(ctx, span.MustParseInvocationName(inv.Name))
		if err != nil {
			return nil, err
		}
		ret = append(ret, updateToken)
	}
	return ret, nil
}
