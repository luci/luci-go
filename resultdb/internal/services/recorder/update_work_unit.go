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
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/invocations/invocationspb"
	"go.chromium.org/luci/resultdb/internal/masking"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tasks"
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
	if err := validateUpdateWorkUnitRequest(ctx, in, cfg); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	var ret *pb.WorkUnit
	shouldFinalizeWorkUnit := false
	updated := false
	wuID := workunits.MustParseName(in.WorkUnit.Name)
	ct, err := mutateWorkUnit(ctx, wuID, func(ctx context.Context) error {
		curWu, err := workunits.Read(ctx, wuID, workunits.AllFields)
		if err != nil {
			return err
		}
		ret = masking.WorkUnit(curWu, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_FULL)

		updateMask, err := mask.FromFieldMask(in.UpdateMask, in.WorkUnit, mask.ForUpdate())
		if err != nil {
			return errors.Fmt("update_mask: %w", err)
		}

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
				// In the case of ACTIVE, it should be a No-op.
				if in.WorkUnit.State == pb.WorkUnit_FINALIZING {
					shouldFinalizeWorkUnit = true
					values["State"] = pb.WorkUnit_FINALIZING
					values["FinalizeStartTime"] = spanner.CommitTimestamp
					legacyInvocationValues["State"] = pb.Invocation_FINALIZING
					legacyInvocationValues["FinalizeStartTime"] = spanner.CommitTimestamp
					ret.State = pb.WorkUnit_FINALIZING
				}
			case "deadline":
				if !proto.Equal(ret.Deadline, in.WorkUnit.Deadline) {
					deadline := in.WorkUnit.Deadline
					values["Deadline"] = deadline
					legacyInvocationValues["Deadline"] = deadline
					ret.Deadline = deadline
				}
			case "module_id":
				curModuleID := ret.ModuleId
				if diff := diffModuleIdentifier(in.WorkUnit.ModuleId, curModuleID); diff != "" {
					if ret.ModuleId != nil {
						return appstatus.BadRequest(errors.Fmt("work_unit: module_id: cannot modify module_id once set (do you need to create a child work unit?); %s", diff))
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
					ret.ModuleId = in.WorkUnit.ModuleId
					pbutil.PopulateModuleIdentifierHashes(ret.ModuleId)
				}

			case "properties":
				if !proto.Equal(ret.Properties, in.WorkUnit.Properties) {
					values["Properties"] = spanutil.Compressed(pbutil.MustMarshal(in.WorkUnit.Properties))
					legacyInvocationValues["Properties"] = spanutil.Compressed(pbutil.MustMarshal(in.WorkUnit.Properties))
					ret.Properties = in.WorkUnit.Properties
				}

			case "tags":
				if !pbutil.StringPairsEqual(ret.Tags, in.WorkUnit.Tags) {
					values["Tags"] = in.WorkUnit.Tags
					legacyInvocationValues["Tags"] = in.WorkUnit.Tags
					ret.Tags = in.WorkUnit.Tags
				}

			case "extended_properties":
				extendedProperties := in.WorkUnit.ExtendedProperties
				updatedExtendedProperties := updateExtendedProperties(ret.ExtendedProperties, extendedProperties, submask)
				if !pbutil.ExtendedPropertiesEqual(updatedExtendedProperties, ret.ExtendedProperties) {
					ret.ExtendedProperties = updatedExtendedProperties
					if err := pbutil.ValidateInvocationExtendedProperties(ret.ExtendedProperties); err != nil {
						// One more validation to ensure the size is within the limit.
						return appstatus.BadRequest(errors.Fmt("work_unit: extended_properties: %w", err))
					}
					internalExtendedProperties := &invocationspb.ExtendedProperties{
						ExtendedProperties: ret.ExtendedProperties,
					}
					values["ExtendedProperties"] = spanutil.Compressed(pbutil.MustMarshal(internalExtendedProperties))
					legacyInvocationValues["ExtendedProperties"] = spanutil.Compressed(pbutil.MustMarshal(internalExtendedProperties))
				}
			case "instructions":
				ins := instructionutil.RemoveInstructionsName(in.WorkUnit.Instructions)
				curIns := instructionutil.RemoveInstructionsName(ret.Instructions)
				if !proto.Equal(curIns, ins) {
					values["Instructions"] = spanutil.Compressed(pbutil.MustMarshal(ins))
					legacyInvocationValues["Instructions"] = spanutil.Compressed(pbutil.MustMarshal(ins))
					ret.Instructions = instructionutil.InstructionsWithNames(in.WorkUnit.Instructions, wuID.Name())
				}
			default:
				panic("impossible")
			}
		}
		// The `values` map is initialized with the 2 primary key columns of a WorkUnit.
		// There is no update if no other columns are added.
		updated = len(values) > 2
		if updated {
			values["LastUpdated"] = spanner.CommitTimestamp
			span.BufferWrite(ctx, spanutil.UpdateMap("WorkUnits", values))
			span.BufferWrite(ctx, spanutil.UpdateMap("Invocations", legacyInvocationValues))
			if shouldFinalizeWorkUnit {
				tasks.StartInvocationFinalization(ctx, wuID.LegacyInvocationID())
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if updated {
		ret.LastUpdated = timestamppb.New(ct)
	}
	if shouldFinalizeWorkUnit {
		ret.FinalizeStartTime = timestamppb.New(ct)
	}
	return ret, nil
}

func validateUpdateWorkUnitRequest(ctx context.Context, req *pb.UpdateWorkUnitRequest, cfg *config.CompiledServiceConfig) error {
	// Work unit name already been validated in verifyUpdateWorkUnitPermissions.
	if len(req.UpdateMask.GetPaths()) == 0 {
		return errors.New("update_mask: paths is empty")
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

		// TODO: support update producer_resource.

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
