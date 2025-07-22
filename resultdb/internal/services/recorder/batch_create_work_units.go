// Copyright 2025 The LUCI Authors.
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
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/masking"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// BatchCreateWorkUnits implements pb.RecorderServer.
func (s *recorderServer) BatchCreateWorkUnits(ctx context.Context, in *pb.BatchCreateWorkUnitsRequest) (*pb.BatchCreateWorkUnitsResponse, error) {
	if err := validateBatchCreateWorkUnitsPermissions(ctx, in); err != nil {
		return nil, err
	}
	if err := validateBatchCreateWorkUnitsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	if err := createWorkUnitsIdempotent(ctx, in, s.ExpectedResultsExpiration); err != nil {
		return nil, err
	}

	// TODO: The response is read in a separate transaction
	// from the creation. This means a duplicate request may not receive an
	// identical response if the invocation was updated in the meantime.
	// While AIP-155 (https://google.aip.dev/155#stale-success-responses)
	// permits returning the most current data, we could instead construct the
	// response from the request data for better consistency.
	ids := make([]workunits.ID, 0, len(in.Requests))
	for _, r := range in.Requests {
		ids = append(ids, extractWorkUnitIDFromRequest(r))
	}
	wuRows, err := workunits.ReadBatch(span.Single(ctx), ids, workunits.AllFields)
	if err != nil {
		return nil, err
	}
	workUnits := make([]*pb.WorkUnit, len(in.Requests))
	updateTokens := make([]string, len(in.Requests))
	for i, id := range ids {
		token, err := generateWorkUnitToken(ctx, id)
		if err != nil {
			return nil, err
		}
		updateTokens[i] = token
		inputs := masking.WorkUnitFields{
			Row: wuRows[i],
			// TODO:populate child work unit.
			// There should be no child invocation at this point.
		}
		workUnits[i] = masking.WorkUnit(inputs, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_FULL)
	}
	return &pb.BatchCreateWorkUnitsResponse{
		WorkUnits:    workUnits,
		UpdateTokens: updateTokens,
	}, nil
}

