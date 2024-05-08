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
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/exportroots"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/services/exportnotifier"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// validateBatchCreateInvocationsRequest checks that the individual requests
// are valid, that they match the batch request requestID and that their names
// are not repeated.
// It also returns an IDSet containing the ids of all the invocations to be
// included in the new invocations.
func validateBatchCreateInvocationsRequest(
	now time.Time, reqs []*pb.CreateInvocationRequest, requestID string) (newInvs, includedInvs invocations.IDSet, err error) {
	if err := pbutil.ValidateRequestID(requestID); err != nil {
		return nil, nil, errors.Annotate(err, "request_id").Err()
	}

	if err := pbutil.ValidateBatchRequestCount(len(reqs)); err != nil {
		return nil, nil, err
	}

	newInvs = make(invocations.IDSet, len(reqs))
	allIncludedIDs := make(invocations.IDSet)
	for i, req := range reqs {
		if err := validateCreateInvocationRequest(req, now, allIncludedIDs); err != nil {
			return nil, nil, errors.Annotate(err, "requests[%d]", i).Err()
		}

		// If there's multiple `CreateInvocationRequest`s their request id
		// must either be empty or match the one in the batch request.
		if req.RequestId != "" && req.RequestId != requestID {
			return nil, nil, errors.Reason("requests[%d].request_id: %q does not match request_id %q", i, requestID, req.RequestId).Err()
		}

		invID := invocations.ID(req.InvocationId)
		if newInvs.Has(invID) {
			return nil, nil, errors.Reason("requests[%d].invocation_id: duplicated invocation id %q", i, req.InvocationId).Err()
		}
		newInvs.Add(invID)
	}

	return newInvs, allIncludedIDs, nil
}

// BatchCreateInvocations implements pb.RecorderServer.
func (s *recorderServer) BatchCreateInvocations(ctx context.Context, in *pb.BatchCreateInvocationsRequest) (*pb.BatchCreateInvocationsResponse, error) {
	now := clock.Now(ctx).UTC()
	for i, r := range in.Requests {
		if err := verifyCreateInvocationPermissions(ctx, r); err != nil {
			return nil, errors.Annotate(err, "requests[%d]", i).Err()
		}

	}

	idSet, includedInvs, err := validateBatchCreateInvocationsRequest(now, in.Requests, in.RequestId)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	includedInvs.RemoveAll(idSet)
	if err := permissions.VerifyInvocations(span.Single(ctx), includedInvs, permIncludeInvocation); err != nil {
		return nil, err
	}

	invs, tokens, err := s.createInvocations(ctx, in.Requests, in.RequestId, now, idSet)
	if err != nil {
		return nil, err
	}
	return &pb.BatchCreateInvocationsResponse{Invocations: invs, UpdateTokens: tokens}, nil
}

// createInvocations is a shared implementation for CreateInvocation and BatchCreateInvocations RPCs.
func (s *recorderServer) createInvocations(ctx context.Context, reqs []*pb.CreateInvocationRequest, requestID string, now time.Time, idSet invocations.IDSet) ([]*pb.Invocation, []string, error) {
	createdBy := string(auth.CurrentIdentity(ctx))

	var err error
	deduped := false
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		deduped, err = deduplicateCreateInvocations(ctx, idSet, requestID, createdBy)
		if err != nil {
			return err
		}
		if !deduped {
			s.createInvocationsInternal(ctx, now, reqs, requestID, createdBy)
		}

		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if !deduped {
		for _, r := range reqs {
			spanutil.IncRowCount(ctx, 1, spanutil.Invocations, spanutil.Inserted, r.Invocation.GetRealm())
		}
	}

	return getCreatedInvocationsAndUpdateTokens(ctx, idSet, reqs)
}

