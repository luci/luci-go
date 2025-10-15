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
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/masking"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	// maxDeadlineDuration is the maximum amount of time a root invocation or
	// work unit can spend in the ACTIVE state before it should be finalized.
	//
	// The longest root invocations are currently from Android Test Production
	// which has a deadline of 7 days. This is set with a margin to that to
	// allow for clock drift.
	//
	// If updating this value, also update WorkUnitUpdateRequests and
	// RootInvocationUpdateRequests row deletion TTLs.
	maxDeadlineDuration = (7*24 + 1) * time.Hour // 7 days, 1 hour

	// By default, finalize root invocation and work unit 2d after creation if it is still
	// not finalized.
	defaultDeadlineDuration = 2 * 24 * time.Hour // 2 days
)

// CreateRootInvocation implements pb.RecorderServer.
func (s *recorderServer) CreateRootInvocation(ctx context.Context, in *pb.CreateRootInvocationRequest) (*pb.CreateRootInvocationResponse, error) {
	// As per https://google.aip.dev/211, check authorisation before
	// validating the request.
	if err := verifyCreateRootInvocationPermissions(ctx, in); err != nil {
		return nil, err
	}
	cfg, err := config.Service(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateCreateRootInvocationRequest(in, cfg); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	if err := createIdempotentRootInvocation(ctx, in, s.ExpectedResultsExpiration); err != nil {
		return nil, err
	}

	// TODO: The response is read in a separate transaction
	// from the creation. This means a duplicate request may not receive an
	// identical response if the invocation was updated in the meantime.
	// While AIP-155 (https://google.aip.dev/155#stale-success-responses)
	// permits returning the most current data, we could instead construct the
	// response from the request data for better consistency.
	rootInvocationID := rootinvocations.ID(in.RootInvocationId)
	rootInvocation, err := rootinvocations.Read(span.Single(ctx), rootInvocationID)
	if err != nil {
		// Error is already annotated with NotFound appstatus if record is not found.
		// Otherwise it is an internal error.
		return nil, err
	}

	workUnitID := workunits.ID{
		RootInvocationID: rootInvocationID,
		WorkUnitID:       workunits.RootWorkUnitID,
	}
	workUnitRow, err := workunits.Read(span.Single(ctx), workUnitID, workunits.AllFields)
	if err != nil {
		// Error is already annotated with NotFound appstatus if record is not found.
		// Otherwise it is an internal error.
		return nil, err
	}

	// RootInvocation and root work unit share the same update token.
	// Use the update token generated from root work unit id.
	token, err := generateWorkUnitUpdateToken(ctx, workUnitID)
	if err != nil {
		return nil, err
	}
	md := metadata.MD{}
	md.Set(pb.UpdateTokenMetadataKey, token)
	prpc.SetHeader(ctx, md)

	return &pb.CreateRootInvocationResponse{
		RootInvocation: rootInvocation.ToProto(),
		RootWorkUnit:   masking.WorkUnit(workUnitRow, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_FULL),
	}, nil
}

func createIdempotentRootInvocation(
	ctx context.Context,
	req *pb.CreateRootInvocationRequest,
	uninterestingTestVerdictsExpirationTime time.Duration,
) error {
	now := clock.Now(ctx)
	createdBy := string(auth.CurrentIdentity(ctx))
	deduped := false
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		rootInvocationID := rootinvocations.ID(req.RootInvocationId)
		var err error
		deduped, err = deduplicateCreateRootInvocations(ctx, rootInvocationID, req.RequestId, createdBy)
		if err != nil {
			return err
		}
		if deduped {
			// This call should be deduplicated, do not write to database.
			return nil
		}
		// Root invocation doesn't exist, create it.
		finalizationState := req.RootInvocation.FinalizationState
		if finalizationState == pb.RootInvocation_FINALIZATION_STATE_UNSPECIFIED {
			finalizationState = pb.RootInvocation_ACTIVE
		}
		deadline := req.RootInvocation.Deadline.AsTime()
		if req.RootInvocation.Deadline == nil {
			deadline = now.Add(defaultDeadlineDuration)
		}

		rootInvocationRow := &rootinvocations.RootInvocationRow{
			RootInvocationID:                        rootInvocationID,
			FinalizationState:                       finalizationState,
			Realm:                                   req.RootInvocation.Realm,
			CreatedBy:                               createdBy,
			Deadline:                                deadline,
			UninterestingTestVerdictsExpirationTime: spanner.NullTime{Valid: true, Time: now.Add(uninterestingTestVerdictsExpirationTime)},
			CreateRequestID:                         req.RequestId,
			ProducerResource:                        req.RootInvocation.ProducerResource,
			Tags:                                    req.RootInvocation.Tags,
			Properties:                              req.RootInvocation.Properties,
			Sources:                                 req.RootInvocation.Sources,
			IsSourcesFinal:                          req.RootInvocation.SourcesFinal,
			BaselineID:                              req.RootInvocation.BaselineId,
			Submitted:                               false, // Submitted is set in separate MarkInvocationSubmitted call.
			FinalizerPending:                        false,
			FinalizerSequence:                       0,
		}
		span.BufferWrite(ctx, rootinvocations.Create(rootInvocationRow)...)

		// Root work unit and Root invocation are always created in the same transaction,
		// so the idempotency check on the root invocation is sufficient.
		wuRow := &workunits.WorkUnitRow{
			ID: workunits.ID{
				RootInvocationID: rootInvocationID,
				WorkUnitID:       workunits.RootWorkUnitID,
			},
			// Root work unit has parent work unit set to null.
			ParentWorkUnitID: spanner.NullString{Valid: false},
			// Fields should be the same as root invocation.
			FinalizationState: rootInvocationStateToWorkUnitState(rootInvocationRow.FinalizationState),
			Realm:             rootInvocationRow.Realm,
			CreatedBy:         createdBy,
			Deadline:          rootInvocationRow.Deadline,
			CreateRequestID:   req.RequestId,
			ModuleID:          req.RootWorkUnit.ModuleId,
			ProducerResource:  rootInvocationRow.ProducerResource,
			// Fields should be set with value in request.RootWorkUnit.
			Tags:               req.RootWorkUnit.Tags,
			Properties:         req.RootWorkUnit.Properties,
			Instructions:       req.RootWorkUnit.Instructions,
			ExtendedProperties: req.RootWorkUnit.ExtendedProperties,
		}
		legacyCreateOpts := workunits.LegacyCreateOptions{
			ExpectedTestResultsExpirationTime: now.Add(uninterestingTestVerdictsExpirationTime),
		}
		span.BufferWrite(ctx, workunits.Create(wuRow, legacyCreateOpts)...)
		return nil
	})
	if err != nil {
		return err
	}
	if !deduped {
		spanutil.IncRowCount(ctx, 1, spanutil.RootInvocations, spanutil.Inserted, req.RootInvocation.Realm)
		spanutil.IncRowCount(ctx, 1, spanutil.WorkUnits, spanutil.Inserted, req.RootInvocation.Realm)
		// Two shadow legacy invocation for root invocation and root work unit.
		spanutil.IncRowCount(ctx, 2, spanutil.Invocations, spanutil.Inserted, req.RootInvocation.Realm)
	}
	return nil
}

