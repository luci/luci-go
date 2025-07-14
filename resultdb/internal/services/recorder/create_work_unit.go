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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/tokens"

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
	// TODO: Validate we have the update-token for the parent work unit and that
	// work unit is not FINALIZED.

	return nil, appstatus.Error(codes.Unimplemented, "not yet implemented")
}

// workUnitTokenKind generates and validates tokens issued to authorize
// updating a given work unit or root invocation.
var workUnitTokenKind = tokens.TokenKind{
	Algo:      tokens.TokenAlgoHmacSHA256,
	SecretKey: "work_unit_tokens_secret",
	Version:   1,
}

// generateWorkUnitToken generates an update token for a given work unit or root invocation.
func generateWorkUnitToken(ctx context.Context, id workunits.ID) (string, error) {
	// TODO: If a work unit ID has a colon-separated prefix
	// (e.g., "swarming:task1"), the update token should be generated
	// using only the prefix ("swarming"). This would allow multiple work
	// units from the same logical task to share an update token.

	// Use %q instead of %s which convert string to escaped Go string literal.
	// If we ever let these invocation IDs use colons, this make sure the input is unique.
	state := fmt.Sprintf("%q:%q", id.RootInvocationID, id.WorkUnitID)
	// The token should last as long as a build is allowed to run.
	// Buildbucket has a max of 2 days, so one week should be enough even
	// for other use cases.
	return workUnitTokenKind.Generate(ctx, []byte(state), nil, 7*day) // One week.
}

// validateWorkUnitToken validates an update token for a work unit or root invocation.
//
// For a root invocation, use the ID of its corresponding root work unit
// (WorkUnitID: "root"), as they share the same update token.
// Returns an error if the token is invalid, nil otherwise.
func validateWorkUnitToken(ctx context.Context, token string, id workunits.ID) error {
	// TODO: take care of work unit ID with colon-separated prefix.

	state := fmt.Sprintf("%q:%q", id.RootInvocationID, id.WorkUnitID)
	_, err := workUnitTokenKind.Validate(ctx, token, []byte(state))
	return err
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
