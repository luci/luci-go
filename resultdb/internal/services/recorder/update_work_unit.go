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

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/masking"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// UpdateWorkUnit implements pb.RecorderServer.
func (s *recorderServer) UpdateWorkUnit(ctx context.Context, in *pb.UpdateWorkUnitRequest) (*pb.WorkUnit, error) {
	if err := verifyUpdateWorkUnitPermissions(ctx, in); err != nil {
		return nil, err
	}
	const requireRequestID = true
	if err := validateUpdateWorkUnitRequest(ctx, in, requireRequestID); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	// Piggy back on updateWorkUnits.
	updatedRows, err := updateWorkUnits(ctx, []*pb.UpdateWorkUnitRequest{in}, in.RequestId)
	if err != nil {
		// Remove any references to "requests[0]: ", this is a single create RPC not a batch RPC.
		return nil, removeRequestNumberFromAppStatusError(err)
	}

	cfg, err := config.Service(ctx)
	if err != nil {
		return nil, err
	}
	res := masking.WorkUnit(updatedRows[0], permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_FULL, cfg)
	return res, nil
}

// updateWorkUnitInternal returns the mutations to update a work unit, the updated work unit row.
func updateWorkUnitInternal(in *pb.UpdateWorkUnitRequest, curWorkUnitRow *workunits.WorkUnitRow, ordinal int) (updateMutations []*spanner.Mutation, updatedRow *workunits.WorkUnitRow, err error) {
	updatedRow = curWorkUnitRow.Clone()
	updateMask, err := mask.FromFieldMask(in.UpdateMask, in.WorkUnit, mask.ForUpdate())
	if err != nil {
		// Should not happen, the update mask has already been validated.
		return nil, nil, errors.Fmt("requests[%d]: update_mask: %w", ordinal, err)
	}
	wuID := workunits.MustParseName(in.WorkUnit.Name)
	mb := workunits.NewMutationBuilder(wuID)

	for path, submask := range updateMask.Children() {
		switch path {
		// The cases in this switch statement must be synchronized with a
		// similar switch statement in validateUpdateWorkUnitRequest.

		case "state":
			if curWorkUnitRow.State != in.WorkUnit.State {
				// Given we have not failed validation earlier, we know the
				// work unit finalization state is ACTIVE (and therefore the work unit is
				// not yet in a terminal state). We therefore only need to deal with
				// validating transitions starting in a non-terminal state.
				if curWorkUnitRow.State == pb.WorkUnit_RUNNING && in.WorkUnit.State == pb.WorkUnit_PENDING {
					// Cannot transition from RUNNING -> PENDING.
					return nil, nil, appstatus.BadRequest(errors.Fmt("requests[%d]: work_unit: state: cannot transition from %s to %s", ordinal, curWorkUnitRow.State, in.WorkUnit.State))
				}

				mb.UpdateState(in.WorkUnit.State)
				updatedRow.State = in.WorkUnit.State
				if pbutil.IsFinalWorkUnitState(in.WorkUnit.State) {
					updatedRow.FinalizationState = pb.WorkUnit_FINALIZING
					// Use spanner.CommitTimestamp as a placeholder for now, the caller will deal with
					// updating this to the actual start time.
					updatedRow.FinalizeStartTime = spanner.NullTime{Valid: true, Time: spanner.CommitTimestamp}
				}
			}
		case "summary_markdown":
			if in.WorkUnit.SummaryMarkdown != curWorkUnitRow.SummaryMarkdown {
				mb.UpdateSummaryMarkdown(in.WorkUnit.SummaryMarkdown)
				updatedRow.SummaryMarkdown = in.WorkUnit.SummaryMarkdown
			}
		case "deadline":
			if !curWorkUnitRow.Deadline.Equal(in.WorkUnit.Deadline.AsTime()) {
				deadline := in.WorkUnit.Deadline.AsTime()
				mb.UpdateDeadline(deadline)
				updatedRow.Deadline = deadline
			}
		case "properties":
			if !proto.Equal(curWorkUnitRow.Properties, in.WorkUnit.Properties) {
				mb.UpdateProperties(in.WorkUnit.Properties)
				updatedRow.Properties = in.WorkUnit.Properties
			}
		case "tags":
			if !pbutil.StringPairsEqual(curWorkUnitRow.Tags, in.WorkUnit.Tags) {
				mb.UpdateTags(in.WorkUnit.Tags)
				updatedRow.Tags = in.WorkUnit.Tags
			}
		case "extended_properties":
			extendedProperties := in.WorkUnit.ExtendedProperties
			curExtendedProperties := curWorkUnitRow.ExtendedProperties
			updatedExtendedProperties := updateExtendedProperties(curExtendedProperties, extendedProperties, submask)
			if !pbutil.ExtendedPropertiesEqual(updatedExtendedProperties, curExtendedProperties) {
				if err := pbutil.ValidateInvocationExtendedProperties(updatedExtendedProperties); err != nil {
					// One more validation to ensure the size is within the limit.
					return nil, nil, appstatus.BadRequest(errors.Fmt("requests[%d]: work_unit: extended_properties: %w", ordinal, err))
				}
				mb.UpdateExtendedProperties(updatedExtendedProperties)
				updatedRow.ExtendedProperties = updatedExtendedProperties
			}
		case "instructions":
			ins := instructionutil.RemoveInstructionsName(in.WorkUnit.Instructions)
			curIns := instructionutil.RemoveInstructionsName(curWorkUnitRow.Instructions)
			if !proto.Equal(curIns, ins) {
				mb.UpdateInstructions(in.WorkUnit.Instructions)
				updatedRow.Instructions = instructionutil.InstructionsWithNames(in.WorkUnit.Instructions, wuID.Name())
			}
		default:
			panic("impossible")
		}
	}
	ms := mb.Build()
	updated := len(ms) > 0
	if updated {
		// Use spanner.CommitTimestamp as a placeholder for now, the caller
		// will update this to the actual commit timestamp.
		updatedRow.LastUpdated = spanner.CommitTimestamp
	}
	return ms, updatedRow, nil
}