// deduplicateCreateRootInvocations returns whether the CreateRootInvocation call should be deduplicated.
func deduplicateCreateRootInvocations(ctx context.Context, id rootinvocations.ID, requestID, createdBy string) (bool, error) {
	curRequestID, curCreatedBy, err := rootinvocations.ReadRequestIDAndCreatedBy(ctx, id)
	if err != nil {
		st, ok := appstatus.Get(err)
		if ok && st.Code() == codes.NotFound {
			// root invocation doesn't exist yet, do not deduplicate this call.
			return false, nil
		}
		return false, err
	}
	if requestID != curRequestID || curCreatedBy != createdBy {
		// Root invocation with the same id exist, and is not created with the same requestID and creator.
		return false, appstatus.Errorf(codes.AlreadyExists, "%s already exists", id.Name())
	}
	// Root invocation with the same id, requestID and creator exist.
	// Could happen if someone sent two different calls with the same request ID, eg. retry.
	// This call should be deduplicated.
	return true, nil
}

func rootInvocationStateToWorkUnitState(state pb.RootInvocation_FinalizationState) pb.WorkUnit_FinalizationState {
	switch state {
	case pb.RootInvocation_ACTIVE:
		return pb.WorkUnit_ACTIVE
	case pb.RootInvocation_FINALIZING:
		return pb.WorkUnit_FINALIZING
	case pb.RootInvocation_FINALIZED:
		return pb.WorkUnit_FINALIZED
	default:
		return pb.WorkUnit_FINALIZATION_STATE_UNSPECIFIED
	}
}

