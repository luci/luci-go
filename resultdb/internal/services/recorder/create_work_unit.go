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
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tokens"

	"go.chromium.org/luci/resultdb/internal/masking"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// CreateWorkUnit implements pb.RecorderServer.
func (s *recorderServer) CreateWorkUnit(ctx context.Context, in *pb.CreateWorkUnitRequest) (*pb.WorkUnit, error) {
	if err := verifyCreateWorkUnitPermissions(ctx, in); err != nil {
		return nil, err
	}
	if err := validateCreateWorkUnitRequest(in, true); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	if err := createIdempotentWorkUnit(ctx, in, s.ExpectedResultsExpiration); err != nil {
		return nil, err
	}
	wuID := workunits.ID{
		RootInvocationID: workunits.MustParseName(in.Parent).RootInvocationID,
		WorkUnitID:       in.WorkUnitId,
	}
	// TODO: The response is read in a separate transaction
	// from the creation. This means a duplicate request may not receive an
	// identical response if the invocation was updated in the meantime.
	// While AIP-155 (https://google.aip.dev/155#stale-success-responses)
	// permits returning the most current data, we could instead construct the
	// response from the request data for better consistency.
	workUnitRow, err := workunits.Read(span.Single(ctx), wuID, workunits.AllFields)
	if err != nil {
		return nil, err
	}
	token, err := generateWorkUnitUpdateToken(ctx, wuID)
	if err != nil {
		return nil, err
	}
	md := metadata.MD{}
	md.Set(pb.UpdateTokenMetadataKey, token)
	prpc.SetHeader(ctx, md)
	inputs := masking.WorkUnitFields{
		Row: workUnitRow,
		// No child should be created at this point.
	}
	return masking.WorkUnit(inputs, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_FULL), nil
}

