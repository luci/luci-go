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
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
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

	return nil, appstatus.Error(codes.Unimplemented, "not yet implemented")
}

// validateUpdateRootInvocationRequest validates an UpdateRootInvocationRequest.
func validateUpdateRootInvocationRequest(ctx context.Context, req *pb.UpdateRootInvocationRequest) error {
	// The root invocation name is already validated in verifyUpdateRootInvocationPermissions.

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

		case "state":
			// If "state" is set, we only allow "FINALIZING" or "ACTIVE" state.
			// Setting to "FINALIZING" will trigger the finalization process.
			// Setting to "ACTIVE" is a no-op.
			if req.RootInvocation.State != pb.RootInvocation_FINALIZING && req.RootInvocation.State != pb.RootInvocation_ACTIVE {
				return errors.New("root_invocation: state: must be FINALIZING or ACTIVE")
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

	updateMask, err := mask.FromFieldMask(req.UpdateMask, req.RootInvocation, mask.AdvancedSemantics(), mask.ForUpdate())
	if err != nil {
		return appstatus.BadRequest(errors.Fmt("update_mask: %w", err))
	}

	if updateMask.MustIncludes("baseline_id") == mask.IncludeEntirely {
		if err := validateUpdateBaselinePermissions(ctx, realm); err != nil {
			return err // PermissionDenied appstatus error.
		}
	}

	return nil
}
