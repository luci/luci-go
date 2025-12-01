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
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/masking"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// BatchCreateWorkUnits implements pb.RecorderServer.
func (s *recorderServer) BatchCreateWorkUnits(ctx context.Context, in *pb.BatchCreateWorkUnitsRequest) (*pb.BatchCreateWorkUnitsResponse, error) {
	cfg, err := config.Service(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateBatchCreateWorkUnitsPermissions(ctx, in, cfg); err != nil {
		return nil, err
	}
	if err := validateBatchCreateWorkUnitsRequest(in, cfg); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	ids, err := createWorkUnitsIdempotent(ctx, in, s.ExpectedResultsExpiration)
	if err != nil {
		return nil, err
	}

	// TODO: The response is read in a separate transaction
	// from the creation. This means a duplicate request may not receive an
	// identical response if the invocation was updated in the meantime.
	// While AIP-155 (https://google.aip.dev/155#stale-success-responses)
	// permits returning the most current data, we could instead construct the
	// response from the request data for better consistency.
	wuRows, err := workunits.ReadBatch(span.Single(ctx), ids, workunits.AllFields)
	if err != nil {
		return nil, err
	}
	workUnits := make([]*pb.WorkUnit, len(in.Requests))
	updateTokens := make([]string, len(in.Requests))
	for i, id := range ids {
		token, err := generateWorkUnitUpdateToken(ctx, id)
		if err != nil {
			return nil, err
		}
		updateTokens[i] = token
		workUnits[i] = masking.WorkUnit(wuRows[i], permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_FULL)
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
) (createdIDs []workunits.ID, err error) {
	now := clock.Now(ctx)
	createdBy := string(auth.CurrentIdentity(ctx))

	parentIDs := make([]workunits.ID, 0, len(in.Requests))
	ids := make([]workunits.ID, 0, len(in.Requests))
	for _, r := range in.Requests {
		parentID := workunits.MustParseName(r.Parent)
		createdID := workunits.ID{
			RootInvocationID: parentID.RootInvocationID,
			WorkUnitID:       r.WorkUnitId,
		}
		parentIDs = append(parentIDs, parentID)
		ids = append(ids, createdID)
	}

	logging.Infof(ctx, "%s", prepareWorkUnitCreationLogMessage(parentIDs, ids))

	dedup := false

	parentsToCheckSet := workunits.NewIDSet(parentIDs...)
	// Only check parents that are not being created in this batch.
	parentsToCheckSet.RemoveAll(workunits.NewIDSet(ids...))
	parentsToCheck := parentsToCheckSet.SortedByRowID()

	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		dedup, err = deduplicateCreateWorkUnits(ctx, ids, in.RequestId, createdBy)
		if err != nil {
			return err
		}
		if dedup {
			// This call should be deduplicated, do not write to database.
			return nil
		}

		// Validate all work units are active.
		workUnitInfo, err := workunits.ReadSummaryInfos(ctx, parentsToCheck)
		if err != nil {
			return err
		}
		for _, parentID := range parentsToCheck {
			wu := workUnitInfo[parentID]
			if wu.FinalizationState != pb.WorkUnit_ACTIVE {
				return appstatus.Errorf(codes.FailedPrecondition, "parent %q is not active", parentID.Name())
			}
		}

		for i, r := range in.Requests {
			wu := r.WorkUnit
			parentWorkUnitInfo, ok := workUnitInfo[parentIDs[i]]
			if !ok {
				return errors.New("logic error: could not find details for parent work unit")
			}

			var moduleID *pb.ModuleIdentifier
			var moduleShardKey string
			var moduleInheritanceStatus workunits.ModuleInheritanceStatus
			if parentWorkUnitInfo.ModuleID != nil {
				// If the parent work unit has a module set, this work unit must
				// inherit it.

				// If the module_ fields are also set on the request, check they
				// are 100% identical to what is being inherited.
				// N.B. ModuleShardKey may only be set of ModuleId is set.
				if wu.ModuleId != nil {
					if diff := diffModuleIdentifier(wu.ModuleId, parentWorkUnitInfo.ModuleID); diff != "" {
						return appstatus.Errorf(codes.FailedPrecondition, "requests[%d]: work_unit: module_id: must match module_id inherited from parent work unit with module set; %s", i, diff)
					}
					if wu.ModuleShardKey != parentWorkUnitInfo.ModuleShardKey {
						return appstatus.Errorf(codes.FailedPrecondition, "requests[%d]: work_unit: module_shard_key: must match module_shard_key inherited from parent work unit with module set; got %q, was %q", i, wu.ModuleShardKey, parentWorkUnitInfo.ModuleShardKey)
					}
				}
				// Inherit from the parent.
				moduleID = parentWorkUnitInfo.ModuleID
				moduleShardKey = parentWorkUnitInfo.ModuleShardKey
				moduleInheritanceStatus = workunits.ModuleInheritanceStatusInherited
			} else if wu.ModuleId != nil {
				// No value is inherited from the parent, but a module is set on the request.
				moduleID = wu.ModuleId
				moduleShardKey = wu.ModuleShardKey
				moduleInheritanceStatus = workunits.ModuleInheritanceStatusRoot
			} else {
				// No module on the parent or in the request.
				moduleInheritanceStatus = workunits.ModuleInheritanceStatusNoModuleSet
			}

			// Populate information about this work unit just created into the map so that
			// child work units (in later request items) can use information about this work
			// unit. The request ordering is such that children are guaranteed to ordered
			// after their parents.
			workUnitInfo[ids[i]] = workunits.SummaryInfo{
				ModuleID:       moduleID,
				ModuleShardKey: moduleShardKey,
			}

			state := wu.State
			if state == pb.WorkUnit_STATE_UNSPECIFIED {
				// TODO: b/447225325 - Remove this defaulting once the field is mandatory.
				state = pb.WorkUnit_PENDING
			}

			deadline := wu.Deadline.AsTime()
			if wu.Deadline == nil {
				deadline = now.Add(defaultDeadlineDuration)
			}

			wuRow := &workunits.WorkUnitRow{
				ID:                      ids[i],
				ParentWorkUnitID:        spanner.NullString{Valid: true, StringVal: parentIDs[i].WorkUnitID},
				Kind:                    wu.Kind,
				State:                   wu.State,
				FinalizationState:       pb.WorkUnit_ACTIVE,
				SummaryMarkdown:         wu.SummaryMarkdown,
				Realm:                   wu.Realm,
				CreatedBy:               createdBy,
				Deadline:                deadline,
				CreateRequestID:         in.RequestId,
				ModuleID:                moduleID,
				ModuleShardKey:          moduleShardKey,
				ModuleInheritanceStatus: moduleInheritanceStatus,
				ProducerResource:        wu.ProducerResource,
				Tags:                    wu.Tags,
				Properties:              wu.Properties,
				Instructions:            wu.Instructions,
				ExtendedProperties:      wu.ExtendedProperties,
			}
			legacyCreateOpts := workunits.LegacyCreateOptions{
				ExpectedTestResultsExpirationTime: now.Add(uninterestingTestVerdictsExpirationTime),
			}
			span.BufferWrite(ctx, workunits.Create(wuRow, legacyCreateOpts)...)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !dedup {
		for _, r := range in.Requests {
			spanutil.IncRowCount(ctx, 1, spanutil.WorkUnits, spanutil.Inserted, r.WorkUnit.Realm)
			// One shadow legacy invocation for the work unit.
			spanutil.IncRowCount(ctx, 1, spanutil.Invocations, spanutil.Inserted, r.WorkUnit.Realm)
		}
	}
	return ids, nil
}

func validateBatchCreateWorkUnitsPermissions(ctx context.Context, req *pb.BatchCreateWorkUnitsRequest, cfg *config.CompiledServiceConfig) error {
	// Only perform minimal validation necessary to verify permissions. Full validation
	// will be performed in validateBatchCreateWorkUnitsRequest.

	// For denial of service reasons, drop large requests early.
	if err := pbutil.ValidateBatchRequestCountAndSize(req.Requests); err != nil {
		return appstatus.BadRequest(errors.Fmt("requests: %w", err))
	}

	// Make sure requests fit our basic structure:
	// - each request must have valid parent and work unit IDs
	// - each request must create a different work unit
	// - if requests are 'chained' in that one work unit relies on a parent
	//   created by another, they must be in the assumed creation order and
	//   there can't be any cycles.
	// - the realm is specified on each request
	requestOrdering, err := validateBatchCreateWorkUnitsRequestStructure(req)
	if err != nil {
		return appstatus.BadRequest(err)
	}

	// Validate inclusion or update token.
	inclusionToken, updateToken, err := extractInclusionOrUpdateToken(ctx)
	if err != nil {
		// InvalidArgument or Unauthenticated appstatus error.
		return err
	}
	if updateToken != "" {
		// Ensure the requests are ones we could authorise with a single update
		// token.
		state, err := validateSameUpdateTokenState(requestOrdering.parentIDs, "parent")
		if err != nil {
			return appstatus.BadRequest(err)
		}
		if err := validateWorkUnitUpdateTokenForState(ctx, updateToken, state); err != nil {
			return err // PermissionDenied appstatus error.
		}
	} else if inclusionToken != "" {
		// Ensure the requests are ones we could authorise with a single inclusion
		// token.
		state, err := validateSameInclusionTokenState(requestOrdering.parentIDs)
		if err != nil {
			return appstatus.BadRequest(err)
		}
		authorizedRealm, err := validateWorkUnitInclusionTokenForState(ctx, inclusionToken, state)
		if err != nil {
			return err // PermissionDenied appstatus error.
		}
		for i, r := range req.Requests {
			realm := r.WorkUnit.Realm
			if realm != authorizedRealm {
				return appstatus.Errorf(codes.PermissionDenied, "requests[%d]: work_unit: realm: got realm %q but inclusion token only authorizes the source to include realm %q", i, realm, authorizedRealm)
			}
		}
	} else {
		// extractInclusionOrUpdateToken should have errored.
		panic("logic error: either update token or inclusion token should have been extracted")
	}

	// To validate permissions, we first need to collect the realm of the parent work unit
	// referenced in each request.

	// Stores a mapping of work unit ID to parent realm (where it is known from another request
	// in the batch).
	knownParentRealms := make(map[workunits.ID]string)
	// Stores the (parent) work unit IDs for which we need to read to find their realm.
	parentIDsToReadRealm := workunits.NewIDSet()
	for i := range req.Requests {
		parentID := requestOrdering.parentIDs[i]
		parentCreateIdx, ok := requestOrdering.createdIDsToIndex[parentID]
		if ok {
			if parentCreateIdx >= i {
				// There could be a cycle in the work unit graph that is being created.
				// This should have been excluded already in validateRequestUniquenessAndOrdering.
				panic("logic error: possible work unit cycle not identified by validateRequestUniquenessAndOrdering")
			}
			// This request refers to a parent created by an earlier request. Fetch the realm
			// from that request. This access is safe as it has been validated by
			// validateBatchCreateWorkUnitsRequestStructure already.
			knownParentRealms[parentID] = req.Requests[parentCreateIdx].WorkUnit.Realm
		} else {
			parentIDsToReadRealm.Add(parentID)
		}
	}

	// Read the realms of parent work units which are not being created in this request.
	// Even though this read is occurring in a different transaction, it is not susceptible to
	// TOC-TOU vulnerabilities as the realm is immutable.
	readRealms, err := workunits.ReadRealms(span.Single(ctx), parentIDsToReadRealm.SortedByRowID())
	if err != nil {
		return err
	}

	// Validate permissions.
	for i, r := range req.Requests {
		var parentRealm string
		parentID := requestOrdering.parentIDs[i]
		parentRealm, ok := knownParentRealms[parentID]
		if !ok {
			parentRealm, ok = readRealms[parentID]
			if !ok {
				panic(fmt.Sprintf("logic error: expected to have read realm for %q", parentID.Name()))
			}
		}

		hasIncludeToken := inclusionToken != ""
		if err := verifyWorkUnitPermissions(ctx, r, cfg, hasIncludeToken, parentRealm); err != nil {
			return appstatus.Errorf(codes.PermissionDenied, "requests[%d]: %s", i, err.Error())
		}
	}
	return nil
}

type batchCreateWorkUnitIDs struct {
	// The ID of the parent work unit of each request item.
	// parentIDs[i] corresponds to req.Requests[i].
	parentIDs []workunits.ID
	// A mapping of the work unit ID being created by each request item
	// to the index of that request item in req.Requests.
	createdIDsToIndex map[workunits.ID]int
}

// validateBatchCreateWorkUnitsRequestStructure validates the basic request
// structure necessary for permission checks to occur.
//   - the work units to be created are unique
//   - there are no cycles in the work units to be created (via a request
//     order that matches the expected creation order)
//   - the parent and work unit IDs to be created are syntactically valid
//   - a realm is specified on each work unit
func validateBatchCreateWorkUnitsRequestStructure(req *pb.BatchCreateWorkUnitsRequest) (batchCreateWorkUnitIDs, error) {
	parentIDs := make([]workunits.ID, len(req.Requests))
	createdIDsToIndex := make(map[workunits.ID]int, len(req.Requests))
	for i, r := range req.Requests {
		parentID, err := workunits.ParseName(r.Parent)
		if err != nil {
			return batchCreateWorkUnitIDs{}, errors.Fmt("requests[%d]: parent: %w", i, err)
		}
		if err := pbutil.ValidateWorkUnitID(r.WorkUnitId); err != nil {
			return batchCreateWorkUnitIDs{}, errors.Fmt("requests[%d]: work_unit_id: %w", i, err)
		}
		// If the work unit ID is prefixed, it must match the parent's prefix.
		if err := validateWorkUnitIDPrefix(parentID.WorkUnitID, r.WorkUnitId); err != nil {
			return batchCreateWorkUnitIDs{}, errors.Fmt("requests[%d]: work_unit_id: %w", i, err)
		}

		parentIDs[i] = parentID
		createdID := workunits.ID{
			RootInvocationID: parentID.RootInvocationID,
			WorkUnitID:       r.WorkUnitId,
		}
		if createIdx, ok := createdIDsToIndex[createdID]; ok {
			return batchCreateWorkUnitIDs{}, errors.Fmt("requests[%d]: work_unit_id: duplicates work unit id %q from requests[%d]", i, r.WorkUnitId, createIdx)
		}
		if createdID == parentID {
			return batchCreateWorkUnitIDs{}, errors.Fmt("requests[%d]: parent: cannot refer to the work unit created in requests[%d]", i, i)
		}
		createdIDsToIndex[createdID] = i

		wu := r.WorkUnit
		if wu == nil {
			return batchCreateWorkUnitIDs{}, errors.Fmt("requests[%d]: work_unit: unspecified", i)
		}
		realm := wu.Realm
		if realm == "" {
			return batchCreateWorkUnitIDs{}, errors.Fmt("requests[%d]: work_unit: realm: unspecified", i)
		}
		if err := realms.ValidateRealmName(realm, realms.GlobalScope); err != nil {
			return batchCreateWorkUnitIDs{}, errors.Fmt("requests[%d]: work_unit: realm: %w", i, err)
		}
	}

	for i := range req.Requests {
		parentID := parentIDs[i]
		idx, ok := createdIDsToIndex[parentID]
		if ok && idx >= i {
			// This parent is created in the same request, but at a later offset.
			return batchCreateWorkUnitIDs{}, errors.Fmt("requests[%d]: parent: cannot refer to work unit created in the later request requests[%d], please order requests to match expected creation order", i, idx)
		}
	}
	return batchCreateWorkUnitIDs{
		parentIDs:         parentIDs,
		createdIDsToIndex: createdIDsToIndex,
	}, nil
}

// verifyWorkUnitPermissions completes validating that the caller
// has permission to create the given work unit.
func verifyWorkUnitPermissions(ctx context.Context, req *pb.CreateWorkUnitRequest, cfg *config.CompiledServiceConfig, hasInclusionToken bool, parentRealm string) error {
	// Already validated by validateBatchCreateWorkUnitsRequestStructure.
	wu := req.WorkUnit
	realm := wu.Realm

	if parentRealm == "" {
		panic("logic error: parentRealm is not supplied")
	}

	// Creation/inclusion does not need to be re-authorised if the work unit is being created
	// in the same realm:
	// - integrity: the caller already has ability to contribute results to the realm,
	//   as evidenced by them having the update token for the parent work unit. Creating another
	//   work unit in the same realm does not create incremental risk.
	// - confidentiality: the results visible via the newly created work unit will be those
	//   the caller uploads. There is a risk the caller is being duped into uploading results
	//   to a parent work unit/root invocation that shouldn't have those results (e.g. root
	//   invocation in another project or a world-readable realm) but there is nothing we
	//   can check here to determine if that is the case as such declassification operations
	//   can be intentional. If the uploader is distrustful of the system calling it, the
	//   security model relies on them requiring their caller to call DelegateWorkUnitInclusion
	//   and passing the minted inclusion token to this RPC. That will validate their caller's
	//   privileges to see results from (include from) this realm.

	if parentRealm != realm {
		// Check we have permission to create the work unit.
		// This permission exists to authorises the "risk to realm data integrity" posed by this operation.
		if allowed, err := auth.HasPermission(ctx, permCreateWorkUnit, realm, nil); err != nil {
			return err
		} else if !allowed {
			return errors.Fmt(`caller does not have permission %q in realm %q`, permCreateWorkUnit, realm)
		}

		// Include tokens authorise inclusion.
		if !hasInclusionToken {
			// Check we have permission to include this work unit into a root invocation (of possibly different
			// realm). This permission authorises the "risk to realm data confidentiality" posed by this operation,
			// because data in this new work unit may be implicitly shared with readers of the root invocation.
			if allowed, err := auth.HasPermission(ctx, permIncludeWorkUnit, realm, nil); err != nil {
				return err
			} else if !allowed {
				return errors.Fmt(`caller does not have permission %q in realm %q`, permIncludeWorkUnit, realm)
			}
		}
	}

	_, isPrefixed := workUnitIDPrefix(req.WorkUnitId)
	// If the work unit is prefixed, it must match the parent's prefix. This is
	// validated in validateBatchCreateWorkUnitsRequestStructure. No additional permissions
	// are required for this case.
	if !isPrefixed {
		// Otherwise if an ID not starting with "u-" is specified,
		// resultdb.workUnits.createWithReservedID permission is required.
		if !strings.HasPrefix(req.WorkUnitId, "u-") {
			project, _ := realms.Split(realm)
			rootRealm := realms.Join(project, realms.RootRealm)
			allowed, err := checkPermissionOrGroupMember(ctx, rootRealm, permCreateWorkUnitWithReservedID, trustedCreatorGroup)
			if err != nil {
				return err
			}
			if !allowed {
				return errors.Fmt(`work_unit_id: only work units created by trusted systems may have id not starting with "u-"; please generate "u-{GUID}" or reach out to ResultDB owners`)
			}
		}
	}

	// if the producer resource is set,
	// resultdb.workUnits.setProducerResource permission is required.
	if wu.ProducerResource != nil && validateProducerSystemCaller(wu.ProducerResource.System, cfg) {
		project, _ := realms.Split(realm)
		rootRealm := realms.Join(project, realms.RootRealm)
		allowed, err := checkPermissionOrGroupMember(ctx, rootRealm, permSetWorkUnitProducerResource, trustedCreatorGroup)
		if err != nil {
			return err
		}
		if !allowed {
			return errors.Fmt(`work_unit: producer_resource: only work units created by trusted system may have a populated producer_resource field`)
		}
	}
	return nil
}

func validateBatchCreateWorkUnitsRequest(req *pb.BatchCreateWorkUnitsRequest, cfg *config.CompiledServiceConfig) error {
	if req.RequestId == "" {
		// Request ID is required to ensure requests are treated idempotently
		// in case of inevitable retries.
		return errors.Fmt("request_id: unspecified (please provide a per-request UUID to ensure idempotence)")
	}
	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Fmt("request_id: %w", err)
	}

	for i, r := range req.Requests {
		// Validate the sub-request.
		// The request ID is not specified on the sub-request as it is already
		// specified on the parent.
		if err := validateCreateWorkUnitRequest(r, cfg); err != nil {
			return errors.Fmt("requests[%d]: %w", i, err)
		}
		if r.RequestId != "" && r.RequestId != req.RequestId {
			return errors.Fmt("requests[%d]: request_id: inconsistent with top-level request_id", i)
		}
	}
	return nil
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

// prepareWorkUnitCreationLogMessage prepares a log message describing the work units being created.
func prepareWorkUnitCreationLogMessage(parentIDs []workunits.ID, ids []workunits.ID) string {
	var message strings.Builder
	message.WriteString("Creating work unit ")
	for i := range ids {
		if i >= 3 {
			// Don't log more than 3 examples.
			message.WriteString(fmt.Sprintf(" (and %d more)", len(ids)-3))
			break
		}
		if i > 0 {
			isLastItem := i == 2 || (i == len(ids)-1)
			if isLastItem {
				message.WriteString(" and ")
			} else {
				message.WriteString(", ")
			}
		}
		message.WriteString(fmt.Sprintf("%q", ids[i].WorkUnitID))
		message.WriteString(" in ")
		message.WriteString(fmt.Sprintf("%q", parentIDs[i].WorkUnitID))
	}
	message.WriteString(fmt.Sprintf(" (root invocation: %q)", parentIDs[0].RootInvocationID))
	return message.String()
}

// validateSameUpdateTokenState validates all work units share the same update token state.
// If the method succeeds, the value of the shared state is returned.
func validateSameUpdateTokenState(parents []workunits.ID, fieldName string) (state string, err error) {
	s := workUnitUpdateTokenState(parents[0])
	for i, p := range parents {
		// Validate all requests have the same root invocation ID. This will catch
		// a subset of the cases and is more understandable than the update token error below.
		if p.RootInvocationID != parents[0].RootInvocationID {
			return "", errors.Fmt("requests[%d]: %s: all requests must be for the same root invocation", i, fieldName)
		}

		if s != workUnitUpdateTokenState(p) {
			return "", errors.Fmt("requests[%d]: %s %q requires a different update token to requests[0]'s %q %q, but this RPC only accepts one update token", i, fieldName, p.Name(), fieldName, parents[0].Name())
		}
	}
	return s, nil
}

func validateSameInclusionTokenState(parents []workunits.ID) (state string, err error) {
	s := workUnitInclusionTokenState(parents[0])
	for i, p := range parents {
		// Validate all requests have the same root invocation ID. This will catch
		// a subset of the cases and is more understandable than the inclusion token error below.
		if p.RootInvocationID != parents[0].RootInvocationID {
			return "", errors.Fmt("requests[%d]: parent: all requests must be for creations in the same root invocation", i)
		}
		if s != workUnitInclusionTokenState(p) {
			return "", errors.Fmt("requests[%d]: parent %q requires a different inclusion token to requests[0].parent %q, but this RPC only accepts one inclusion token", i, p.Name(), parents[0].Name())
		}
	}
	return s, nil
}
