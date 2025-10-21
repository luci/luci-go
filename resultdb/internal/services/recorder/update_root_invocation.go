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
	"maps"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// UpdateRootInvocation implements pb.RecorderServer.
func (s *recorderServer) UpdateRootInvocation(ctx context.Context, in *pb.UpdateRootInvocationRequest) (*pb.RootInvocation, error) {
	if err := verifyUpdateRootInvocationPermissions(ctx, in); err != nil {
		return nil, err
	}

	if err := validateUpdateRootInvocationRequest(ctx, in); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	rootInvID := rootinvocations.MustParseName(in.RootInvocation.Name)

	updatedBy := string(auth.CurrentIdentity(ctx))
	var updatedRootInvRow *rootinvocations.RootInvocationRow
	shouldFinalizeRootInvocation := false
	updated := false
	ct, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		// Reset variables in case the transaction gets retried.
		updatedRootInvRow = nil
		shouldFinalizeRootInvocation = false
		updated = false

		// Read the current root invocation.
		originalRootInvRow, err := rootinvocations.Read(ctx, rootInvID)
		if err != nil {
			return err
		}
		exist, err := rootinvocations.CheckRootInvocationUpdateRequestExist(ctx, rootInvID, updatedBy, in.RequestId)
		if err != nil {
			return err
		}
		if exist {
			// Request has already been processed before. This call should be deduplicated,
			// so do not write to the database.
			// Return the root invocation as they were read at the start of this transaction.
			// This is a best-effort attempt to return the same response as the original
			// request. Note that if the root invocation were modified between the original
			// request and this one, this RPC will return the current state of the
			// work units, which will be different from the original response.
			updatedRootInvRow = originalRootInvRow
			return nil
		}

		// Validate assumptions.
		// - Etag match (if not empty).
		// - Root invocation is active.
		if in.RootInvocation.Etag != "" {
			match, err := rootinvocations.IsEtagMatch(originalRootInvRow, in.RootInvocation.Etag)
			if err != nil {
				// Impossible, etag has already been validated.
				return err
			}
			if !match {
				// Attach a codes.Aborted appstatus to a vanilla error to avoid
				// ReadWriteTransaction interpreting this case for a scenario
				// in which it should retry the transaction.
				err := errors.New("etag mismatch")
				return appstatus.Attach(err, status.New(codes.Aborted, "the root invocation was modified since it was last read; the update was not applied"))
			}
		}
		if originalRootInvRow.FinalizationState != pb.RootInvocation_ACTIVE {
			return appstatus.Errorf(codes.FailedPrecondition, "root invocation %q is not active", rootInvID.Name())
		}
		var updateMutations []*spanner.Mutation
		updateMutations, updatedRootInvRow, err = updateRootInvocationInternal(in, originalRootInvRow)
		if err != nil {
			return err // BadRequest error, internal error.
		}
		updated = len(updateMutations) > 0
		if updated {
			// Trigger finalization task, when the root invocation is updated to finalizing.
			if updatedRootInvRow.FinalizationState == pb.RootInvocation_FINALIZING {
				// No finalizer task schedule, the work unit finalizer will handle the finalization of root invocation.
				// The ability to finalize a root invocation in UpdateRootInvocation RPC will be deprecating soon.
				shouldFinalizeRootInvocation = true
			}
		}
		// Insert into RootInvocationUpdateRequests table.
		updateMutations = append(updateMutations, rootinvocations.CreateRootInvocationUpdateRequest(rootInvID, updatedBy, in.RequestId))
		span.BufferWrite(ctx, updateMutations...)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Populate output-only fields that are based on the commit timestamp.
	if updated {
		updatedRootInvRow.LastUpdated = ct
	}
	if shouldFinalizeRootInvocation {
		updatedRootInvRow.FinalizeStartTime = spanner.NullTime{Valid: true, Time: ct}
	}

	return updatedRootInvRow.ToProto(), nil
}

