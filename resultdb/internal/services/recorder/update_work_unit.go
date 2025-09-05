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
	"go.chromium.org/luci/resultdb/internal/invocations/invocationspb"
	"go.chromium.org/luci/resultdb/internal/masking"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// UpdateWorkUnit implements pb.RecorderServer.
func (s *recorderServer) UpdateWorkUnit(ctx context.Context, in *pb.UpdateWorkUnitRequest) (*pb.WorkUnit, error) {
	if err := verifyUpdateWorkUnitPermissions(ctx, in); err != nil {
		return nil, err
	}
	cfg, err := config.Service(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateUpdateWorkUnitRequest(ctx, in, cfg, true); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	// Piggy back on updateWorkUnits.
	updatedRows, err := updateWorkUnits(ctx, []*pb.UpdateWorkUnitRequest{in}, in.RequestId)
	if err != nil {
		// Remove any references to "requests[0]: ", this is a single create RPC not a batch RPC.
		return nil, removeRequestNumberFromAppStatusError(err)
	}
	res := masking.WorkUnit(updatedRows[0], permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_FULL)
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
	values := map[string]any{
		"RootInvocationShardId": wuID.RootInvocationShardID(),
		"WorkUnitId":            wuID.WorkUnitID,
	}

	legacyInvocationValues := map[string]any{
		"InvocationId": wuID.LegacyInvocationID(),
	}
	for path, submask := range updateMask.Children() {
		switch path {
		// The cases in this switch statement must be synchronized with a
		// similar switch statement in validateUpdateWorkUnitRequest.

		case "state":
			if in.WorkUnit.State == pb.WorkUnit_FINALIZING {
				values["State"] = pb.WorkUnit_FINALIZING
				values["FinalizeStartTime"] = spanner.CommitTimestamp
				legacyInvocationValues["State"] = pb.Invocation_FINALIZING
				legacyInvocationValues["FinalizeStartTime"] = spanner.CommitTimestamp
				updatedRow.State = pb.WorkUnit_FINALIZING
			}
		case "deadline":
			if !curWorkUnitRow.Deadline.Equal(in.WorkUnit.Deadline.AsTime()) {
				deadline := in.WorkUnit.Deadline
				values["Deadline"] = deadline
				legacyInvocationValues["Deadline"] = deadline
				updatedRow.Deadline = deadline.AsTime()
			}
		case "module_id":
			curModuleID := curWorkUnitRow.ModuleID
			if diff := diffModuleIdentifier(in.WorkUnit.ModuleId, curModuleID); diff != "" {
				if curModuleID != nil {
					return nil, nil, appstatus.BadRequest(errors.Fmt("requests[%d]: work_unit: module_id: cannot modify module_id once set (do you need to create a child work unit?); %s", ordinal, diff))
				}
				// curModuleID is nil. And the specified in.WorkUnit.ModuleId is not equal to it.
				// Therefore, we must be setting the module to something substantive.
				values["ModuleName"] = in.WorkUnit.ModuleId.ModuleName
				values["ModuleScheme"] = in.WorkUnit.ModuleId.ModuleScheme
				values["ModuleVariant"] = in.WorkUnit.ModuleId.ModuleVariant
				values["ModuleVariantHash"] = pbutil.VariantHash(in.WorkUnit.ModuleId.ModuleVariant)

				legacyInvocationValues["ModuleName"] = in.WorkUnit.ModuleId.ModuleName
				legacyInvocationValues["ModuleScheme"] = in.WorkUnit.ModuleId.ModuleScheme
				legacyInvocationValues["ModuleVariant"] = in.WorkUnit.ModuleId.ModuleVariant
				legacyInvocationValues["ModuleVariantHash"] = pbutil.VariantHash(in.WorkUnit.ModuleId.ModuleVariant)

				// Populate output-only fields in the response.
				updatedRow.ModuleID = in.WorkUnit.ModuleId
				pbutil.PopulateModuleIdentifierHashes(updatedRow.ModuleID)
			}

		case "properties":
			if !proto.Equal(curWorkUnitRow.Properties, in.WorkUnit.Properties) {
				values["Properties"] = spanutil.Compressed(pbutil.MustMarshal(in.WorkUnit.Properties))
				legacyInvocationValues["Properties"] = spanutil.Compressed(pbutil.MustMarshal(in.WorkUnit.Properties))
				updatedRow.Properties = in.WorkUnit.Properties
			}

		case "tags":
			if !pbutil.StringPairsEqual(curWorkUnitRow.Tags, in.WorkUnit.Tags) {
				values["Tags"] = in.WorkUnit.Tags
				legacyInvocationValues["Tags"] = in.WorkUnit.Tags
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
				internalExtendedProperties := &invocationspb.ExtendedProperties{
					ExtendedProperties: updatedExtendedProperties,
				}
				values["ExtendedProperties"] = spanutil.Compressed(pbutil.MustMarshal(internalExtendedProperties))
				legacyInvocationValues["ExtendedProperties"] = spanutil.Compressed(pbutil.MustMarshal(internalExtendedProperties))
				updatedRow.ExtendedProperties = updatedExtendedProperties
			}
		case "instructions":
			ins := instructionutil.RemoveInstructionsName(in.WorkUnit.Instructions)
			curIns := instructionutil.RemoveInstructionsName(curWorkUnitRow.Instructions)
			if !proto.Equal(curIns, ins) {
				values["Instructions"] = spanutil.Compressed(pbutil.MustMarshal(ins))
				legacyInvocationValues["Instructions"] = spanutil.Compressed(pbutil.MustMarshal(ins))
				updatedRow.Instructions = instructionutil.InstructionsWithNames(in.WorkUnit.Instructions, wuID.Name())
			}
		default:
			panic("impossible")
		}
	}
	// The `values` map is initialized with the 2 primary key columns of a WorkUnit.
	// There is no update if no other columns are added.
	updated := len(values) > 2
	if updated {
		values["LastUpdated"] = spanner.CommitTimestamp
		updateMutations = append(updateMutations, spanutil.UpdateMap("WorkUnits", values))
		updateMutations = append(updateMutations, spanutil.UpdateMap("Invocations", legacyInvocationValues))
	}
	return updateMutations, updatedRow, nil
}

func validateUpdateWorkUnitRequest(ctx context.Context, req *pb.UpdateWorkUnitRequest, cfg *config.CompiledServiceConfig, requireRequestID bool) error {
	if req.WorkUnit == nil {
		return errors.Fmt("work_unit: unspecified")
	}
	if err := pbutil.ValidateWorkUnitName(req.WorkUnit.Name); err != nil {
		return errors.Fmt("work_unit: name: %w", err)
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
			// If "state" is set, we only allow "FINALIZING" or "ACTIVE" state.
			// Setting to "FINALIZING" will trigger the finalization process.
			// Setting to "ACTIVE" is a no-op.
			if req.WorkUnit.State != pb.WorkUnit_FINALIZING && req.WorkUnit.State != pb.WorkUnit_ACTIVE {
				return errors.New("work_unit: state: must be FINALIZING or ACTIVE")
			}

		case "deadline":
			// Using clock.Now(ctx).UTC() for validation, actual commit time will be used for storage.
			assumedCreateTime := clock.Now(ctx).UTC()
			if err := validateDeadline(req.WorkUnit.Deadline, assumedCreateTime); err != nil {
				return errors.Fmt("work_unit: deadline: %w", err)
			}

		case "module_id":
			// Ensure this is a valid module_id. Later we must validate that if module_id is already set,
			// it can't be updated to a different value.
			if req.WorkUnit.ModuleId != nil {
				if err := pbutil.ValidateModuleIdentifierForStorage(req.WorkUnit.ModuleId); err != nil {
					return errors.Fmt("work_unit: module_id: %w", err)
				}
				if err := validateModuleIdentifierAgainstConfig(req.WorkUnit.ModuleId, cfg); err != nil {
					return errors.Fmt("work_unit: module_id: %w", err)
				}
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
