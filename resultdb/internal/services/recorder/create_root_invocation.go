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
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

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
	cfg, err := config.Service(ctx)
	if err != nil {
		return nil, err
	}
	if err := verifyCreateRootInvocationPermissions(ctx, in, cfg); err != nil {
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
	grpc.SetHeader(ctx, md)

	return &pb.CreateRootInvocationResponse{
		RootInvocation: masking.RootInvocation(rootInvocation, cfg),
		RootWorkUnit:   masking.WorkUnit(workUnitRow, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_FULL, cfg),
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

		shardingAlgorithm, err := rootinvocations.AssignTestShardingAlgorithm(rootInvocationID)
		if err != nil {
			return fmt.Errorf("assigning test sharding algorithm: %w", err)
		}

		rootInvocationRow := &rootinvocations.RootInvocationRow{
			RootInvocationID: rootInvocationID,
			// Should match root work unit.
			FinalizationState: pb.RootInvocation_ACTIVE,
			State:             pbutil.WorkUnitToRootInvocationState(req.RootWorkUnit.State),
			SummaryMarkdown:   req.RootWorkUnit.SummaryMarkdown,

			Realm:                                   req.RootInvocation.Realm,
			CreatedBy:                               createdBy,
			UninterestingTestVerdictsExpirationTime: spanner.NullTime{Valid: true, Time: now.Add(uninterestingTestVerdictsExpirationTime)},
			CreateRequestID:                         req.RequestId,
			ProducerResource:                        req.RootInvocation.ProducerResource,
			Definition:                              req.RootInvocation.Definition,
			Sources:                                 req.RootInvocation.Sources,
			PrimaryBuild:                            req.RootInvocation.PrimaryBuild,
			ExtraBuilds:                             req.RootInvocation.ExtraBuilds,
			Tags:                                    req.RootInvocation.Tags,
			Properties:                              req.RootInvocation.Properties,
			BaselineID:                              req.RootInvocation.BaselineId,
			StreamingExportState:                    req.RootInvocation.StreamingExportState,
			Submitted:                               false, // Submitted is set in separate MarkInvocationSubmitted call.
			FinalizerPending:                        false,
			FinalizerSequence:                       0,
			TestShardingAlgorithm:                   shardingAlgorithm,
		}
		span.BufferWrite(ctx, rootinvocations.Create(rootInvocationRow)...)

		deadline := req.RootWorkUnit.Deadline.AsTime()
		if req.RootWorkUnit.Deadline == nil {
			deadline = now.Add(defaultDeadlineDuration)
		}

		moduleInheritanceStatus := workunits.ModuleInheritanceStatusNoModuleSet
		if req.RootWorkUnit.ModuleId != nil {
			moduleInheritanceStatus = workunits.ModuleInheritanceStatusRoot
		}

		// Root work unit and Root invocation are always created in the same transaction,
		// so the idempotency check on the root invocation is sufficient.
		wuRow := &workunits.WorkUnitRow{
			ID: workunits.ID{
				RootInvocationID: rootInvocationID,
				WorkUnitID:       workunits.RootWorkUnitID,
			},
			// Root work unit has parent work unit set to null.
			ParentWorkUnitID:  spanner.NullString{Valid: false},
			Kind:              req.RootWorkUnit.Kind,
			State:             req.RootWorkUnit.State,
			FinalizationState: pb.WorkUnit_ACTIVE,
			SummaryMarkdown:   req.RootWorkUnit.SummaryMarkdown,
			Realm:             rootInvocationRow.Realm,
			CreatedBy:         createdBy,
			ProducerResource:  rootInvocationRow.ProducerResource,
			// Fields should be set with value in request.RootWorkUnit.
			CreateRequestID:         req.RequestId,
			ModuleID:                req.RootWorkUnit.ModuleId,
			ModuleShardKey:          req.RootWorkUnit.ModuleShardKey,
			ModuleInheritanceStatus: moduleInheritanceStatus,
			Deadline:                deadline,
			Tags:                    req.RootWorkUnit.Tags,
			Properties:              req.RootWorkUnit.Properties,
			Instructions:            req.RootWorkUnit.Instructions,
			ExtendedProperties:      req.RootWorkUnit.ExtendedProperties,
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

func verifyCreateRootInvocationPermissions(ctx context.Context, req *pb.CreateRootInvocationRequest, cfg *config.CompiledServiceConfig) error {
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

	// If an ID not starting with "u-" is specified,
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

	// If the producer resource is set to a system requiring the caller to be
	// validated, resultdb.rootInvocations.setProducerResource permission is required.
	if inv.ProducerResource != nil && validateProducerSystemCaller(inv.ProducerResource.System, cfg) {
		project, _ := realms.Split(realm)
		rootRealm := realms.Join(project, realms.RootRealm)
		allowed, err := checkPermissionOrGroupMember(ctx, rootRealm, permSetRootInvocationProducerResource, trustedCreatorGroup)
		if err != nil {
			return err
		}
		if !allowed {
			return appstatus.Errorf(codes.PermissionDenied, `only root invocations created by trusted system may set the producer_resource.system field to %q`, inv.ProducerResource.System)
		}
	}

	// If a baseline is set,
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

// validateProducerSystemCaller returns whether a producer system requires the
// caller to have resultdb.rootInvocations.setProducerResource permission.
func validateProducerSystemCaller(system string, cfg *config.CompiledServiceConfig) bool {
	if ps, ok := cfg.ProducerSystems[system]; ok {
		return ps.ValidateCallers
	}
	return false
}

func validateCreateRootInvocationRequest(req *pb.CreateRootInvocationRequest, cfg *config.CompiledServiceConfig) error {
	if err := pbutil.ValidateRootInvocationID(req.RootInvocationId); err != nil {
		return errors.Fmt("root_invocation_id: %w", err)
	}
	if err := validateRootInvocationForCreate(req.RootInvocation, cfg); err != nil {
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
func validateRootInvocationForCreate(inv *pb.RootInvocation, cfg *config.CompiledServiceConfig) error {
	if inv == nil {
		return errors.New("unspecified")
	}
	// This method attempts to validate fields in the order that they are specified in the proto.

	// Name, RootInvocationId, FinalizationState, State and SummaryMarkdown are output only and should be ignored
	// as per https://google.aip.dev/203.

	if inv.Realm == "" {
		return errors.New("realm: unspecified")
	}
	if err := realms.ValidateRealmName(inv.Realm, realms.GlobalScope); err != nil {
		return errors.Fmt("realm: %w", err)
	}

	// CreateTime, Creator, FinalizeStartTime, FinalizeTime are output only and should be ignored
	// as per https://google.aip.dev/203.

	// Validate producer_resource.
	if err := pbutil.ValidateProducerResource(inv.ProducerResource); err != nil {
		return errors.Fmt("producer_resource: %w", err)
	}
	if ps, ok := cfg.ProducerSystems[inv.ProducerResource.System]; ok {
		// If the system is configured, apply additional validation.
		if err := ps.Validate(inv.ProducerResource); err != nil {
			return errors.Fmt("producer_resource: %w", err)
		}
	}

	// Validate definition.
	if inv.Definition != nil {
		if err := pbutil.ValidateDefinitionForStorage(inv.Definition); err != nil {
			return errors.Fmt("definition: %w", err)
		}
	}

	// Validate sources.
	if err := pbutil.ValidateSources(inv.Sources); err != nil {
		return errors.Fmt("sources: %w", err)
	}

	// Validate primary build.
	if inv.PrimaryBuild != nil {
		if err := pbutil.ValidateBuildDescriptor(inv.PrimaryBuild); err != nil {
			return errors.Fmt("primary_build: %w", err)
		}
	}

	// Validate extra builds, including validating they don't duplicate the primary build or each other.
	if err := pbutil.ValidateExtraBuildDescriptors(inv.ExtraBuilds); err != nil {
		return errors.Fmt("extra_builds: %w", err)
	}
	if err := pbutil.ValidateBuildDescriptorsUniquenessAndOrder(inv.ExtraBuilds, inv.PrimaryBuild); err != nil {
		return errors.Fmt("extra_builds: %w", err)
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

	// Validate streaming_export_state. This is a required field.
	if err := pbutil.ValidateStreamingExportState(inv.StreamingExportState); err != nil {
		return errors.Fmt("streaming_export_state: %w", err)
	}

	return nil
}

// validateRootWorkUnitForCreate validates the fields of a pb.WorkUnit message
// for a creation request within a root invocation.
func validateRootWorkUnitForCreate(wu *pb.WorkUnit, cfg *config.CompiledServiceConfig) error {
	if wu == nil {
		return errors.New("unspecified")
	}

	if wu.Realm != "" {
		return errors.New("realm: must not be set; always inherited from root invocation")
	}

	if wu.ProducerResource != nil {
		return errors.New("producer_resource: must not be set; always inherited from root invocation")
	}

	return validateWorkUnitForCreate(wu, cfg)
}