// createInvocationsInternal creates each invocation requested. It enqueues
// task queue tasks for finalization and/or export notifications as appropriate.
// Must be called within a Spanner Read/Write transaction.
func (s *recorderServer) createInvocationsInternal(ctx context.Context, now time.Time, reqs []*pb.CreateInvocationRequest, requestID, createdBy string) {
	ms := make([]*spanner.Mutation, 0, len(reqs))
	// Compute mutations
	for _, req := range reqs {
		newInvState := req.Invocation.GetState()
		if newInvState == pb.Invocation_STATE_UNSPECIFIED {
			newInvState = pb.Invocation_ACTIVE
		}
		if newInvState != pb.Invocation_ACTIVE && newInvState != pb.Invocation_FINALIZING {
			// validateCreateInvocationRequest should have rejected any other states.
			panic("do not create invocations in states other than active or finalizing")
		}

		// Prepare the invocation we will save to spanner.
		inv := &pb.Invocation{
			Name:               invocations.ID(req.InvocationId).Name(),
			State:              newInvState,
			Deadline:           req.Invocation.GetDeadline(),
			Tags:               req.Invocation.GetTags(),
			IsExportRoot:       req.Invocation.GetIsExportRoot(),
			BigqueryExports:    req.Invocation.GetBigqueryExports(),
			CreatedBy:          createdBy,
			ProducerResource:   req.Invocation.GetProducerResource(),
			Realm:              req.Invocation.GetRealm(),
			Properties:         req.Invocation.GetProperties(),
			SourceSpec:         req.Invocation.GetSourceSpec(),
			IsSourceSpecFinal:  req.Invocation.GetIsSourceSpecFinal(),
			BaselineId:         req.Invocation.GetBaselineId(),
			TestInstruction:    req.Invocation.GetTestInstruction(),
			StepInstructions:   req.Invocation.GetStepInstructions(),
			ExtendedProperties: req.Invocation.GetExtendedProperties(),
		}

		// Ensure the invocation has a deadline.
		if inv.Deadline == nil {
			inv.Deadline = pbutil.MustTimestampProto(now.Add(defaultInvocationDeadlineDuration))
		}

		pbutil.NormalizeInvocation(inv)
		// Create a mutation to create the invocation.
		ms = append(ms, spanutil.InsertMap("Invocations", s.rowOfInvocation(ctx, inv, requestID)))

		// Add any inclusions.
		var includedInvocationIDs []string
		for _, incName := range req.Invocation.IncludedInvocations {
			invID := invocations.MustParseName(incName)
			ms = append(ms, spanutil.InsertMap("IncludedInvocations", map[string]any{
				"InvocationId":         invocations.ID(req.InvocationId),
				"IncludedInvocationId": invID,
			}))
			includedInvocationIDs = append(includedInvocationIDs, string(invID))
		}

		if req.Invocation.GetIsExportRoot() {
			// An export root has itself as an export root. Exportnotifier service
			// will propagate the export root to included invocations in a separate
			// task.
			root := exportroots.ExportRoot{
				Invocation:            invocations.ID(req.InvocationId),
				RootInvocation:        invocations.ID(req.InvocationId),
				IsInheritedSourcesSet: true, // Roots inherit nil sources.
				InheritedSources:      nil,
				IsNotified:            false,
			}
			ms = append(ms, exportroots.Create(root))

			if len(includedInvocationIDs) > 0 {
				// Enqueue task to propagate export root to any included invocations
				// and send any notifications.
				exportnotifier.EnqueueTask(ctx, &taskspb.RunExportNotifications{
					InvocationId:          req.InvocationId,
					RootInvocationIds:     []string{req.InvocationId},
					IncludedInvocationIds: includedInvocationIDs,
				})
			}
		}

		if req.Invocation.State == pb.Invocation_FINALIZING {
			// Enqueue finalization task and run export notifications task.
			tasks.StartInvocationFinalization(ctx, invocations.ID(req.InvocationId), false)
		}
	}
	span.BufferWrite(ctx, ms...)
}

// getCreatedInvocationsAndUpdateTokens reads the full details of the
// invocations just created in a separate read-only transaction, and
// generates an update token for each.
func getCreatedInvocationsAndUpdateTokens(ctx context.Context, idSet invocations.IDSet, reqs []*pb.CreateInvocationRequest) ([]*pb.Invocation, []string, error) {
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	invMap, err := invocations.ReadBatch(ctx, idSet)
	if err != nil {
		return nil, nil, err
	}

	// Arrange them in same order as the incoming requests.
	// Ordering is important to match the tokens.
	invs := make([]*pb.Invocation, len(reqs))
	for i, req := range reqs {
		invs[i] = invMap[invocations.ID(req.InvocationId)]
	}

	tokens, err := generateTokens(ctx, invs)
	if err != nil {
		return nil, nil, err
	}
	return invs, tokens, nil
}

// deduplicateCreateInvocations checks if the invocations have already been
// created with the given requestID and current requester.
// Returns a true if they have.
func deduplicateCreateInvocations(ctx context.Context, idSet invocations.IDSet, requestID, createdBy string) (bool, error) {
	invCount := 0
	columns := []string{"InvocationId", "CreateRequestId", "CreatedBy"}
	err := span.Read(ctx, "Invocations", idSet.Keys(), columns).Do(func(r *spanner.Row) error {
		var invID invocations.ID
		var rowRequestID spanner.NullString
		var rowCreatedBy spanner.NullString
		switch err := spanutil.FromSpanner(r, &invID, &rowRequestID, &rowCreatedBy); {
		case err != nil:
			return err
		case !rowRequestID.Valid || rowRequestID.StringVal != requestID:
			return invocationAlreadyExists(invID)
		case rowCreatedBy.StringVal != createdBy:
			return invocationAlreadyExists(invID)
		default:
			invCount++
			return nil
		}
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
		updateToken, err := generateInvocationToken(ctx, invocations.MustParseName(inv.Name))
		if err != nil {
			return nil, err
		}
		ret[i] = updateToken
	}
	return ret, nil
}