func updateRootInvocationInternal(in *pb.UpdateRootInvocationRequest, originalRootInvRow *rootinvocations.RootInvocationRow) (updateMutations []*spanner.Mutation, updatedRow *rootinvocations.RootInvocationRow, err error) {
	updatedRootInvRow := originalRootInvRow.Clone()
	updateMask, err := mask.FromFieldMask(in.UpdateMask, in.RootInvocation, mask.AdvancedSemantics(), mask.ForUpdate())
	if err != nil {
		// Should not happen, as it's validated in the initial request validation.
		return nil, nil, errors.Fmt("update_mask: %w", err)
	}
	rootInvID := rootinvocations.MustParseName(in.RootInvocation.Name)
	// Update map for the RootInvocations table.
	rootInvocationValues := map[string]any{
		"RootInvocationId": rootInvID,
	}
	// Update map for the legacy Invocations table.
	legacyInvocationValues := map[string]any{
		"InvocationId": rootInvID.LegacyInvocationID(),
	}
	// Update map for RootInvocationShards table.
	shardRootInvocationValues := map[string]any{}

	for path := range updateMask.Children() {
		switch path {
		// The cases in this switch statement must be synchronized with a
		// similar switch statement in validateUpdateRootInvocationRequest.

		// TODO(meiring): Remove "state" here once clients have updated to the new field name.
		case "finalization_state", "state":
			// In the case of ACTIVE it should be a No-op.
			if in.RootInvocation.FinalizationState == pb.RootInvocation_FINALIZING {
				rootInvocationValues["FinalizationState"] = pb.RootInvocation_FINALIZING
				rootInvocationValues["FinalizeStartTime"] = spanner.CommitTimestamp
				legacyInvocationValues["State"] = pb.Invocation_FINALIZING
				legacyInvocationValues["FinalizeStartTime"] = spanner.CommitTimestamp
				updatedRootInvRow.FinalizationState = pb.RootInvocation_FINALIZING
			}

		case "deadline":
			if !originalRootInvRow.Deadline.Equal(in.RootInvocation.Deadline.AsTime()) {
				deadline := in.RootInvocation.Deadline
				rootInvocationValues["Deadline"] = deadline
				legacyInvocationValues["Deadline"] = deadline
				updatedRootInvRow.Deadline = deadline.AsTime()
			}
		case "sources":
			// Are we setting the field to a value other than its current value?
			if !proto.Equal(originalRootInvRow.Sources, in.RootInvocation.Sources) {
				// We can't set the field to a value other than its current value, if IsSourcesFinal already set to true.
				if originalRootInvRow.IsSourcesFinal {
					return nil, nil, appstatus.BadRequest(errors.New("root_invocation: sources: cannot modify already finalized sources"))
				}
				compressedSources := spanutil.Compressed(pbutil.MustMarshal(in.RootInvocation.Sources))
				rootInvocationValues["Sources"] = compressedSources
				legacyInvocationValues["Sources"] = compressedSources
				shardRootInvocationValues["Sources"] = compressedSources
				updatedRootInvRow.Sources = in.RootInvocation.Sources
			}

		case "sources_final":
			if originalRootInvRow.IsSourcesFinal != in.RootInvocation.SourcesFinal {
				if !in.RootInvocation.SourcesFinal {
					return nil, nil, appstatus.BadRequest(errors.New("root_invocation: sources_final: cannot un-finalize already finalized sources"))
				}
				rootInvocationValues["IsSourcesFinal"] = true
				legacyInvocationValues["IsSourceSpecFinal"] = spanner.NullBool{Valid: true, Bool: true}
				shardRootInvocationValues["IsSourcesFinal"] = true
				updatedRootInvRow.IsSourcesFinal = true
			}

		case "tags":
			if !pbutil.StringPairsEqual(originalRootInvRow.Tags, in.RootInvocation.Tags) {
				tags := in.RootInvocation.Tags
				rootInvocationValues["Tags"] = tags
				legacyInvocationValues["Tags"] = tags
				updatedRootInvRow.Tags = tags
			}
		case "properties":
			if !proto.Equal(originalRootInvRow.Properties, in.RootInvocation.Properties) {
				compressedProps := spanutil.Compressed(pbutil.MustMarshal(in.RootInvocation.Properties))
				rootInvocationValues["Properties"] = compressedProps
				legacyInvocationValues["Properties"] = compressedProps
				updatedRootInvRow.Properties = in.RootInvocation.Properties
			}
		case "baseline_id":
			if originalRootInvRow.BaselineID != in.RootInvocation.BaselineId {
				baselineID := in.RootInvocation.BaselineId
				rootInvocationValues["BaselineId"] = baselineID
				legacyInvocationValues["BaselineId"] = baselineID
				updatedRootInvRow.BaselineID = baselineID
			}
		default:
			// This should not be reached due to the validation step.
			panic("impossible")
		}
	}
	// The `rootInvocationValues` map is initialized with the primary key.
	// There is no update if no other columns are added.
	updated := len(rootInvocationValues) > 1
	if updated {
		rootInvocationValues["LastUpdated"] = spanner.CommitTimestamp
		updateMutations = append(updateMutations, spanutil.UpdateMap("RootInvocations", rootInvocationValues))
		updateMutations = append(updateMutations, spanutil.UpdateMap("Invocations", legacyInvocationValues))
		// Check if any update is needed to the RootInvocationShards table.
		if len(shardRootInvocationValues) > 0 {
			// Update all records for this root invocation in RootInvocationShards table.
			for shardID := range rootInvID.AllShardIDs() {
				shardUpdate := make(map[string]any, len(shardRootInvocationValues)+1)
				maps.Copy(shardUpdate, shardRootInvocationValues)
				shardUpdate["RootInvocationShardId"] = shardID
				updateMutations = append(updateMutations, spanutil.UpdateMap("RootInvocationShards", shardUpdate))
			}
		}
	}
	return updateMutations, updatedRootInvRow, nil
}