func validateUpdateWorkUnitRequest(ctx context.Context, req *pb.UpdateWorkUnitRequest, requireRequestID bool) error {
	if req.WorkUnit == nil {
		return errors.Fmt("work_unit: unspecified")
	}
	if err := pbutil.ValidateWorkUnitName(req.WorkUnit.Name); err != nil {
		return errors.Fmt("work_unit: name: %w", err)
	}
	if req.WorkUnit.Etag != "" {
		if _, err := masking.ParseWorkUnitETag(req.WorkUnit.Etag); err != nil {
			return errors.Fmt("work_unit: etag: %w", err)
		}
	}
	if len(req.UpdateMask.GetPaths()) == 0 {
		return errors.New("update_mask: paths is empty")
	}
	if requireRequestID && req.RequestId == "" {
		return errors.Fmt("request_id: unspecified (please provide a per-request UUID to ensure idempotence)")
	}
	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Fmt("request_id: %w", err)
	}
	updateMask, err := mask.FromFieldMask(req.UpdateMask, req.WorkUnit, mask.ForUpdate())
	if err != nil {
		return errors.Fmt("update_mask: %w", err)
	}

	for path, submask := range updateMask.Children() {
		if err := validateUpdateWorkUnitRequestSubmask(path, submask); err != nil {
			return err
		}
		switch path {
		// The cases in this switch statement must be synchronized with a
		// similar switch statement in UpdateWorkUnit implementation.

		case "state":
			// This checks the state is valid by itself. Later on, we will check the
			// transition is valid.
			if err := pbutil.ValidateWorkUnitState(req.WorkUnit.State); err != nil {
				return errors.Fmt("work_unit: state: %w", err)
			}
		case "summary_markdown":
			// In FinalizeWorkUnit, we could be permissive about length and truncate it server-side
			// as it is a AIP-136 custom method. However, as this is a standard AIP-134 Update method,
			// we follow the rule what you put is what you get.
			const enforceLength = true
			if err := pbutil.ValidateSummaryMarkdown(req.WorkUnit.SummaryMarkdown, enforceLength); err != nil {
				return errors.Fmt("work_unit: summary_markdown: %w", err)
			}
		case "deadline":
			// Using clock.Now(ctx).UTC() for validation, actual commit time will be used for storage.
			assumedCreateTime := clock.Now(ctx).UTC()
			if err := validateDeadline(req.WorkUnit.Deadline, assumedCreateTime); err != nil {
				return errors.Fmt("work_unit: deadline: %w", err)
			}
		case "tags":
			if err := pbutil.ValidateWorkUnitTags(req.WorkUnit.Tags); err != nil {
				return errors.Fmt("work_unit: tags: %w", err)
			}
		case "properties":
			if err := pbutil.ValidateWorkUnitProperties(req.WorkUnit.Properties); err != nil {
				return errors.Fmt("work_unit: properties: %w", err)
			}
		case "extended_properties":
			if err := pbutil.ValidateInvocationExtendedProperties(req.WorkUnit.ExtendedProperties); err != nil {
				return errors.Fmt("work_unit: extended_properties: %w", err)
			}
		case "instructions":
			if err := pbutil.ValidateInstructions(req.WorkUnit.Instructions); err != nil {
				return errors.Fmt("work_unit: instructions: %w", err)
			}
		default:
			return errors.Fmt("update_mask: unsupported path %q", path)
		}
	}
	return nil
}

// validateUpdateWorkUnitRequestSubmask returns non-nil error if path should
// not have submask, e.g. "deadline.seconds".
func validateUpdateWorkUnitRequestSubmask(path string, submask *mask.Mask) error {
	if path == "extended_properties" {
		for extPropKey, extPropMask := range submask.Children() {
			if err := pbutil.ValidateInvocationExtendedPropertyKey(extPropKey); err != nil {
				return errors.Fmt("update_mask: extended_properties: key %q: %w", extPropKey, err)
			}
			if len(extPropMask.Children()) > 0 {
				return errors.Fmt("update_mask: extended_properties[%q] should not have any submask", extPropKey)
			}
		}
	} else if len(submask.Children()) > 0 {
		return errors.Fmt("update_mask: %q should not have any submask", path)
	}
	return nil
}

func verifyUpdateWorkUnitPermissions(ctx context.Context, req *pb.UpdateWorkUnitRequest) error {
	if req.WorkUnit == nil {
		return appstatus.BadRequest(errors.New("work_unit: unspecified"))
	}
	rootInvocationID, workUnitID, err := pbutil.ParseWorkUnitName(req.WorkUnit.Name)
	if err != nil {
		return appstatus.BadRequest(errors.Fmt("work_unit: name: %w", err))
	}

	token, err := extractUpdateToken(ctx)
	if err != nil {
		return err
	}
	wuID := workunits.ID{RootInvocationID: rootinvocations.ID(rootInvocationID), WorkUnitID: workUnitID}
	if err := validateWorkUnitUpdateToken(ctx, token, wuID); err != nil {
		return appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}
	return nil
}
