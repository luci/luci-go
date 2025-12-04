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
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/masking"
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
	rootInvID := rootinvocations.MustParseName(in.RootInvocation.Name)

	updatedBy := string(auth.CurrentIdentity(ctx))
	var updatedRootInvRow *rootinvocations.RootInvocationRow
	updated := false
	ct, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		// Reset variables in case the transaction gets retried.
		updatedRootInvRow = nil
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
			match, err := masking.IsRootInvocationEtagMatch(originalRootInvRow, in.RootInvocation.Etag)
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

	cfg, err := config.Service(ctx)
	if err != nil {
		// Internal error.
		return nil, errors.Fmt("get service config: %w", err)
	}

	return masking.RootInvocation(updatedRootInvRow, cfg), nil
}

func updateRootInvocationInternal(in *pb.UpdateRootInvocationRequest, originalRootInvRow *rootinvocations.RootInvocationRow) (updateMutations []*spanner.Mutation, updatedRow *rootinvocations.RootInvocationRow, err error) {
	updatedRootInvRow := originalRootInvRow.Clone()
	updateMask, err := mask.FromFieldMask(in.UpdateMask, in.RootInvocation, mask.AdvancedSemantics(), mask.ForUpdate())
	if err != nil {
		// Should not happen, as it's validated in the initial request validation.
		return nil, nil, errors.Fmt("update_mask: %w", err)
	}
	rootInvID := rootinvocations.MustParseName(in.RootInvocation.Name)
	mb := rootinvocations.NewMutationBuilder(rootInvID)

	revalidateBuildUniquenessAndOrder := true

	for path := range updateMask.Children() {
		switch path {
		// The cases in this switch statement must be synchronized with a
		// similar switch statement in validateUpdateRootInvocationRequest.

		case "definition":
			if !pbutil.DefinitionsEqual(originalRootInvRow.Definition, in.RootInvocation.Definition) {
				// We can't set the field to a value other than its current value, if the metadata has been marked final.
				if originalRootInvRow.StreamingExportState == pb.RootInvocation_METADATA_FINAL {
					return nil, nil, appstatus.Errorf(codes.FailedPrecondition, "root_invocation: definition: cannot modify already finalized definition (streaming_export_state set to METADATA_FINAL)")
				}

				mb.UpdateDefinition(in.RootInvocation.Definition)
				definition := proto.Clone(in.RootInvocation.Definition).(*pb.RootInvocationDefinition)
				if definition != nil {
					pbutil.PopulateDefinitionHashes(definition)
				}
				updatedRootInvRow.Definition = definition
			}

		case "sources":
			// Are we setting the field to a value other than its current value?
			if !proto.Equal(originalRootInvRow.Sources, in.RootInvocation.Sources) {
				// We can't set the field to a value other than its current value, if the metadata has been marked final.
				if originalRootInvRow.StreamingExportState == pb.RootInvocation_METADATA_FINAL {
					return nil, nil, appstatus.Errorf(codes.FailedPrecondition, "root_invocation: sources: cannot modify already finalized sources (streaming_export_state set to METADATA_FINAL)")
				}
				mb.UpdateSources(in.RootInvocation.Sources)
				updatedRootInvRow.Sources = in.RootInvocation.Sources
			}

		case "primary_build":
			// Are we setting the field to a value other than its current value?
			if !proto.Equal(originalRootInvRow.PrimaryBuild, in.RootInvocation.PrimaryBuild) {
				// We can't set the field to a value other than its current value, if the metadata has been marked final.
				if originalRootInvRow.StreamingExportState == pb.RootInvocation_METADATA_FINAL {
					return nil, nil, appstatus.Errorf(codes.FailedPrecondition, "root_invocation: primary_build: cannot modify already finalized primary_build (streaming_export_state set to METADATA_FINAL)")
				}
				// Force re-validation of the final PrimaryBuild + ExtraBuilds field combination to
				// make sure there are no duplicates and fields are set in the correct order.
				// For now, prepare the update as if it will be OK.
				revalidateBuildUniquenessAndOrder = true
				mb.UpdatePrimaryBuild(in.RootInvocation.PrimaryBuild)
				updatedRootInvRow.PrimaryBuild = in.RootInvocation.PrimaryBuild
			}

		case "extra_builds":
			if !extraBuildsEqual(originalRootInvRow.ExtraBuilds, in.RootInvocation.ExtraBuilds) {
				// We can't set the field to a value other than its current value, if the metadata has been marked final.
				if originalRootInvRow.StreamingExportState == pb.RootInvocation_METADATA_FINAL {
					return nil, nil, appstatus.Errorf(codes.FailedPrecondition, "root_invocation: extra_builds: cannot modify already finalized extra_builds (streaming_export_state set to METADATA_FINAL)")
				}
				// Force re-validation of the final PrimaryBuild + ExtraBuilds field combination to
				// make sure there are no duplicates and fields are set in the correct order.
				// For now, prepare the update as if it will be OK.
				revalidateBuildUniquenessAndOrder = true
				mb.UpdateExtraBuilds(in.RootInvocation.ExtraBuilds)
				updatedRootInvRow.ExtraBuilds = in.RootInvocation.ExtraBuilds
			}

		case "streaming_export_state":
			if originalRootInvRow.StreamingExportState != in.RootInvocation.StreamingExportState {
				if originalRootInvRow.StreamingExportState == pb.RootInvocation_METADATA_FINAL {
					return nil, nil, appstatus.Errorf(codes.FailedPrecondition, "root_invocation: streaming_export_state: transitioning from %v to %v is not allowed", originalRootInvRow.StreamingExportState, in.RootInvocation.StreamingExportState)
				}
				mb.UpdateStreamingExportState(in.RootInvocation.StreamingExportState)
				updatedRootInvRow.StreamingExportState = in.RootInvocation.StreamingExportState
			}

		case "tags":
			if !pbutil.StringPairsEqual(originalRootInvRow.Tags, in.RootInvocation.Tags) {
				tags := in.RootInvocation.Tags
				mb.UpdateTags(tags)
				updatedRootInvRow.Tags = tags
			}
		case "properties":
			if !proto.Equal(originalRootInvRow.Properties, in.RootInvocation.Properties) {
				mb.UpdateProperties(in.RootInvocation.Properties)
				updatedRootInvRow.Properties = in.RootInvocation.Properties
			}
		case "baseline_id":
			if originalRootInvRow.BaselineID != in.RootInvocation.BaselineId {
				baselineID := in.RootInvocation.BaselineId
				mb.UpdateBaselineID(baselineID)
				updatedRootInvRow.BaselineID = baselineID
			}
		default:
			// This should not be reached due to the validation step.
			panic("impossible")
		}
	}

	if revalidateBuildUniquenessAndOrder {
		// Validate the final values of ExtraBuilds and PrimaryBuild will work together.
		if err := pbutil.ValidateBuildDescriptorsUniquenessAndOrder(updatedRootInvRow.ExtraBuilds, updatedRootInvRow.PrimaryBuild); err != nil {
			return nil, nil, appstatus.Errorf(codes.FailedPrecondition, "root invocation would be in inconsistent state after update: extra_builds: %s", err)
		}
	}
	return mb.Build(), updatedRootInvRow, nil
}