func verifyCreateRootInvocationPermissions(ctx context.Context, req *pb.CreateRootInvocationRequest) error {
	// Only perform validation that is necessary for performing authorisation checks,
	// the full set of validation will occur if these checks pass.
	inv := req.RootInvocation
	if inv == nil {
		return appstatus.BadRequest(errors.New("root_invocation: unspecified"))
	}
	realm := inv.Realm
	if realm == "" {
		return appstatus.BadRequest(errors.New("root_invocation: realm: unspecified"))
	}
	if err := realms.ValidateRealmName(realm, realms.GlobalScope); err != nil {
		return appstatus.BadRequest(errors.Fmt("root_invocation: realm: %w", err))
	}

	// Check we have permission to create the root invocation and its root work unit.
	if allowed, err := auth.HasPermission(ctx, permCreateRootInvocation, realm, nil); err != nil {
		return err
	} else if !allowed {
		return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %q in realm %q`, permCreateRootInvocation, realm)
	}
	if allowed, err := auth.HasPermission(ctx, permCreateWorkUnit, realm, nil); err != nil {
		return err
	} else if !allowed {
		return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %q in realm %q`, permCreateWorkUnit, realm)
	}

	// if an ID not starting with "u-" is specified,
	// resultdb.rootInvocations.createWithReservedID permission is required.
	if !strings.HasPrefix(req.RootInvocationId, "u-") {
		project, _ := realms.Split(realm)
		rootRealm := realms.Join(project, realms.RootRealm)
		allowed, err := checkPermissionOrGroupMember(ctx, rootRealm, permCreateRootInvocationWithReservedID, trustedCreatorGroup)
		if err != nil {
			return err
		}
		if !allowed {
			return appstatus.Errorf(codes.PermissionDenied, `only root invocations created by trusted systems may have id not starting with "u-"; please generate "u-{GUID}" or reach out to ResultDB owners`)
		}
	}

	// if the producer resource is set,
	// resultdb.rootInvocations.setProducerResource permission is required.
	if inv.ProducerResource != "" {
		project, _ := realms.Split(realm)
		rootRealm := realms.Join(project, realms.RootRealm)
		allowed, err := checkPermissionOrGroupMember(ctx, rootRealm, permSetRootInvocationProducerResource, trustedCreatorGroup)
		if err != nil {
			return err
		}
		if !allowed {
			return appstatus.Errorf(codes.PermissionDenied, `only root invocations created by trusted system may have a populated producer_resource field`)
		}
	}

	// if a baseline is set,
	// resultdb.baselines.put permission is required in the ":@project" realm
	// of the LUCI project the root invocation is being created in.
	if inv.BaselineId != "" {
		project, _ := realms.Split(realm)
		projectRealm := realms.Join(project, realms.ProjectRealm)
		if allowed, err := auth.HasPermission(ctx, permPutBaseline, projectRealm, nil); err != nil {
			return err
		} else if !allowed {
			return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission to write to test baseline in realm %q`, projectRealm)
		}
	}

	return nil
}

func validateCreateRootInvocationRequest(req *pb.CreateRootInvocationRequest, cfg *config.CompiledServiceConfig) error {
	if err := pbutil.ValidateRootInvocationID(req.RootInvocationId); err != nil {
		return errors.Fmt("root_invocation_id: %w", err)
	}
	if err := validateRootInvocationForCreate(req.RootInvocation); err != nil {
		return errors.Fmt("root_invocation: %w", err)
	}
	if err := validateRootWorkUnitForCreate(req.RootWorkUnit, cfg); err != nil {
		return errors.Fmt("root_work_unit: %w", err)
	}
	if req.RequestId == "" {
		// Request ID is required to ensure requests are treated idempotently
		// in case of inevitable retries.
		return errors.Fmt("request_id: unspecified (please provide a per-request UUID to ensure idempotence)")
	}
	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Fmt("request_id: %w", err)
	}
	return nil
}

// validateRootInvocationForCreate validates the fields of a pb.RootInvocation message
// for a creation request.
func validateRootInvocationForCreate(inv *pb.RootInvocation) error {
	if inv == nil {
		return errors.New("unspecified")
	}
	// This method attempts to validate fields in the order that they are specified in the proto.

	// Name and RootInvocationId is output only and should be ignored
	// as per https://google.aip.dev/203.

	// Validate state.
	switch inv.FinalizationState {
	case pb.RootInvocation_FINALIZATION_STATE_UNSPECIFIED, pb.RootInvocation_ACTIVE, pb.RootInvocation_FINALIZING:
		// Allowed states for creation.
	default:
		return errors.Fmt("finalization_state: cannot be created in the state %s", inv.FinalizationState)
	}

	if inv.Realm == "" {
		return errors.New("realm: unspecified")
	}
	if err := realms.ValidateRealmName(inv.Realm, realms.GlobalScope); err != nil {
		return errors.Fmt("realm: %w", err)
	}

	// CreateTime, Creator, FinalizeStartTime, FinalizeTime are output only and should be ignored
	// as per https://google.aip.dev/203.

	// Validate deadline.
	if inv.Deadline != nil {
		// Using time.Now() for validation, actual commit time will be used for storage.
		assumedCreateTime := time.Now().UTC()
		if err := validateDeadline(inv.Deadline, assumedCreateTime); err != nil {
			return errors.Fmt("deadline: %w", err)
		}
	}

	// Validate producer_resource.
	if inv.ProducerResource != "" {
		if err := pbutil.ValidateFullResourceName(inv.ProducerResource); err != nil {
			return errors.Fmt("producer_resource: %w", err)
		}
	}

	// Validate sources.
	if inv.Sources != nil {
		if err := pbutil.ValidateSources(inv.Sources); err != nil {
			return errors.Fmt("sources: %w", err)
		}
	}

	// Validate tags.
	if err := pbutil.ValidateRootInvocationTags(inv.Tags); err != nil {
		return errors.Fmt("tags: %w", err)
	}

	// Validate properties.
	if inv.Properties != nil {
		if err := pbutil.ValidateRootInvocationProperties(inv.Properties); err != nil {
			return errors.Fmt("properties: %w", err)
		}
	}

	// Validate baseline_id.
	if inv.BaselineId != "" {
		if err := pbutil.ValidateBaselineID(inv.BaselineId); err != nil {
			return errors.Fmt("baseline_id: %w", err)
		}
	}

	return nil
}

// validateRootWorkUnitForCreate validates the fields of a pb.WorkUnit message
// for a creation request within a root invocation.
func validateRootWorkUnitForCreate(wu *pb.WorkUnit, cfg *config.CompiledServiceConfig) error {
	if wu == nil {
		return errors.New("unspecified")
	}
	// This method attempts to validate fields in the order that they are specified in the proto.

	// Name and WorkUnitId is output only and should be ignored
	// as per https://google.aip.dev/203.

	if wu.FinalizationState != pb.WorkUnit_FINALIZATION_STATE_UNSPECIFIED {
		return errors.New("finalization_state: must not be set; always inherited from root invocation")
	}
	if wu.Realm != "" {
		return errors.New("realm: must not be set; always inherited from root invocation")
	}

	// CreateTime, Creator, FinalizeStartTime, FinalizeTime are output only and should be ignored
	// as per https://google.aip.dev/203.

	if wu.Deadline != nil {
		return errors.New("deadline: must not be set; always inherited from root invocation")
	}

	// Parent, ChildWorkUnits and ChildInvocations are output only fields and should be ignored
	// as per https://google.aip.dev/203.

	if wu.ProducerResource != "" {
		return errors.New("producer_resource: must not be set; always inherited from root invocation")
	}

	return validateWorkUnitForCreate(wu, cfg)
}

// validateDeadline validates a deadline for a root invocation or work unit.
// Returns a non-nil error if deadline is invalid.
func validateDeadline(deadline *timestamppb.Timestamp, createTime time.Time) error {
	internal.AssertUTC(createTime)
	if err := deadline.CheckValid(); err != nil {
		return err
	}

	d := deadline.AsTime()
	if d.Sub(createTime) < 10*time.Second {
		return errors.New("must be at least 10 seconds in the future")
	}
	if d.Sub(createTime) > maxDeadlineDuration {
		return errors.Fmt("must be before %dh in the future", int(maxDeadlineDuration.Hours()))
	}
	return nil
}
