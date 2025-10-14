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
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tokens"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// CreateWorkUnit implements pb.RecorderServer.
func (s *recorderServer) CreateWorkUnit(ctx context.Context, in *pb.CreateWorkUnitRequest) (*pb.WorkUnit, error) {
	// Piggy back off BatchCreateWorkUnits.
	req := &pb.BatchCreateWorkUnitsRequest{
		Requests:  []*pb.CreateWorkUnitRequest{in},
		RequestId: in.RequestId,
	}
	rsp, err := s.BatchCreateWorkUnits(ctx, req)
	if err != nil {
		// Remove any references to "requests[0]: ", this is a single create RPC not a batch RPC.
		return nil, removeRequestNumberFromAppStatusError(err)
	}

	md := metadata.MD{}
	md.Set(pb.UpdateTokenMetadataKey, rsp.UpdateTokens[0])
	prpc.SetHeader(ctx, md)
	return rsp.WorkUnits[0], nil
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
	if err != nil {
		return appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}
	return nil
}

// validateWorkUnitUpdateTokenForState validates an update token for a work unit or set of work units.
//
// State is the the state returned by workUnitUpdateTokenState.
// Returns an error if the token is invalid, nil otherwise.
func validateWorkUnitUpdateTokenForState(ctx context.Context, token string, state string) error {
	_, err := workUnitUpdateTokenKind.Validate(ctx, token, []byte(state))
	if err != nil {
		return appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}
	return nil
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
// It ensures that any modifications happen only if the work unit is ACTIVE.
//
// It executes the provided function `f` within a read-write transaction after
// performing these checks.
//
// On success, it returns the Spanner commit timestamp.
func mutateWorkUnit(ctx context.Context, id workunits.ID, f func(context.Context) error) (time.Time, error) {
	commitTimestamp, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		state, err := workunits.ReadFinalizationState(ctx, id)
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

// validateCreateWorkUnitRequest validates the given create work unit request.
// requireRequestID should be set to true for all single work unit creation requests.
// It should only be false for batch work unit creations where the request ID is set
// on the parent.
func validateCreateWorkUnitRequest(req *pb.CreateWorkUnitRequest, cfg *config.CompiledServiceConfig) error {
	if err := validateNonRootWorkUnitForCreate(req.WorkUnit, cfg); err != nil {
		return errors.Fmt("work_unit: %w", err)
	}
	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Fmt("request_id: %w", err)
	}
	return nil
}

// validateNonRootWorkUnitForCreate validates the fields of a pb.WorkUnit message
// for creation (outside of a root work unit creation).
func validateNonRootWorkUnitForCreate(wu *pb.WorkUnit, cfg *config.CompiledServiceConfig) error {
	if wu == nil {
		return errors.New("unspecified")
	}
	// This method attempts to validate fields in the order that they are specified in the proto.

	// Name and WorkUnitId is output only and should be ignored
	// as per https://google.aip.dev/203.

	// Validate state.
	switch wu.FinalizationState {
	case pb.WorkUnit_FINALIZATION_STATE_UNSPECIFIED, pb.WorkUnit_ACTIVE, pb.WorkUnit_FINALIZING:
		// Allowed states for creation.
	default:
		return errors.Fmt("finalization_state: cannot be created in the state %s", wu.FinalizationState)
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
	return validateWorkUnitForCreate(wu, cfg)
}

// validateWorkUnitForCreate implements common work unit validation between
// root and non-root work unit creations.
func validateWorkUnitForCreate(wu *pb.WorkUnit, cfg *config.CompiledServiceConfig) error {
	if wu.ModuleId != nil {
		if err := pbutil.ValidateModuleIdentifierForStorage(wu.ModuleId); err != nil {
			return errors.Fmt("module_id: %w", err)
		}
		if err := validateModuleIdentifierAgainstConfig(wu.ModuleId, cfg); err != nil {
			return errors.Fmt("module_id: %w", err)
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

// validateModuleIdentifierAgainstConfig validates the module identifier
// against the service configuration.
func validateModuleIdentifierAgainstConfig(moduleID *pb.ModuleIdentifier, cfg *config.CompiledServiceConfig) error {
	_, ok := cfg.Schemes[moduleID.ModuleScheme]
	if !ok {
		return errors.Fmt("module_scheme: scheme %q is not a known scheme by the ResultDB deployment; see go/resultdb-schemes for instructions how to define a new scheme", moduleID.ModuleScheme)
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