func extraBuildsEqual(a, b []*pb.BuildDescriptor) bool {
	if len(a) != len(b) {
		return false
	}
	// Length are equal. Compare each element.
	for i := range a {
		if !proto.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

// validateUpdateRootInvocationRequest validates an UpdateRootInvocationRequest.
func validateUpdateRootInvocationRequest(ctx context.Context, req *pb.UpdateRootInvocationRequest) error {
	// The root invocation name is already validated in verifyUpdateRootInvocationPermissions.

	if req.RootInvocation.Etag != "" {
		if _, err := masking.ParseRootInvocationEtag(req.RootInvocation.Etag); err != nil {
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
		case "definition":
			if req.RootInvocation.Definition != nil {
				if err := pbutil.ValidateDefinitionForStorage(req.RootInvocation.Definition); err != nil {
					return errors.Fmt("root_invocation: definition: %w", err)
				}
			}

		case "sources":
			if err := pbutil.ValidateSources(req.RootInvocation.Sources); err != nil {
				return errors.Fmt("root_invocation: sources: %w", err)
			}

		case "primary_build":
			if req.RootInvocation.PrimaryBuild != nil {
				if err := pbutil.ValidateBuildDescriptor(req.RootInvocation.PrimaryBuild); err != nil {
					return errors.Fmt("root_invocation: primary_build: %w", err)
				}
			}

		case "extra_builds":
			if err := pbutil.ValidateExtraBuildDescriptors(req.RootInvocation.ExtraBuilds); err != nil {
				return errors.Fmt("extra_builds: %w", err)
			}
			// Later, we need to validate the extra builds do not duplicate the existing or updated primary build.

		case "streaming_export_state":
			if err := pbutil.ValidateStreamingExportState(req.RootInvocation.StreamingExportState); err != nil {
				return errors.Fmt("root_invocation: streaming_export_state: %w", err)
			}
			// Any value is OK for this first pass validation.
			// However, later we must validate that if we are already in METADATA_FINAL state,
			// we do not transition to WAIT_FOR_METADATA.

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