// createWorkUnitsIdempotent atomically creates the work units in the request.
// The operation is idempotent: if the work units already exist with the same
// request ID, the creation is skipped.
func createWorkUnitsIdempotent(
	ctx context.Context,
	in *pb.BatchCreateWorkUnitsRequest,
	uninterestingTestVerdictsExpirationTime time.Duration,
) error {
	now := clock.Now(ctx)
	createdBy := string(auth.CurrentIdentity(ctx))

	ids := make([]workunits.ID, 0, len(in.Requests))
	parentIDs := make([]workunits.ID, 0, len(in.Requests))
	for _, r := range in.Requests {
		ids = append(ids, extractWorkUnitIDFromRequest(r))
		parentIDs = append(parentIDs, workunits.MustParseName(r.Parent))
	}
	_, err := mutateWorkUnitsForCreate(ctx, parentIDs, ids, func(ctx context.Context) error {
		dedup, err := deduplicateCreateWorkUnits(ctx, ids, in.RequestId, createdBy)
		if err != nil {
			return err
		}
		if dedup {
			// This call should be deduplicated, do not write to database.
			return nil
		}
		for i, r := range in.Requests {
			wu := r.WorkUnit
			state := wu.State
			if state == pb.WorkUnit_STATE_UNSPECIFIED {
				state = pb.WorkUnit_ACTIVE
			}
			deadline := wu.Deadline.AsTime()
			if wu.Deadline == nil {
				deadline = now.Add(defaultDeadlineDuration)
			}

			wuRow := &workunits.WorkUnitRow{
				ID:                 ids[i],
				ParentWorkUnitID:   spanner.NullString{Valid: true, StringVal: parentIDs[i].WorkUnitID},
				State:              state,
				Realm:              wu.Realm,
				CreatedBy:          createdBy,
				Deadline:           deadline,
				CreateRequestID:    in.RequestId,
				ProducerResource:   wu.ProducerResource,
				Tags:               wu.Tags,
				Properties:         wu.Properties,
				Instructions:       wu.Instructions,
				ExtendedProperties: wu.ExtendedProperties,
			}
			legacyCreateOpts := workunits.LegacyCreateOptions{
				ExpectedTestResultsExpirationTime: now.Add(uninterestingTestVerdictsExpirationTime),
			}
			span.BufferWrite(ctx, workunits.Create(wuRow, legacyCreateOpts)...)
		}
		// TODO: add inclusions
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func validateBatchCreateWorkUnitsPermissions(ctx context.Context, req *pb.BatchCreateWorkUnitsRequest) error {
	// Only perform minimal validation necessary to verify permissions. Full validation
	// will be performed in validateBatchCreateWorkUnitsRequest.

	if err := pbutil.ValidateBatchRequestCount(len(req.Requests)); err != nil {
		return appstatus.BadRequest(errors.Fmt("requests: %w", err))
	}
	for i, r := range req.Requests {
		if err := verifyCreateWorkUnitPermissions(ctx, r); err != nil {
			// Wrap the app-status error if any by inserting the wrapping text inside the error.
			st, ok := appstatus.Get(err)
			if ok {
				return appstatus.Error(st.Code(), fmt.Sprintf("requests[%d]: %s", i, st.Message()))
			}
			return errors.Fmt("requests[%d]: %w", i, err)
		}
	}
	return nil
}

// mutateWorkUnitsForCreate provides a transactional wrapper for creating work units.
//
// It ensures that modifications happen only if the parent work unit is ACTIVE and
// the caller provides a valid update token. For parents created in the same batch
// request, the state check is skipped as they don't exist in the database yet.
//
// Both the parentIDs and newIDs slices must be corresponding the BatchCreateWorkUnits.requests
// (i.e. parentIDs[i] and  newIDs[i] are the parent ID and work unit ID in BatchCreateWorkUnits.requests[i])
//
// On success, it returns the Spanner commit timestamp.
func mutateWorkUnitsForCreate(ctx context.Context, parentIDs []workunits.ID, newIDs []workunits.ID, f func(context.Context) error) (time.Time, error) {
	// Per AIP-211, authorization should precede validation. We intentionally deviate
	// from that pattern by performing update-token authorization inside `mutateWorkUnitsForCreate`,
	// after initial request validation. This make sure data modification is always
	// performed with authorization check.
	//
	// Because the update-token is not yet validated, logic
	// before the `mutateWorkUnitsForCreate` call MUST NOT access or return any protected
	// resource data. All preceding checks should operate only on the incoming
	// request parameters.
	token, err := extractUpdateToken(ctx)
	if err != nil {
		return time.Time{}, err
	}

	if err := validateSameUpdateTokenState(parentIDs); err != nil {
		return time.Time{}, appstatus.BadRequest(err)
	}

	// We have already check all parentIDs have the same update token, so we just need to check against one of them.
	if err := validateWorkUnitToken(ctx, token, parentIDs[0]); err != nil {
		return time.Time{}, appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}

	newIDSet := workunits.NewIDSet(newIDs...)
	parentsToCheckSet := workunits.NewIDSet(parentIDs...)
	// Only check parents that are not being created in this batch.
	parentsToCheckSet.RemoveAll(newIDSet)
	parentsToCheck := parentsToCheckSet.ToSlice()

	commitTimestamp, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		states, err := workunits.ReadStates(ctx, parentsToCheck)
		if err != nil {
			return err
		}
		for i, st := range states {
			if st != pb.WorkUnit_ACTIVE {
				return appstatus.Errorf(codes.FailedPrecondition, "parent %q is not active", parentsToCheck[i].Name())
			}
		}
		return f(ctx)
	})
	if err != nil {
		return time.Time{}, err
	}
	return commitTimestamp, nil
}

func deduplicateCreateWorkUnits(ctx context.Context, ids []workunits.ID, requestID, createdBy string) (shouldDedup bool, err error) {
	results, err := workunits.ReadRequestIDsAndCreatedBys(ctx, ids)
	if err != nil {
		return false, err
	}
	var exampleExistWorkUnit *workunits.ID
	existCount := 0
	for i := range ids {
		if results[i] == nil {
			// Work unit ID doesn't exist yet.
			continue
		}
		if exampleExistWorkUnit == nil {
			exampleExistWorkUnit = &ids[i]
		}
		existCount += 1
		if results[i].RequestID != requestID || results[i].CreatedBy != createdBy {
			// Work unit with the same id exist, and is not created with the same requestID and creator.
			return false, appstatus.Errorf(codes.AlreadyExists, "%q already exists with different requestID or creator", ids[i].Name())
		}
	}
	if existCount == 0 {
		// Do not deduplicate, none of the id exists.
		return false, nil
	}
	if existCount != len(ids) {
		// some ids already exist, but some doesn't exist.
		// Could happen if someone sent two different but overlapping batch create
		// requests, but reused the request_id.
		return false, appstatus.Errorf(codes.AlreadyExists, "some work units already exist (eg. %q)", exampleExistWorkUnit.Name())
	}
	// All id exist, deduplicate this call.
	return true, nil
}

func validateBatchCreateWorkUnitsRequest(req *pb.BatchCreateWorkUnitsRequest) error {
	if req.RequestId == "" {
		// Request ID is required to ensure requests are treated idempotently
		// in case of inevitable retries.
		return errors.Fmt("request_id: unspecified (please provide a per-request UUID to ensure idempotence)")
	}
	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Fmt("request_id: %w", err)
	}

	if err := pbutil.ValidateBatchRequestCount(len(req.Requests)); err != nil {
		return errors.Fmt("requests: %w", err)
	}
	seenIDs := workunits.NewIDSet()
	var rootInvocationID rootinvocations.ID
	for i, r := range req.Requests {
		// Validate the sub-request.
		// The request ID is not specified on the sub-request as it is already
		// specified on the parent.
		if err := validateCreateWorkUnitRequest(r, false /*requireRequestID*/); err != nil {
			return errors.Fmt("requests[%d]: %w", i, err)
		}
		if r.RequestId != "" && r.RequestId != req.RequestId {
			return errors.Fmt("requests[%d]: request_id: inconsistent with top-level request_id", i)
		}
		// Validate no duplicated work unit.
		newID := extractWorkUnitIDFromRequest(r)
		if ok := seenIDs.Has(newID); ok {
			return errors.Fmt("requests[%d]: work_unit_id: duplicated work unit id %q", i, r.WorkUnitId)
		}
		seenIDs.Add(newID)
		// Validate all requests have the same root invocation ID.
		if rootInvocationID != "" && rootInvocationID != newID.RootInvocationID {
			return errors.Fmt("requests[%d]: parent: all requests must be for creations in the same root invocation", i)
		}
		if rootInvocationID == "" {
			rootInvocationID = newID.RootInvocationID
		}
	}
	if err := validateRequestOrdering(req); err != nil {
		return err
	}
	return nil
}

