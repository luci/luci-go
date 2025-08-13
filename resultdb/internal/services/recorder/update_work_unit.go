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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal/config"
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
	cfg, err := config.Service(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateUpdateWorkUnitRequest(ctx, in, cfg); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	return nil, appstatus.Error(codes.Unimplemented, "not yet implemented")
}

func validateUpdateWorkUnitRequest(ctx context.Context, req *pb.UpdateWorkUnitRequest, cfg *config.CompiledServiceConfig) error {
	// Work unit name already been validated in verifyUpdateWorkUnitPermissions.
	if len(req.UpdateMask.GetPaths()) == 0 {
		return errors.New("update_mask: paths is empty")
	}
	updateMask, err := mask.FromFieldMask(req.UpdateMask, req.WorkUnit, mask.AdvancedSemantics(), mask.ForUpdate())
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