func createIdempotentWorkUnit(
	ctx context.Context,
	in *pb.CreateWorkUnitRequest,
	uninterestingTestVerdictsExpirationTime time.Duration,
) error {
	parentID := workunits.MustParseName(in.Parent)
	now := clock.Now(ctx)
	createdBy := string(auth.CurrentIdentity(ctx))
	wuID := workunits.ID{
		RootInvocationID: parentID.RootInvocationID,
		WorkUnitID:       in.WorkUnitId,
	}
	_, err := mutateWorkUnit(ctx, parentID, func(ctx context.Context) error {
		wu := in.WorkUnit
		deduped, err := deduplicateCreateWorkUnit(ctx, wuID, in.RequestId, createdBy)
		if err != nil {
			return err
		}
		if deduped {
			// This call should be deduplicated, do not write to database.
			return nil
		}

		state := wu.State
		if state == pb.WorkUnit_STATE_UNSPECIFIED {
			state = pb.WorkUnit_ACTIVE
		}
		deadline := wu.Deadline.AsTime()
		if wu.Deadline == nil {
			deadline = now.Add(defaultDeadlineDuration)
		}

		wuRow := &workunits.WorkUnitRow{
			ID:                 wuID,
			ParentWorkUnitID:   spanner.NullString{Valid: true, StringVal: parentID.WorkUnitID},
			State:              state,
			Realm:              wu.Realm,
			CreatedBy:          createdBy,
			Deadline:           deadline,
			CreateRequestID:    in.RequestId,
			ProducerResource:   wu.ProducerResource,
			Tags:               wu.Tags,
			Properties:         wu.Properties,
			Instructions:       wu.Instructions,
			ExtendedProperties: wu.ExtendedProperties,
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
	// TODO: increment row count metrics on new record creation.
	return nil
}

func deduplicateCreateWorkUnit(ctx context.Context, id workunits.ID, requestID, createdBy string) (bool, error) {
	curRequestID, curCreatedBy, err := workunits.ReadRequestIDAndCreatedBy(ctx, id)
	if err != nil {
		st, ok := appstatus.Get(err)
		if ok && st.Code() == codes.NotFound {
			// Work unit doesn't exist yet, do not deduplicate this call.
			return false, nil
		}
		return false, err
	}
	if requestID != curRequestID || curCreatedBy != createdBy {
		// Work unit with the same id exist, and is not created with the same requestID and creator.
		return false, appstatus.Errorf(codes.AlreadyExists, "%q already exists", id.Name())
	}
	// Work unit with the same id and requestID and creator exist.
	// Could happen if someone sent two different calls with the same request ID, eg. retry.
	// This call should be deduplicated.
	return true, nil
}

// workUnitUpdateTokenKind generates and validates tokens issued to authorize
// updating a given work unit or root invocation.
var workUnitUpdateTokenKind = tokens.TokenKind{
	Algo: tokens.TokenAlgoHmacSHA256,
	// The actual secret is derived by mixing the root secret with this key.
	SecretKey: "work_unit_update_token_secret",
	Version:   1,
}

// generateWorkUnitUpdateToken generates an update token for a given work unit or root invocation.
func generateWorkUnitUpdateToken(ctx context.Context, id workunits.ID) (string, error) {
	// The token should last at least as long as a work unit is allowed to remain active.
	// Add one day to give some margin, so that "invocation is not active" errors
	// will occur for a while before invalid token errors.
	return workUnitUpdateTokenKind.Generate(ctx, []byte(workUnitUpdateTokenState(id)), nil, maxDeadlineDuration+day)
}

// validateWorkUnitUpdateToken validates an update token for a work unit or root invocation.
//
// For a root invocation, use the ID of its corresponding root work unit
// (WorkUnitID: "root"), as they share the same update token.
// Returns an error if the token is invalid, nil otherwise.
func validateWorkUnitUpdateToken(ctx context.Context, token string, id workunits.ID) error {
	_, err := workUnitUpdateTokenKind.Validate(ctx, token, []byte(workUnitUpdateTokenState(id)))
	return err
}

func workUnitUpdateTokenState(id workunits.ID) string {
	// A work unit can share an update token with an ancestor by using a prefixed ID
	// format: "ancestorID:suffix". For example, a work unit with ID "wu0:s1" is
	// should have the same update token as the work unit "wu0".
	prefix, ok := workUnitIDPrefix(id.WorkUnitID)
	if ok {
		// Use %q instead of %s which convert string to escaped Go string literal.
		// If we ever let these IDs use semicolons, this makes sure the input is unique.
		return fmt.Sprintf("%q;%q", id.RootInvocationID, prefix)
	} else {
		return fmt.Sprintf("%q;%q", id.RootInvocationID, id.WorkUnitID)
	}
}

// mutateWorkUnit provides a transactional wrapper for modifying a work unit.
// It ensures that any modifications happen only if the work unit is ACTIVE and
// the caller provides a valid update token.
//
// It executes the provided function `f` within a read-write transaction after
// performing these checks. This function MUST be used for all work unit
// mutations (e.g., adding test results, artifacts) to guarantee atomicity
// and proper authorization.
//
// On success, it returns the Spanner commit timestamp.
func mutateWorkUnit(ctx context.Context, id workunits.ID, f func(context.Context) error) (time.Time, error) {
	// Per AIP-211, authorization should precede validation. We intentionally deviate
	// from that pattern by performing update-token authorization inside `mutateWorkUnit`,
	// after initial request validation. This make sure data modification is always
	// performed with authorization check.
	//
	// Because the update-token is not yet validated, logic
	// before the `mutateWorkUnit` call MUST NOT access or return any protected
	// resource data. All preceding checks should operate only on the incoming
	// request parameters.
	token, err := extractUpdateToken(ctx)
	if err != nil {
		return time.Time{}, err
	}
	if err := validateWorkUnitUpdateToken(ctx, token, id); err != nil {
		return time.Time{}, appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}
	commitTimestamp, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		state, err := workunits.ReadState(ctx, id)
		if err != nil {
			return err
		}
		if state != pb.WorkUnit_ACTIVE {
			return appstatus.Errorf(codes.FailedPrecondition, "work unit %q is not active", id.Name())
		}
		return f(ctx)
	})
	if err != nil {
		return time.Time{}, err
	}
	return commitTimestamp, nil
}

func verifyCreateWorkUnitPermissions(ctx context.Context, req *pb.CreateWorkUnitRequest) error {
	// Only perform validation that is necessary for performing authorisation checks,
	// the full set of validation will occur if these checks pass.
	wu := req.WorkUnit
	if wu == nil {
		return appstatus.BadRequest(errors.New("work_unit: unspecified"))
	}
	realm := wu.Realm
	if realm == "" {
		return appstatus.BadRequest(errors.New("work_unit: realm: unspecified"))
	}
	if err := realms.ValidateRealmName(realm, realms.GlobalScope); err != nil {
		return appstatus.BadRequest(errors.Fmt("work_unit: realm: %w", err))
	}

	// Check we have permission to create the work unit.
	// This permission exists to authorises the "risk to realm data integrity" posed by this operation.
	if allowed, err := auth.HasPermission(ctx, permCreateWorkUnit, realm, nil); err != nil {
		return err
	} else if !allowed {
		return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %q in realm %q`, permCreateWorkUnit, realm)
	}

	// Check we have permission to include this work unit into a root invocation (of possibly different
	// realm). This permission authorises the "risk to realm data confidentiality" posed by this operation,
	// because data in this new work unit may be implicitly shared with readers of the root invocation.
	if allowed, err := auth.HasPermission(ctx, permIncludeWorkUnit, realm, nil); err != nil {
		return err
	} else if !allowed {
		return appstatus.Errorf(codes.PermissionDenied, `caller does not have permission %q in realm %q`, permIncludeWorkUnit, realm)
	}

	// if an ID not starting with "u-" is specified,
	// resultdb.workUnits.createWithReservedID permission is required.
	if !strings.HasPrefix(req.WorkUnitId, "u-") {
		project, _ := realms.Split(realm)
		rootRealm := realms.Join(project, realms.RootRealm)
		allowed, err := checkPermissionOrGroupMember(ctx, rootRealm, permCreateWorkUnitWithReservedID, trustedCreatorGroup)
		if err != nil {
			return err
		}
		if !allowed {
			return appstatus.Errorf(codes.PermissionDenied, `work_unit_id: only work units created by trusted systems may have id not starting with "u-"; please generate "u-{GUID}" or reach out to ResultDB owners`)
		}
	}

	// if the producer resource is set,
	// resultdb.workUnits.setProducerResource permission is required.
	if wu.ProducerResource != "" {
		project, _ := realms.Split(realm)
		rootRealm := realms.Join(project, realms.RootRealm)
		allowed, err := checkPermissionOrGroupMember(ctx, rootRealm, permSetWorkUnitProducerResource, trustedCreatorGroup)
		if err != nil {
			return err
		}
		if !allowed {
			return appstatus.Errorf(codes.PermissionDenied, `work_unit: producer_resource: only work units created by trusted system may have a populated producer_resource field`)
		}
	}
	return nil
}

// validateCreateWorkUnitRequest validates the given create work unit request.
// requireRequestID should be set to true for all single work unit creation requests.
// It should only be false for batch work unit creations where the request ID is set
// on the parent.
func validateCreateWorkUnitRequest(req *pb.CreateWorkUnitRequest, requireRequestID bool) error {
	if err := pbutil.ValidateWorkUnitName(req.Parent); err != nil {
		return errors.Fmt("parent: %w", err)
	}
	if err := pbutil.ValidateWorkUnitID(req.WorkUnitId); err != nil {
		return errors.Fmt("work_unit_id: %w", err)
	}

	parentID := workunits.MustParseName(req.Parent)
	if err := validateWorkUnitIDPrefix(parentID.WorkUnitID, req.WorkUnitId); err != nil {
		return errors.Fmt("work_unit_id: %w", err)
	}
	if err := validateWorkUnitForCreate(req.WorkUnit); err != nil {
		return errors.Fmt("work_unit: %w", err)
	}
	if requireRequestID && req.RequestId == "" {
		// Request ID is required to ensure requests are treated idempotently
		// in case of inevitable retries.
		return errors.Fmt("request_id: unspecified (please provide a per-request UUID to ensure idempotence)")
	}
	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Fmt("request_id: %w", err)
	}
	return nil
}

// validateWorkUnitForCreate validates the fields of a pb.WorkUnit message
// for a creation request within a root invocation.
func validateWorkUnitForCreate(wu *pb.WorkUnit) error {
	if wu == nil {
		return errors.New("unspecified")
	}
	// This method attempts to validate fields in the order that they are specified in the proto.

	// Name and WorkUnitId is output only and should be ignored
	// as per https://google.aip.dev/203.

	// Validate state.
	switch wu.State {
	case pb.WorkUnit_STATE_UNSPECIFIED, pb.WorkUnit_ACTIVE, pb.WorkUnit_FINALIZING:
		// Allowed states for creation.
	default:
		return errors.Fmt("state: cannot be created in the state %s", wu.State)
	}

	// Validate realm.
	if wu.Realm == "" {
		return errors.New("realm: unspecified")
	}
	if err := realms.ValidateRealmName(wu.Realm, realms.GlobalScope); err != nil {
		return errors.Fmt("realm: %w", err)
	}

	// CreateTime, Creator, FinalizeStartTime, FinalizeTime are output only and should be ignored
	// as per https://google.aip.dev/203.

	// Validate deadline.
	if wu.Deadline != nil {
		// Using time.Now() for validation, actual commit time will be used for storage.
		assumedCreateTime := time.Now().UTC()
		if err := validateDeadline(wu.Deadline, assumedCreateTime); err != nil {
			return errors.Fmt("deadline: %w", err)
		}
	}

	// Parent, ChildWorkUnits and ChildInvocations are output only fields and should be ignored
	// as per https://google.aip.dev/203.

	if wu.ProducerResource != "" {
		if err := pbutil.ValidateFullResourceName(wu.ProducerResource); err != nil {
			return errors.Fmt("producer_resource: %w", err)
		}
	}
	if err := pbutil.ValidateWorkUnitTags(wu.Tags); err != nil {
		return errors.Fmt("tags: %w", err)
	}
	if wu.Properties != nil {
		if err := pbutil.ValidateWorkUnitProperties(wu.Properties); err != nil {
			return errors.Fmt("properties: %w", err)
		}
	}
	if wu.ExtendedProperties != nil {
		if err := pbutil.ValidateInvocationExtendedProperties(wu.ExtendedProperties); err != nil {
			return errors.Fmt("extended_properties: %w", err)
		}
	}
	if wu.Instructions != nil {
		if err := pbutil.ValidateInstructions(wu.Instructions); err != nil {
			return errors.Fmt("instructions: %w", err)
		}
	}
	return nil
}

// validateWorkUnitIDPrefix ensures that update tokens are shared correctly down a
// work unit hierarchy.
//
// This function enforces a work unit can only share an ancestor's token if its direct parent is either
// that ancestor itself or is also sharing that same ancestor's token.
//
// Example Hierarchy:
//
//	wu0 (has its own token)
//	├── wu0:s1 (Allowed: direct child of wu0, shares wu0's token)
//	│    └── wu0:s2 (Allowed: child of wu0:s1, continues sharing wu0's token)
//	│
//	└── wu2 (Allowed: child of wu0, not prefixed, so it has its own token)
//	     └── wu0:s3 (NOT Allowed: attempts to share wu0's token, but its parent wu2 doesn't share update token with wu0).
func validateWorkUnitIDPrefix(parentID, workUnitID string) error {
	prefix, ok := workUnitIDPrefix(workUnitID)
	if !ok {
		// Not a prefixed work unit ID.
		return nil
	}
	parentPrefix, ok := workUnitIDPrefix(parentID)
	if !ok {
		// Parent work unit id is not prefixed.
		if parentID != prefix {
			return errors.Fmt("work unit ID prefix %q must match parent work unit ID %q", prefix, parentID)
		}
	} else {
		// Parent work unit id is prefixed.
		if parentPrefix != prefix {
			return errors.Fmt("work unit ID prefix %q must match parent work unit ID prefix %q", prefix, parentPrefix)
		}
	}
	return nil
}

// workUnitIDPrefix returns the prefix of a work unit ID and a boolean indicating if a prefix
// was present. It assumes the work unit ID has already been validated.
func workUnitIDPrefix(workUnitID string) (prefix string, ok bool) {
	parts := strings.SplitN(workUnitID, ":", 2)
	if len(parts) > 1 {
		return parts[0], true
	}
	return "", false
}