// validateUpdateRootInvocationRequest validates an UpdateRootInvocationRequest.
func validateUpdateRootInvocationRequest(ctx context.Context, req *pb.UpdateRootInvocationRequest) error {
	// The root invocation name is already validated in verifyUpdateRootInvocationPermissions.

	if req.RootInvocation.Etag != "" {
		if _, err := rootinvocations.ParseEtag(req.RootInvocation.Etag); err != nil {
			return errors.Fmt("root_invocation: etag: %w", err)
		}
	}
	if req.RequestId == "" {
		return errors.Fmt("request_id: unspecified (please provide a per-request UUID to ensure idempotence)")
	}
	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Fmt("request_id: %w", err)
	}

	if len(req.UpdateMask.GetPaths()) == 0 {
		return errors.New("update_mask: paths is empty")
	}

	updateMask, err := mask.FromFieldMask(req.UpdateMask, req.RootInvocation, mask.AdvancedSemantics(), mask.ForUpdate())
	if err != nil {
		return errors.Fmt("update_mask: %w", err)
	}

	for path, submask := range updateMask.Children() {
		// For RootInvocation, none of the updatable fields should contain submask.
		if len(submask.Children()) > 0 {
			return errors.Fmt("update_mask: %q should not have any submask", path)
		}
		switch path {
		// The cases in this switch statement must be synchronized with a
		// similar switch statement in the UpdateRootInvocation implementation.

		// TODO(meiring): Remove "state" here once clients have updated to the new field name.
		case "finalization_state", "state":
			// If "finalization_state" is set, we only allow "FINALIZING" or "ACTIVE" state.
			// Setting to "FINALIZING" will trigger the finalization process.
			// Setting to "ACTIVE" is a no-op.
			if req.RootInvocation.FinalizationState != pb.RootInvocation_FINALIZING && req.RootInvocation.FinalizationState != pb.RootInvocation_ACTIVE {
				return errors.New("root_invocation: finalization_state: must be FINALIZING or ACTIVE")
			}

		case "deadline":
			// Use clock.Now(ctx).UTC() for validation; the actual commit time will be used for storage.
			assumedCreateTime := clock.Now(ctx).UTC()
			if err := validateDeadline(req.RootInvocation.Deadline, assumedCreateTime); err != nil {
				return errors.Fmt("root_invocation: deadline: %w", err)
			}

		case "sources":
			if err := pbutil.ValidateSources(req.RootInvocation.Sources); err != nil {
				return errors.Fmt("root_invocation: sources: %w", err)
			}

		case "sources_final":
			// Either true or false is OK for this first pass validation.
			// However, later we must validate that if the field is true,
			// it is not being set to false.

		case "tags":
			if err := pbutil.ValidateRootInvocationTags(req.RootInvocation.Tags); err != nil {
				return errors.Fmt("root_invocation: tags: %w", err)
			}

		case "properties":
			if err := pbutil.ValidateRootInvocationProperties(req.RootInvocation.Properties); err != nil {
				return errors.Fmt("root_invocation: properties: %w", err)
			}

		case "baseline_id":
			if req.RootInvocation.BaselineId != "" {
				if err := pbutil.ValidateBaselineID(req.RootInvocation.BaselineId); err != nil {
					return errors.Fmt("root_invocation: baseline_id: %w", err)
				}
			}

		default:
			return errors.Fmt("update_mask: unsupported path %q", path)
		}
	}
	return nil
}

// verifyUpdateRootInvocationPermissions checks if the caller has permission to
// update the given root invocation. It uses an update token for authentication.
func verifyUpdateRootInvocationPermissions(ctx context.Context, req *pb.UpdateRootInvocationRequest) error {
	if req.RootInvocation == nil {
		return appstatus.BadRequest(errors.New("root_invocation: unspecified"))
	}
	rootInvocationID, err := rootinvocations.ParseName(req.RootInvocation.Name)
	if err != nil {
		return appstatus.BadRequest(errors.Fmt("root_invocation: name: %w", err))
	}

	token, err := extractUpdateToken(ctx)
	if err != nil {
		return err // Appstatus error.
	}

	// RootInvocation and root work unit share the same update token.
	// Use the update token generated from root work unit id.
	rootWorkUnitID := workunits.ID{
		RootInvocationID: rootInvocationID,
		WorkUnitID:       workunits.RootWorkUnitID,
	}
	if err := validateWorkUnitUpdateToken(ctx, token, rootWorkUnitID); err != nil {
		return err // PermissionDenied appstatus error.
	}

	// Read the realms of the root invocation. Even though this read is occurring in
	// a different transaction, it is not susceptible to TOC-TOU vulnerabilities as
	// the realm is immutable.
	realm, err := rootinvocations.ReadRealm(span.Single(ctx), rootInvocationID)

	updateMask, err := mask.FromFieldMask(req.UpdateMask, req.RootInvocation, mask.ForUpdate())
	if err != nil {
		return appstatus.BadRequest(errors.Fmt("update_mask: %w", err))
	}

	for path := range updateMask.Children() {
		switch path {
		case "baseline_id":
			if err := validateUpdateBaselinePermissions(ctx, realm); err != nil {
				return err // PermissionDenied appstatus error.
			}
		}
	}
	return nil
}