// validate request entries only refer to work units created by earlier
// request entries. This make sure no loop in the work unit structure.
func validateRequestOrdering(req *pb.BatchCreateWorkUnitsRequest) error {
	idToIndex := make(map[workunits.ID]int, len(req.Requests))
	for i, r := range req.Requests {
		newID := extractWorkUnitIDFromRequest(r)
		idToIndex[newID] = i
	}
	for i, r := range req.Requests {
		parentID := workunits.MustParseName(r.Parent)
		idx, ok := idToIndex[parentID]
		if ok && idx >= i {
			return errors.Fmt("requests[%d]: parent: cannot refer to work unit created in the later request requests[%d], please order requests to match expected creation order", i, idx)
		}
	}
	return nil
}

func extractWorkUnitIDFromRequest(r *pb.CreateWorkUnitRequest) workunits.ID {
	parentID := workunits.MustParseName(r.Parent)
	return workunits.ID{
		RootInvocationID: parentID.RootInvocationID,
		WorkUnitID:       r.WorkUnitId,
	}
}

// validate only a single update token is required.
func validateSameUpdateTokenState(parents []workunits.ID) error {
	s := workUnitTokenState(parents[0])
	for i, p := range parents {
		if s != workUnitTokenState(p) {
			return errors.Fmt("requests[%d]: parent %q requires a different update token to requests[0].parent %q, but this RPC only accepts one update token", i, p.Name(), parents[0].Name())
		}
	}
	return nil
}
